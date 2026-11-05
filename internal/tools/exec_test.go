package tools

import (
	"context"
	"testing"

	"github.com/adaop/o/internal/llm"
)

// panickingTool panics inside Execute; the pipeline must turn that into an
// error tool_result instead of crashing the process.
type panickingTool struct{}

func (panickingTool) Name() string        { return "boom" }
func (panickingTool) Description() string { return "always panics" }
func (panickingTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (panickingTool) Mutating() bool { return false }
func (panickingTool) Summarize(args map[string]any) string {
	return "boom()"
}
func (panickingTool) Execute(ctx context.Context, args map[string]any, ec *ExecContext) (ToolResult, error) {
	panic("kaboom")
}

func TestExecuteCallRecoversFromToolPanic(t *testing.T) {
	reg := NewRegistry()
	reg.Add(panickingTool{})
	policy := NewPolicy(ModeFullAuto, nil)
	call := llm.ToolCall{ID: "c1", Name: "boom", Arguments: "{}"}
	part := ExecuteCall(context.Background(), reg, policy, call, &ExecContext{MaxToolOutput: 1000})
	if part.Type != "tool_result" || !part.IsError {
		t.Fatalf("part = %+v, want error tool_result", part)
	}
	if part.ToolUseID != "c1" {
		t.Errorf("ToolUseID = %q", part.ToolUseID)
	}
}
