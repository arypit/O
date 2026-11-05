package builtin

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/adaop/o/internal/sandbox"
	"github.com/adaop/o/internal/tools"
)

// EditTool does exact string replacement in a file.
// old_string must occur exactly once unless replace_all is set; this avoids
// ambiguous edits.
type EditTool struct{}

func (EditTool) Name() string { return "edit" }
func (EditTool) Description() string {
	return "Replace an exact string in a file. old_string must be unique unless replace_all=true."
}
func (EditTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path":   map[string]any{"type": "string"},
			"old_string":  map[string]any{"type": "string"},
			"new_string":  map[string]any{"type": "string"},
			"replace_all": map[string]any{"type": "boolean"},
		},
		"required":             []any{"file_path", "old_string", "new_string"},
		"additionalProperties": false,
	}
}
func (EditTool) Mutating() bool { return true }
func (t EditTool) Summarize(args map[string]any) string {
	s, _ := args["file_path"].(string)
	return s
}

func (t EditTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	path, _ := args["file_path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)
	replaceAll, _ := args["replace_all"].(bool)
	abs, err := ec.SB.Resolve(path)
	if err != nil {
		return tools.ToolResult{}, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("edit %s: cannot read: %v", path, err), IsError: true}, nil
	}
	content := string(data)
	count := strings.Count(content, oldStr)
	switch {
	case count == 0:
		return tools.ToolResult{Content: fmt.Sprintf("edit %s: old_string not found", path), IsError: true}, nil
	case count > 1 && !replaceAll:
		return tools.ToolResult{Content: fmt.Sprintf("edit %s: old_string occurs %d times; set replace_all=true to replace all", path, count), IsError: true}, nil
	}
	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		updated = strings.Replace(content, oldStr, newStr, 1)
	}
	if err := ec.SB.AllowWrite(abs); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := sandbox.EnsureDir(abs); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := os.WriteFile(abs, []byte(updated), 0o644); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("edit %s: %v", path, err), IsError: true}, nil
	}
	return tools.ToolResult{Content: fmt.Sprintf("edited %s (%d occurrence(s) replaced)", path, count)}, nil
}
