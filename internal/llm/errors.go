package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Error kinds. They describe what went wrong in terms the UI can explain
// to a human, instead of forwarding the raw backend message.
const (
	KindAuth      = "auth"       // the API key was rejected
	KindForbidden = "forbidden"  // the key is valid but not allowed
	KindNotFound  = "not_found"  // wrong endpoint or model
	KindRateLimit = "rate_limit" // too many requests
	KindServer    = "server"     // provider-side failure
	KindRequest   = "request"    // malformed request
	KindProtocol  = "protocol"   // the provider does not speak our request format
	KindNetwork   = "network"    // the provider could not be reached
	KindParse     = "parse"      // the response could not be read
	KindStream    = "stream"     // the response was cut short
)

// Error is a provider failure with a machine-readable kind. The TUI turns
// it into an explanation and a recovery suggestion; the raw backend body is
// only kept as a short one-line detail.
type Error struct {
	Provider string
	Status   int
	Kind     string
	Detail   string
	Err      error
}

func (e *Error) Error() string {
	switch {
	case e.Status > 0 && e.Detail != "":
		return fmt.Sprintf("provider %s: HTTP %d (%s): %s", e.Provider, e.Status, e.Kind, e.Detail)
	case e.Status > 0:
		return fmt.Sprintf("provider %s: HTTP %d (%s)", e.Provider, e.Status, e.Kind)
	case e.Detail != "":
		return fmt.Sprintf("provider %s: %s: %s", e.Provider, e.Kind, e.Detail)
	default:
		return fmt.Sprintf("provider %s: %s", e.Provider, e.Kind)
	}
}

func (e *Error) Unwrap() error { return e.Err }

// StatusKind maps an HTTP status code to an error kind.
func StatusKind(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return KindAuth
	case status == http.StatusForbidden:
		return KindForbidden
	case status == http.StatusNotFound:
		return KindNotFound
	case status == http.StatusTooManyRequests:
		return KindRateLimit
	case status >= 500:
		return KindServer
	case status >= 400:
		return KindRequest
	default:
		return KindRequest
	}
}

// BodyKind inspects a provider error body for protocol complaints, e.g.
// a tool definition that is missing its "function" object.
func BodyKind(body string) string {
	b := strings.ToLower(body)
	if b == "" {
		return ""
	}
	switch {
	case strings.Contains(b, "tool") && (strings.Contains(b, "function") || strings.Contains(b, "parameter")):
		return KindProtocol
	case strings.Contains(b, "missing") && strings.Contains(b, "function"):
		return KindProtocol
	case strings.Contains(b, "unsupported") && strings.Contains(b, "tool"):
		return KindProtocol
	case strings.Contains(b, "invalid") && strings.Contains(b, "json"):
		return KindParse
	}
	return ""
}

// SummarizeBody turns a provider response body into one readable line: the
// provider's own message when it is JSON, otherwise the first line.
func SummarizeBody(b []byte) string {
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return ""
	}
	if msg := jsonMessage(raw); msg != "" {
		return clip(msg, 300)
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return clip(line, 300)
		}
	}
	return ""
}

// jsonMessage digs the human-readable message out of a JSON error body.
func jsonMessage(raw string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return ""
	}
	if e, ok := obj["error"].(map[string]any); ok {
		if m, ok := e["message"].(string); ok && m != "" {
			return m
		}
		if m, ok := e["type"].(string); ok && m != "" {
			return m
		}
	}
	if e, ok := obj["error"].(string); ok && e != "" {
		return e
	}
	for _, key := range []string{"message", "detail", "reason"} {
		if v, ok := obj[key].(string); ok && v != "" {
			return v
		}
	}
	if v, ok := obj["detail"].([]any); ok && len(v) > 0 {
		if first, ok := v[0].(map[string]any); ok {
			if m, ok := first["msg"].(string); ok {
				return m
			}
		}
	}
	return ""
}

// clip shortens s to n characters on a rune boundary.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
