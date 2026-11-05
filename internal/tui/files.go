package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/adaop/o/internal/config"
)

// walkUpFind searches cwd and its parents for a file named name.
func walkUpFind(cwd, name string) string {
	dir := cwd
	for {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// readVersion returns the O version for cwd (VERSION file, then config).
func readVersion(cwd string) string {
	if p := walkUpFind(cwd, "VERSION"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if v := strings.TrimSpace(string(b)); v != "" {
				return v
			}
		}
	}
	if cfg, err := config.Load(cwd); err == nil && cfg.Version != "" {
		return cfg.Version
	}
	return "0.0.0"
}

// loadChangelog reads CHANGELOG.md walking up from cwd.
func loadChangelog(cwd string) []string {
	p := walkUpFind(cwd, "CHANGELOG.md")
	if p == "" {
		return []string{"no CHANGELOG.md found walking up from " + cwd}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return []string{"error reading " + p + ": " + err.Error()}
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}
