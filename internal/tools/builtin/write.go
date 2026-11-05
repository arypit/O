package builtin

import (
	"context"
	"fmt"
	"os"

	"github.com/adaop/o/internal/sandbox"
	"github.com/adaop/o/internal/tools"
)

// WriteTool creates or overwrites a file.
type WriteTool struct{}

func (WriteTool) Name() string { return "write" }
func (WriteTool) Description() string {
	return "Write content to a file, creating it if needed. Fails if the file already exists unless the model first read it."
}
func (WriteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string"},
			"content":   map[string]any{"type": "string"},
		},
		"required":             []any{"file_path", "content"},
		"additionalProperties": false,
	}
}
func (WriteTool) Mutating() bool { return true }
func (t WriteTool) Summarize(args map[string]any) string {
	s, _ := args["file_path"].(string)
	return s
}

func (t WriteTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	path, _ := args["file_path"].(string)
	content, _ := args["content"].(string)
	abs, err := ec.SB.Resolve(path)
	if err != nil {
		return tools.ToolResult{}, err
	}
	if err := ec.SB.AllowWrite(abs); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := sandbox.EnsureDir(abs); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("write %s: %v", path, err), IsError: true}, nil
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("write %s: %v", path, err), IsError: true}, nil
	}
	return tools.ToolResult{Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}
