package context

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/llm"
)

// newTestProvider is a real local HTTP server answering non-streamed
// completions (used for compaction summaries). Test-only; no mock ships.
func newTestProvider(t *testing.T) llm.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "test summary"}}},
		})
		fmt.Fprint(w)
	}))
	t.Cleanup(srv.Close)
	return llm.NewOpenAI("test", srv.URL, "")
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("abcd"); got != 1 {
		t.Errorf("4 chars = %d tokens, want 1", got)
	}
	if got := EstimateTokens(strings.Repeat("x", 400)); got != 100 {
		t.Errorf("400 chars = %d tokens, want 100", got)
	}
}

func TestChatMessagesConversion(t *testing.T) {
	store := NewStore(
		core.NewMessage(core.RoleUser, core.TextPart("hi")),
		core.NewMessage(core.RoleAssistant,
			core.TextPart("let me check"),
			core.Part{Type: "tool_use", ID: "c1", Name: "read", Input: []byte(`{"file_path":"a.go"}`)},
		),
		core.NewMessage(core.RoleTool, core.Part{Type: "tool_result", ToolUseID: "c1", Content: "file contents"}),
	)
	msgs := store.ChatMessages("sys")
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "sys" {
		t.Errorf("system = %+v", msgs[0])
	}
	if msgs[2].ToolCalls[0].Name != "read" {
		t.Errorf("assistant tool call = %+v", msgs[2])
	}
	if msgs[3].Role != "tool" || msgs[3].ToolCallID != "c1" || msgs[3].Content != "file contents" {
		t.Errorf("tool msg = %+v", msgs[3])
	}
}

func TestMaybeCompact(t *testing.T) {
	store := NewStore()
	for i := 0; i < 20; i++ {
		store.Add(core.NewMessage(core.RoleUser, core.TextPart(strings.Repeat("word ", 50))))
	}
	prov := newTestProvider(t)
	compacted, err := store.MaybeCompact(context.Background(), prov, "m", 200, 0.8, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !compacted {
		t.Fatal("expected compaction to trigger")
	}
	if len(store.Messages) != 5 { // 1 summary + 4 kept
		t.Errorf("messages after compact = %d, want 5", len(store.Messages))
	}
	if store.Messages[0].Parts[0].Type != "summary" || store.Messages[0].Parts[0].Text != "test summary" {
		t.Errorf("first message = %+v", store.Messages[0])
	}
	// below threshold: no compaction
	store2 := NewStore(core.NewMessage(core.RoleUser, core.TextPart("hi")))
	compacted, _ = store2.MaybeCompact(context.Background(), prov, "m", 100000, 0.8, 4)
	if compacted {
		t.Error("should not compact below threshold")
	}
}
