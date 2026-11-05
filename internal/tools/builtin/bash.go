package builtin

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/adaop/o/internal/tools"
)

// BashTool runs a shell command with a timeout. Blocked in read-only sandbox mode.
type BashTool struct{}

func (BashTool) Name() string { return "bash" }
func (BashTool) Description() string {
	return "Run a shell command in the workspace. Output is captured and truncated. Use for builds, tests, git, etc."
}
func (BashTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":         map[string]any{"type": "string"},
			"timeout_seconds": map[string]any{"type": "integer", "description": "default 120, max 600"},
		},
		"required":             []any{"command"},
		"additionalProperties": false,
	}
}
func (BashTool) Mutating() bool { return true }
func (t BashTool) Summarize(args map[string]any) string {
	s, _ := args["command"].(string)
	return s
}

func (t BashTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	command, _ := args["command"].(string)
	if err := ec.SB.AllowExec(); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	timeout := 120
	if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
		timeout = int(v)
		if timeout > 600 {
			timeout = 600
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	var name string
	var shArgs []string
	if runtime.GOOS == "windows" {
		name, shArgs = "cmd", []string{"/C", command}
	} else {
		name, shArgs = "sh", []string{"-c", command}
	}
	cmd := exec.CommandContext(ctx, name, shArgs...)
	cmd.Dir = ec.CWD
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	result := out.String()
	if ctx.Err() == context.DeadlineExceeded {
		result += fmt.Sprintf("\n[timed out after %ds]", timeout)
		return tools.Truncate(ec, result), nil
	}
	if err != nil {
		result += fmt.Sprintf("\n[exit error: %v]", err)
	}
	return tools.Truncate(ec, result), nil
}
