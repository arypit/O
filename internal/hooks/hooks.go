// Package hooks runs shell commands on agent events (PreToolUse,
// PostToolUse, Stop, SessionStart). A PreToolUse hook exits non-zero to
// block the call; its stderr becomes the tool result.
package hooks

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
)

type Hooks struct {
	// Events maps event name -> list of shell commands.
	Events map[string][]string
}

func New(events map[string][]string) *Hooks {
	return &Hooks{Events: events}
}

func shellCommand(cmd string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", cmd}
	}
	return "sh", []string{"-c", cmd}
}

// Run executes the hooks for event with the given environment additions.
// Returns an error (with output) if any hook fails.
func (h *Hooks) Run(event string, env map[string]string) error {
	if h == nil {
		return nil
	}
	for _, cmd := range h.Events[event] {
		if cmd == "" {
			continue
		}
		name, args := shellCommand(cmd)
		c := exec.Command(name, args...)
		for k, v := range env {
			c.Env = append(c.Env, k+"="+v)
		}
		var out bytes.Buffer
		c.Stdout = &out
		c.Stderr = &out
		if err := c.Run(); err != nil {
			return fmt.Errorf("hook %s (%s) failed: %w: %s", event, cmd, err, out.String())
		}
	}
	return nil
}
