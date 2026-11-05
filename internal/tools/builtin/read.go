package builtin

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/adaop/o/internal/tools"
)

// ReadTool reads a file (optionally a line range).
type ReadTool struct{}

func (ReadTool) Name() string { return "read" }
func (ReadTool) Description() string {
	return "Read a file from the local filesystem. Use offset/limit for large files."
}
func (ReadTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "Absolute or workspace-relative path"},
			"offset":    map[string]any{"type": "integer", "description": "1-based line to start from"},
			"limit":     map[string]any{"type": "integer", "description": "Max lines to return"},
		},
		"required":             []any{"file_path"},
		"additionalProperties": false,
	}
}
func (ReadTool) Mutating() bool { return false }
func (t ReadTool) Summarize(args map[string]any) string {
	s, _ := args["file_path"].(string)
	return s
}

func (t ReadTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	path, _ := args["file_path"].(string)
	abs, err := ec.SB.Resolve(path)
	if err != nil {
		return tools.ToolResult{}, err
	}
	if err := ec.SB.AllowRead(abs); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("read %s: %v", path, err), IsError: true}, nil
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	offset, _ := args["offset"].(float64)
	limit, _ := args["limit"].(float64)
	if offset > 0 || limit > 0 {
		start := int(offset)
		if start < 1 {
			start = 1
		}
		end := len(lines)
		if limit > 0 && start-1+int(limit) < end {
			end = start - 1 + int(limit)
		}
		if start > len(lines) {
			start = len(lines)
		}
		content = fmt.Sprintf("lines %d-%d of %d\n", start, end, len(lines)) + strings.Join(lines[start-1:end], "\n")
	}
	return tools.Truncate(ec, content), nil
}
