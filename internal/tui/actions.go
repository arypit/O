package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// actionInfo describes one tool call in plain language. The TUI shows this
// instead of the raw tool name, JSON payload or backend structure.
type actionInfo struct {
	Title   string // "Reading a file"
	Detail  string // "internal/tui/model.go"
	Why     string // why the agent needs this
	Level   string // "read access · workspace files"
	Summary string // one-line summary for the transcript
}

// decodeArgs turns the raw tool arguments of an event into a map.
func decodeArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil
	}
	return args
}

func argString(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argInt(args map[string]any, key string) int {
	if args == nil {
		return 0
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return 0
}

// describeAction translates a tool call into a human-readable request.
func describeAction(tool string, args map[string]any) actionInfo {
	switch tool {
	case "read":
		p := argString(args, "file_path")
		return actionInfo{
			Title:   "Reading a file",
			Detail:  p,
			Why:     "The agent needs the contents of this file to continue.",
			Level:   "read access · workspace files",
			Summary: "Reading " + shortPath(p),
		}
	case "write":
		p := argString(args, "file_path")
		return actionInfo{
			Title:   "Writing a file",
			Detail:  p,
			Why:     "The agent is creating or replacing this file.",
			Level:   "write access · changes files on disk",
			Summary: "Writing " + shortPath(p),
		}
	case "edit":
		p := argString(args, "file_path")
		return actionInfo{
			Title:   "Editing a file",
			Detail:  p,
			Why:     "The agent is changing the contents of this file.",
			Level:   "write access · changes files on disk",
			Summary: "Editing " + shortPath(p),
		}
	case "bash":
		c := argString(args, "command")
		return actionInfo{
			Title:   "Running a shell command",
			Detail:  c,
			Why:     "The agent asked to run this command in your workspace.",
			Level:   "shell access · can change anything the user can",
			Summary: "Running `" + clipText(c, 90) + "`",
		}
	case "grep":
		pat := argString(args, "pattern")
		where := argString(args, "path")
		if where == "" {
			where = "the workspace"
		}
		return actionInfo{
			Title:   "Searching the code",
			Detail:  fmt.Sprintf("%s in %s", pat, shortPath(where)),
			Why:     "The agent is looking for lines that match a pattern.",
			Level:   "read access · workspace files",
			Summary: fmt.Sprintf("Searching for %q in %s", clipText(pat, 60), shortPath(where)),
		}
	case "glob":
		pat := argString(args, "pattern")
		where := argString(args, "path")
		if where == "" {
			where = "the workspace"
		}
		return actionInfo{
			Title:   "Finding files",
			Detail:  fmt.Sprintf("%s in %s", pat, shortPath(where)),
			Why:     "The agent is matching files by name.",
			Level:   "read access · workspace files",
			Summary: fmt.Sprintf("Finding files matching %s", clipText(pat, 60)),
		}
	case "webfetch":
		u := argString(args, "url")
		return actionInfo{
			Title:   "Fetching a web page",
			Detail:  u,
			Why:     "The agent needs the contents of this page.",
			Level:   "network access · sends a request to this address",
			Summary: "Fetching " + clipText(u, 90),
		}
	case "todo":
		n := 0
		if list, ok := args["todos"].([]any); ok {
			n = len(list)
		}
		return actionInfo{
			Title:   "Updating the task list",
			Detail:  fmt.Sprintf("%d item(s)", n),
			Why:     "The agent keeps the plan for this request visible.",
			Level:   "no file access",
			Summary: "Updating the task list",
		}
	case "plan":
		g := argString(args, "goal")
		return actionInfo{
			Title:   "Planning the work",
			Detail:  g,
			Why:     "The agent works out an approach before acting.",
			Level:   "no file access",
			Summary: "Planning: " + clipText(g, 80),
		}
	case "task":
		d := argString(args, "description")
		agent := argString(args, "agent")
		if agent == "" {
			agent = "sub-agent"
		}
		return actionInfo{
			Title:   "Running a sub-agent",
			Detail:  d,
			Why:     "The agent delegated part of the work.",
			Level:   "inherits the agent's permissions",
			Summary: "Delegating " + clipText(d, 70) + " to " + agent,
		}
	default:
		return actionInfo{
			Title:   "Running " + tool,
			Detail:  "",
			Why:     "The agent requested this action.",
			Level:   "access level depends on the tool",
			Summary: "Running " + tool,
		}
	}
}

// activityLabel maps the current operation to the live activity label shown
// in the status bar. It follows the real stage of the run.
func activityLabel(tool string) string {
	switch tool {
	case "read":
		return "Reading files"
	case "write":
		return "Creating a file"
	case "edit":
		return "Writing code"
	case "bash":
		return "Executing command"
	case "grep":
		return "Searching the code"
	case "glob":
		return "Looking for files"
	case "webfetch":
		return "Fetching a page"
	case "todo":
		return "Planning"
	case "plan":
		return "Planning"
	case "task":
		return "Running a sub-agent"
	default:
		return "Working"
	}
}

// resultSummary turns a tool result into one readable line.
func resultSummary(tool string, args map[string]any, content string) string {
	trimmed := strings.TrimSpace(content)
	switch tool {
	case "read":
		return "Read " + shortPath(argString(args, "file_path"))
	case "write":
		return "Wrote " + shortPath(argString(args, "file_path"))
	case "edit":
		return "Applied the edit to " + shortPath(argString(args, "file_path"))
	case "bash":
		// the summary stays in plain language: the raw output of the
		// command belongs to the expanded block, not to the transcript
		return "Command finished"
	case "grep":
		return "Search finished"
	case "glob":
		return "Found " + fmt.Sprintf("%d file(s)", countLines(trimmed))
	case "webfetch":
		return "Fetched the page"
	case "todo":
		return "Task list updated"
	case "plan":
		return "Plan ready"
	case "task":
		return "Sub-agent finished"
	default:
		if trimmed == "" {
			return "Done"
		}
		return clipText(oneLine(trimmed, 100), 100)
	}
}

// countLines counts the non-empty lines of s.
func countLines(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// shortPath keeps the tail of a path so it fits on one line.
func shortPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "the workspace"
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if len(p) <= 60 {
		return p
	}
	return "…" + p[len(p)-59:]
}

// clipText shortens s to n runes.
func clipText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
