package llm

import (
	"context"
	"errors"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RetryConfig controls how failed provider requests are retried.
// Defaults survive rate limits (HTTP 429), transient 5xx and flaky
// low-bandwidth networks: exponential backoff with jitter, honoring
// the Retry-After header, plus fail-fast dial/header timeouts.
type RetryConfig struct {
	MaxAttempts int
	Base        time.Duration
	Cap         time.Duration
}

func DefaultRetry() RetryConfig {
	n := 6
	if v := os.Getenv("O_LLM_MAX_RETRIES"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			n = i
		}
	}
	return RetryConfig{MaxAttempts: n, Base: 400 * time.Millisecond, Cap: 20 * time.Second}
}

func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, // 408, 429
		http.StatusInternalServerError, http.StatusBadGateway, // 500, 502
		http.StatusServiceUnavailable, http.StatusGatewayTimeout: // 503, 504
		return true
	}
	return false
}

func backoff(base, cap time.Duration, attempt int) time.Duration {
	d := float64(base) * math.Pow(2, float64(attempt))
	if d > float64(cap) {
		d = float64(cap)
	}
	return time.Duration(d * (0.5 + rand.Float64())) // 50%-150% jitter
}

func retryAfterDelay(resp *http.Response, fallback time.Duration) time.Duration {
	if resp == nil {
		return fallback
	}
	h := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if h == "" {
		return fallback
	}
	d := fallback
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(h); err == nil {
		d = time.Until(t)
		if d < 0 {
			d = 0
		}
	}
	if d > 90*time.Second {
		d = 90 * time.Second
	}
	if d < fallback {
		d = fallback
	}
	return d
}

func retryableNetErr(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var ne *net.OpError
	if errors.As(err, &ne) {
		return true
	}
	return os.IsTimeout(err) || errors.Is(err, context.DeadlineExceeded)
}

// doWithRetry executes req, retrying transient failures with backoff.
// The request body is rebuilt from req.GetBody on every attempt.
func (p *OpenAIProvider) doWithRetry(ctx context.Context, req *http.Request, rc RetryConfig) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < rc.MaxAttempts; attempt++ {
		if attempt > 0 {
			if req.GetBody != nil {
				b, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = b
			}
		}
		resp, err := p.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = &Error{Provider: p.id, Kind: KindNetwork, Detail: err.Error(), Err: err}
			if !retryableNetErr(err) || attempt == rc.MaxAttempts-1 {
				return nil, lastErr
			}
			if !sleepCtx(ctx, backoff(rc.Base, rc.Cap, attempt)) {
				return nil, ctx.Err()
			}
			continue
		}
		if resp.StatusCode == http.StatusOK || !retryableStatus(resp.StatusCode) {
			return resp, nil
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		delay := retryAfterDelay(resp, backoff(rc.Base, rc.Cap, attempt))
		resp.Body.Close()
		lastErr = providerError(p.id, resp.StatusCode, b)
		if attempt == rc.MaxAttempts-1 {
			return nil, lastErr
		}
		if !sleepCtx(ctx, delay) {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// watchdogBody closes the underlying body if no Read happens within idle,
// so a stalled low-network stream surfaces as an error instead of hanging.
type watchdogBody struct {
	rc    io.ReadCloser
	idle  time.Duration
	mu    sync.Mutex
	timer *time.Timer
}

func newWatchdogBody(rc io.ReadCloser, idle time.Duration) io.ReadCloser {
	b := &watchdogBody{rc: rc, idle: idle}
	b.timer = time.AfterFunc(idle, func() { _ = rc.Close() })
	return b
}

func (b *watchdogBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	if b.timer != nil {
		b.timer.Reset(b.idle)
	}
	b.mu.Unlock()
	return b.rc.Read(p)
}

func (b *watchdogBody) Close() error {
	b.mu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()
	return b.rc.Close()
}

// newHTTPClient fails fast on slow networks (dial/TLS/first-byte timeouts)
// but does not cap total stream time — the caller's context and the body
// watchdog govern that.
func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 45 * time.Second, // time-to-first-byte guard
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   4,
		},
	}
}
