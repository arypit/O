package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/tools"
)

// TodoTool tracks the multi-step plan (todowrite/todoread).
type TodoTool struct{}

func (TodoTool) Name() string { return "todo" }
func (TodoTool) Description() string {
	return "Read or replace the session todo list. Use it to track multi-step work."
}
func (TodoTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []any{"read", "write"}},
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content":    map[string]any{"type": "string"},
						"status":     map[string]any{"type": "string", "enum": []any{"pending", "in_progress", "completed"}},
						"activeForm": map[string]any{"type": "string"},
					},
					"required": []any{"content", "status"},
				},
			},
		},
		"required":             []any{"action"},
		"additionalProperties": false,
	}
}
func (TodoTool) Mutating() bool { return false }
func (t TodoTool) Summarize(args map[string]any) string {
	a, _ := args["action"].(string)
	return a
}

func (t TodoTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	action, _ := args["action"].(string)
	if action == "write" {
		raw, _ := args["todos"].([]any)
		items := make([]core.TodoItem, 0, len(raw))
		for _, it := range raw {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			item := core.TodoItem{}
			item.Content, _ = m["content"].(string)
			item.Status, _ = m["status"].(string)
			item.ActiveForm, _ = m["activeForm"].(string)
			items = append(items, item)
		}
		ec.Todos.Set(items)
		if ec.Emit != nil {
			ec.Emit(core.AgentEvent{Type: "todo", Todos: ec.Todos.Items})
		}
		return tools.ToolResult{Content: fmt.Sprintf("todo list updated (%d items)", len(items))}, nil
	}
	var b strings.Builder
	for i, item := range ec.Todos.Items {
		mark := "[ ]"
		if item.Status == "completed" {
			mark = "[x]"
		} else if item.Status == "in_progress" {
			mark = "[~]"
		}
		fmt.Fprintf(&b, "%d. %s %s\n", i+1, mark, item.Content)
	}
	if b.Len() == 0 {
		return tools.ToolResult{Content: "todo list is empty"}, nil
	}
	return tools.ToolResult{Content: b.String()}, nil
}
