package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestToolCallWireFormat: the assistant message that carries tool calls
// must use the OpenAI-compatible shape. The flat form makes the provider
// answer with a "missing function field" error.
func TestToolCallWireFormat(t *testing.T) {
	msg := ChatMessage{
		Role:    "assistant",
		Content: "",
		ToolCalls: []ToolCall{
			{ID: "call_1", Name: "read", Arguments: `{"file_path":"a.go"}`},
		},
	}
	b, err := json.Marshal([]ChatMessage{msg})
	if err != nil {
		t.Fatal(err)
	}
	raw := string(b)
	if !strings.Contains(raw, `"type":"function"`) {
		t.Fatalf("the tool call has no function type: %s", raw)
	}
	if !strings.Contains(raw, `"function":{"name":"read"`) {
		t.Fatalf("the tool call has no function object: %s", raw)
	}
	// the content field is always present, even when it is empty
	if !strings.Contains(raw, `"content":""`) {
		t.Fatalf("the assistant message has no content field: %s", raw)
	}
	// and it can be read back
	var back []ChatMessage
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back[0].ToolCalls) != 1 || back[0].ToolCalls[0].Name != "read" {
		t.Fatalf("round trip lost the tool call: %+v", back[0].ToolCalls)
	}
}

// TestToolCallAcceptsFlatShape: endpoints that send the flat form are
// still understood.
func TestToolCallAcceptsFlatShape(t *testing.T) {
	var tc ToolCall
	if err := json.Unmarshal([]byte(`{"id":"c1","name":"read","arguments":"{}"}`), &tc); err != nil {
		t.Fatal(err)
	}
	if tc.Name != "read" || tc.ID != "c1" {
		t.Fatalf("flat tool call not parsed: %+v", tc)
	}
}

// TestProviderNegotiatesToolFormat: when the endpoint refuses the tool
// definitions, the provider asks again with the legacy field, and finally
// without any tools, instead of failing the run.
func TestProviderNegotiatesToolFormat(t *testing.T) {
	var calls int32
	var sawTools, sawFunctions, sawPlain bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, hasTools := body["tools"]
		_, hasFunctions := body["functions"]
		switch {
		case hasTools:
			sawTools = true
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"Missing required parameter: 'function'"}}`)
		case hasFunctions:
			sawFunctions = true
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"function calling is not supported for this model"}}`)
		default:
			sawPlain = true
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseBody("answer without tools"))
		}
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "k")
	p.SetRetry(fastRetry())
	tools := []ToolSpec{ToolSpecFromSchema("read", "read a file", map[string]any{
		"type":       "object",
		"properties": map[string]any{"file_path": map[string]any{"type": "string"}},
	})}
	stream, err := p.StreamChat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, tools, "m")
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	var got string
	for ev := range stream {
		if ev.Type == "text" {
			got += ev.Text
		}
	}
	if got != "answer without tools" {
		t.Fatalf("got %q", got)
	}
	if !sawTools || !sawFunctions || !sawPlain {
		t.Fatalf("negotiation incomplete: tools=%v functions=%v plain=%v", sawTools, sawFunctions, sawPlain)
	}
	if p.Warning() == "" {
		t.Fatal("the provider must warn that it answers without tools")
	}
}

// TestProviderKeepsToolsWhenAccepted: a healthy endpoint is asked with the
// tool definitions and is not bothered with a second format.
func TestProviderKeepsToolsWhenAccepted(t *testing.T) {
	var calls int32
	var sawTools bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; ok {
			sawTools = true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody("ok"))
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "k")
	p.SetRetry(fastRetry())
	tools := []ToolSpec{ToolSpecFromSchema("read", "read a file", nil)}
	stream, err := p.StreamChat(context.Background(), []ChatMessage{{Role: "user", Content: "x"}}, tools, "m")
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	drain(stream)
	if !sawTools {
		t.Fatal("the tool definitions were not sent")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("requests=%d want 1", n)
	}
	if p.Warning() != "" {
		t.Fatal("a healthy provider must not warn")
	}
}
