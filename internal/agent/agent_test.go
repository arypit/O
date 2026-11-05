package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/adaop/o/internal/config"
	"github.com/adaop/o/internal/core"
	octx "github.com/adaop/o/internal/context"
	"github.com/adaop/o/internal/hitl"
	"github.com/adaop/o/internal/llm"
	"github.com/adaop/o/internal/sandbox"
	"github.com/adaop/o/internal/session"
	"github.com/adaop/o/internal/tools"
	"github.com/adaop/o/internal/tools/builtin"
)

// testStep is one scripted assistant turn served by the test HTTP server.
type testStep struct {
	text string
	tool string
	args map[string]any
}

// newTestProvider starts a real HTTP server that speaks the OpenAI-compatible
// SSE streaming protocol, serving the given steps one per chat/completions call.
// No mock provider is shipped; tests exercise the real HTTP+SSE path.
func newTestProvider(t *testing.T, steps []testStep) llm.Provider {
	t.Helper()
	var mu sync.Mutex
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		if call >= len(steps) {
			mu.Unlock()
			http.Error(w, "script exhausted", http.StatusBadRequest)
			return
		}
		s := steps[call]
		call++
		mu.Unlock()
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": "test summary"}}},
			})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		send := func(v map[string]any) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		usage := map[string]any{"input_tokens": 100, "output_tokens": 20}
		if s.text != "" {
			send(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"content": s.text}, "finish_reason": "",
			}}})
		}
		if s.tool != "" {
			argsJSON, _ := json.Marshal(s.args)
			send(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"tool_calls": []any{map[string]any{
					"index": 0, "id": fmt.Sprintf("call_%d", call),
					"function": map[string]any{"name": s.tool, "arguments": string(argsJSON)},
				}}},
				"finish_reason": "",
			}}})
			send(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "tool_calls"}},
				"usage":   usage,
			})
		} else {
			send(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "end_turn"}},
				"usage":   usage,
			})
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	t.Cleanup(srv.Close)
	return llm.NewOpenAI("test", srv.URL, "")
}

