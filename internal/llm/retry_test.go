package llm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastRetry() RetryConfig {
	return RetryConfig{MaxAttempts: 5, Base: time.Millisecond, Cap: 5 * time.Millisecond}
}

func sseBody(text string) string {
	return "data: {\"choices\":[{\"delta\":{\"content\":\"" + text + "\"}}]}\n\ndata: [DONE]\n\n"
}

func drain(stream <-chan StreamEvent) []StreamEvent {
	var out []StreamEvent
	for ev := range stream {
		out = append(out, ev)
	}
	return out
}

func TestStreamChatRetriesOn429ThenOK(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody("hi"))
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "k")
	p.SetRetry(fastRetry())
	stream, err := p.StreamChat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, nil, "m")
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	var got string
	for ev := range stream {
		if ev.Type == "text" {
			got += ev.Text
		}
	}
	if got != "hi" {
		t.Fatalf("got %q, want hi", got)
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Fatalf("attempts = %d, want 3", n)
	}
}

func TestStreamChatRetriesOn500ThenOK(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "boom")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody("ok"))
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "k")
	p.SetRetry(fastRetry())
	stream, err := p.StreamChat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, nil, "m")
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	evs := drain(stream)
	if len(evs) == 0 || evs[0].Text != "ok" {
		t.Fatalf("events = %+v", evs)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
}

func TestStreamChatNoRetryOn401(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "k")
	p.SetRetry(fastRetry())
	_, err := p.StreamChat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, nil, "m")
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v, want HTTP 401", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on 401)", n)
	}
}

func TestStreamChatRetriesOnConnRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here anymore

	p := NewOpenAI("test", "http://"+addr, "k")
	p.SetRetry(RetryConfig{MaxAttempts: 3, Base: time.Millisecond, Cap: 2 * time.Millisecond})
	start := time.Now()
	_, err = p.StreamChat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, nil, "m")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "provider test") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("retries too slow: %v", time.Since(start))
	}
}

func TestCompleteRetriesOn503(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"done"}}]}`)
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "k")
	p.SetRetry(fastRetry())
	s, err := p.Complete(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, "m")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if s != "done" {
		t.Fatalf("got %q", s)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
}

func TestFallbackProvider(t *testing.T) {
	dead := NewOpenAI("dead", "http://127.0.0.1:1", "k")
	dead.SetRetry(RetryConfig{MaxAttempts: 2, Base: time.Millisecond, Cap: 2 * time.Millisecond})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody("fb"))
	}))
	defer srv.Close()
	live := NewOpenAI("live", srv.URL, "k")
	live.SetRetry(fastRetry())

	f := NewFallback(dead, live)
	stream, err := f.StreamChat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, nil, "m")
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	evs := drain(stream)
	if len(evs) == 0 || evs[0].Text != "fb" {
		t.Fatalf("events = %+v", evs)
	}
}
