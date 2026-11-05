package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/adaop/o/internal/tools"
)

// GlobTool finds files by glob pattern, supporting * ? and ** (any depth).
type GlobTool struct{}

func (GlobTool) Name() string { return "glob" }
func (GlobTool) Description() string {
	return "Find files matching a glob pattern. '*' matches within a path segment, '**' matches any depth."
}
func (GlobTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "e.g. **/*.go or src/**/*.ts"},
			"path":    map[string]any{"type": "string", "description": "Directory to search in (default workspace)"},
		},
		"required":             []any{"pattern"},
		"additionalProperties": false,
	}
}
func (GlobTool) Mutating() bool { return false }
func (t GlobTool) Summarize(args map[string]any) string {
	s, _ := args["pattern"].(string)
	return s
}

// globToRegexp converts a glob with ** support to a regex over slash-separated paths.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	i := 0
	for i < len(pattern) {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// '**' crosses directories; '**/' also matches zero directories
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 3
				} else {
					b.WriteString(".*")
					i += 2
				}
			} else {
				b.WriteString("[^/]*")
				i++
			}
		case '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

func (t GlobTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	root, _ := args["path"].(string)
	if root == "" {
		root = "."
	}
	absRoot, err := ec.SB.Resolve(root)
	if err != nil {
		return tools.ToolResult{}, err
	}
	re, err := globToRegexp(filepath.ToSlash(pattern))
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("bad pattern: %v", err), IsError: true}, nil
	}
	var matches []string
	filepath.WalkDir(absRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "dist" || d.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(absRoot, p)
		if err != nil {
			return nil
		}
		if re.MatchString(filepath.ToSlash(rel)) {
			matches = append(matches, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(matches)
	if len(matches) > 1000 {
		matches = matches[:1000]
	}
	if len(matches) == 0 {
		return tools.ToolResult{Content: fmt.Sprintf("no files match %s", pattern)}, nil
	}
	return tools.Truncate(ec, strings.Join(matches, "\n")), nil
}
