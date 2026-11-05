package builtin

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/adaop/o/internal/tools"
)

// GrepTool searches file contents with a regular expression.
type GrepTool struct{}

func (GrepTool) Name() string { return "grep" }
func (GrepTool) Description() string {
	return "Search file contents with a Go regular expression. Returns file:line: match lines."
}
func (GrepTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern":     map[string]any{"type": "string", "description": "Go regexp"},
			"path":        map[string]any{"type": "string", "description": "File or directory (default workspace)"},
			"include":     map[string]any{"type": "string", "description": "Optional glob to limit files, e.g. *.go"},
			"max_matches": map[string]any{"type": "integer"},
		},
		"required":             []any{"pattern"},
		"additionalProperties": false,
	}
}
func (GrepTool) Mutating() bool { return false }
func (t GrepTool) Summarize(args map[string]any) string {
	s, _ := args["pattern"].(string)
	return s
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "dist": true, "bin": true}

func (t GrepTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("bad regexp: %v", err), IsError: true}, nil
	}
	root, _ := args["path"].(string)
	if root == "" {
		root = "."
	}
	absRoot, err := ec.SB.Resolve(root)
	if err != nil {
		return tools.ToolResult{}, err
	}
	include, _ := args["include"].(string)
	var includeRe *regexp.Regexp
	if include != "" {
		includeRe, err = globToRegexp(filepath.ToSlash(include))
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("bad include glob: %v", err), IsError: true}, nil
		}
	}
	maxMatches := 100
	if m, ok := args["max_matches"].(float64); ok && m > 0 {
		maxMatches = int(m)
	}
	var out []string
	count := 0
	filepath.WalkDir(absRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || count >= maxMatches {
			return filepath.SkipAll
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 5*1024*1024 {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, p)
		relSlash := filepath.ToSlash(rel)
		if includeRe != nil && !includeRe.MatchString(relSlash) {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		// skip binary files: look for NUL in the first 8KB
		head := make([]byte, 8192)
		n, _ := f.Read(head)
		if bytes.IndexByte(head[:n], 0) >= 0 {
			return nil
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			text := sc.Text()
			if re.MatchString(text) {
				out = append(out, fmt.Sprintf("%s:%d: %s", relSlash, lineNo, text))
				count++
				if count >= maxMatches {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if count == 0 {
		return tools.ToolResult{Content: fmt.Sprintf("no matches for %s", pattern)}, nil
	}
	res := fmt.Sprintf("%d match(es):\n%s", count, strings.Join(out, "\n"))
	return tools.Truncate(ec, res), nil
}
