// Package subagent provides the "task" tool: spawn a bounded child agent
// with its own context, smaller tool set and turn limit, and return its
// final answer as the tool result.
package subagent

import (
	"context"
	"fmt"

	"github.com/adaop/o/internal/agent"
	"github.com/adaop/o/internal/tools"
)

// Factory builds a child agent for a named subagent role.
type Factory func(name, prompt string) (*agent.Agent, error)

type TaskTool struct {
	Factory Factory
}

func (TaskTool) Name() string { return "task" }
func (TaskTool) Description() string {
	return "Delegate a self-contained sub-task to a bounded subagent and get its final answer back. Use for exploration, research or review."
}
func (TaskTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent":       map[string]any{"type": "string", "description": "subagent name, e.g. planner or reviewer"},
			"description": map[string]any{"type": "string", "description": "one-line description of the task"},
			"prompt":      map[string]any{"type": "string", "description": "full instructions for the subagent"},
		},
		"required":             []any{"description", "prompt"},
		"additionalProperties": false,
	}
}
func (TaskTool) Mutating() bool { return false }
func (t TaskTool) Summarize(args map[string]any) string {
	d, _ := args["description"].(string)
	return d
}

func (t TaskTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	if ec.Depth >= 2 {
		return tools.ToolResult{Content: "subagent depth limit reached (max depth 2)", IsError: true}, nil
	}
	name, _ := args["agent"].(string)
	prompt, _ := args["prompt"].(string)
	child, err := t.Factory(name, prompt)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("cannot spawn subagent: %v", err), IsError: true}, nil
	}
	out, err := child.Run(ctx, prompt)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("subagent %s failed: %v\npartial output: %s", name, err, out), IsError: true}, nil
	}
	return tools.ToolResult{Content: fmt.Sprintf("subagent %s result:\n%s", name, out)}, nil
}