func setupAgent(t *testing.T, steps []testStep, maxTurns int) (*Agent, *session.Session, string) {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n"), 0o644)
	sessDir := filepath.Join(dir, "sessions")

	cfg := config.Default()
	sb, err := sandbox.New(dir, sandbox.WorkspaceWrite)
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry()
	builtin.RegisterDefaults(reg)
	builtin.RegisterMutating(reg)
	builtin.RegisterPlan(reg, builtin.PlanCandidates(reg))
	policy := tools.NewPolicy(tools.ModeDefault, nil)
	sess, err := session.New(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	learner := hitl.NewLearner("")
	a := &Agent{
		Cfg: &cfg, Provider: newTestProvider(t, steps), Model: "test-model",
		Reg: reg, Policy: policy, SB: sb,
		Store: octx.NewStore(), Sess: sess,
		Decider: hitl.NewAutoDecider(learner), Learner: learner,
		MaxTurns: maxTurns, CWD: dir, Home: dir, AllowOnce: tools.NewAllowSet(),
	}
	return a, sess, sessDir
}

func TestAgentLoopEndToEnd(t *testing.T) {
	steps := []testStep{
		{tool: "read", args: map[string]any{"file_path": "go.mod"}},
		{tool: "todo", args: map[string]any{"action": "write", "todos": []any{
			map[string]any{"content": "read module", "status": "completed"},
		}}},
		{text: "The module is example.com/x."},
	}
	a, sess, sessDir := setupAgent(t, steps, 10)
	var events []core.AgentEvent
	a.Emit = func(ev core.AgentEvent) { events = append(events, ev) }
	final, err := a.Run(context.Background(), "what module is this?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(final, "example.com/x") {
		t.Errorf("final = %q", final)
	}
	if len(a.Store.Messages) < 6 {
		t.Errorf("store has %d messages, want >= 6", len(a.Store.Messages))
	}
	msgs, err := session.Load(sessDir, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != len(a.Store.Messages) {
		t.Errorf("session has %d messages, store %d", len(msgs), len(a.Store.Messages))
	}
	var toolCalls, toolResults, dones int
	for _, ev := range events {
		switch ev.Type {
		case "tool_call":
			toolCalls++
		case "tool_result":
			toolResults++
		case "done":
			dones++
		}
	}
	if toolCalls != 2 || toolResults != 2 || dones != 1 {
		t.Errorf("events: calls=%d results=%d done=%d", toolCalls, toolResults, dones)
	}
	found := false
	for _, m := range a.Store.Messages {
		for _, p := range m.Parts {
			if p.Type == "tool_result" && strings.Contains(p.Content, "example.com/x") {
				found = true
			}
		}
	}
	if !found {
		t.Error("tool_result with file contents not found in store")
	}
}

func TestAgentMaxTurnsGuard(t *testing.T) {
	steps := []testStep{
		{tool: "read", args: map[string]any{"file_path": "go.mod"}},
		{tool: "read", args: map[string]any{"file_path": "go.mod"}},
	}
	a, _, _ := setupAgent(t, steps, 1)
	_, err := a.Run(context.Background(), "loop forever")
	if err == nil || !strings.Contains(err.Error(), "max turns") {
		t.Errorf("expected max turns error, got %v", err)
	}
}

func TestAgentUnknownTool(t *testing.T) {
	steps := []testStep{
		{tool: "nonexistent", args: map[string]any{}},
		{text: "recovered"},
	}
	a, _, _ := setupAgent(t, steps, 5)
	final, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if final != "recovered" {
		t.Errorf("final = %q", final)
	}
	foundErr := false
	for _, m := range a.Store.Messages {
		for _, p := range m.Parts {
			if p.Type == "tool_result" && p.IsError && strings.Contains(p.Content, "unknown tool") {
				foundErr = true
			}
		}
	}
	if !foundErr {
		t.Error("expected error tool_result for unknown tool")
	}
}

func TestAgentPermissionDeny(t *testing.T) {
	steps := []testStep{
		{tool: "bash", args: map[string]any{"command": "echo hi"}},
		{text: "bash was denied"},
	}
	a, _, _ := setupAgent(t, steps, 5)
	a.Policy = tools.NewPolicy(tools.ModePlan, nil)
	final, err := a.Run(context.Background(), "run echo")
	if err != nil {
		t.Fatal(err)
	}
	if final != "bash was denied" {
		t.Errorf("final = %q", final)
	}
	denied := false
	for _, m := range a.Store.Messages {
		for _, p := range m.Parts {
			if p.Type == "tool_result" && p.IsError && strings.Contains(p.Content, "denied") {
				denied = true
			}
		}
	}
	if !denied {
		t.Error("expected denied tool_result in plan mode")
	}
}

// TestAgentSSERoundTrip verifies the real HTTP/SSE provider parses streamed
// tool calls and text correctly (this is what the test server above serves).
func TestAgentSSERoundTrip(t *testing.T) {
	p := newTestProvider(t, []testStep{{text: "hello"}, {tool: "read", args: map[string]any{"file_path": "x"}}})
	ch, err := p.StreamChat(context.Background(), []llm.ChatMessage{{Role: "user", Content: "hi"}}, nil, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	var texts, calls int
	stop := ""
	for ev := range ch {
		switch ev.Type {
		case "text":
			texts++
		case "tool_call":
			calls++
		case "finish":
			stop = ev.StopReason
		}
	}
	if texts != 1 || stop != "end_turn" {
		t.Errorf("first call: texts=%d stop=%s", texts, stop)
	}
	ch, err = p.StreamChat(context.Background(), []llm.ChatMessage{{Role: "user", Content: "again"}}, nil, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	calls = 0
	stop = ""
	for ev := range ch {
		if ev.Type == "tool_call" {
			calls++
			if ev.Call == nil || ev.Call.Name != "read" {
				t.Errorf("tool call = %+v", ev.Call)
			}
		}
		if ev.Type == "finish" {
			stop = ev.StopReason
		}
	}
	if calls != 1 || stop != "tool_use" {
		t.Errorf("second call: calls=%d stop=%s", calls, stop)
	}
}
