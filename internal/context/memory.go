package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adaop/o/internal/config"
	"github.com/adaop/o/internal/core"
)

// LoadMemoryFiles reads the O.md memory chain: user-level ~/.o/O.md,
// project ./O.md and local ./O.md.local. Missing files are skipped.
func LoadMemoryFiles(cwd, home string, name string) []string {
	if name == "" {
		name = "O.md"
	}
	var out []string
	candidates := []string{
		filepath.Join(home, ".o", name),
		filepath.Join(cwd, name),
		filepath.Join(cwd, name+".local"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		out = append(out, fmt.Sprintf("--- memory file: %s ---\n%s", p, strings.TrimSpace(string(data))))
	}
	return out
}

var snapshotSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "bin": true, ".o": true, "vendor": true,
}

// RepoSnapshot lists the workspace file tree (capped) plus git info.
func RepoSnapshot(cwd string, maxFiles int) string {
	var files []string
	filepath.WalkDir(cwd, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != cwd && snapshotSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(cwd, p)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	var sb strings.Builder
	sb.WriteString("workspace: " + cwd + "\n")
	if branch := gitOutput(cwd, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		sb.WriteString("git branch: " + branch + "\n")
	}
	if st := gitOutput(cwd, "status", "--porcelain"); st != "" {
		lines := strings.Split(strings.TrimSpace(st), "\n")
		if len(lines) > 20 {
			lines = lines[:20]
		}
		sb.WriteString("git status:\n" + strings.Join(lines, "\n") + "\n")
	}
	sb.WriteString("files:\n" + strings.Join(files, "\n") + "\n")
	return sb.String()
}

func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// BuildSystemPrompt assembles the stable system prompt prefix: identity,
// environment, memory files, repo snapshot and current todos.
func BuildSystemPrompt(cfg config.Config, cwd, home string, todos []core.TodoItem, now time.Time) string {
	return BuildSystemPromptWith(cfg, cwd, home, todos, now, true)
}

// BuildSystemPromptWith assembles the system prompt. When projectContext is
// false the memory files and the repository snapshot are left out, so the
// agent starts from the request alone.
func BuildSystemPromptWith(cfg config.Config, cwd, home string, todos []core.TodoItem, now time.Time, projectContext bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "You are O, an autonomous coding agent built by %s (adaop.sbs). Version %s.\n", cfg.Company, cfg.Version)
	fmt.Fprintf(&sb, "Today is %s. OS: %s. Working directory: %s\n", now.Format("2006-01-02"), runtimeOS(), cwd)
	sb.WriteString("You have tools: read, write, edit, glob, grep, bash, todo, webfetch, plan, task, and any MCP tools. ")
	sb.WriteString("Work step by step, use the todo tool for multi-step tasks, and keep answers concise.\n")
	if len(todos) > 0 {
		sb.WriteString("\nCurrent todos:\n")
		for i, t := range todos {
			fmt.Fprintf(&sb, "%d. [%s] %s\n", i+1, t.Status, t.Content)
		}
	}
	if !projectContext {
		return sb.String()
	}
	mem := LoadMemoryFiles(cwd, home, cfg.Paths.MemoryFile)
	if len(mem) > 0 {
		sb.WriteString("\n" + strings.Join(mem, "\n\n") + "\n")
	}
	sb.WriteString("\n" + RepoSnapshot(cwd, 500))
	return sb.String()
}

func runtimeOS() string {
	return strings.ToUpper(os.Getenv("OS"))
}
