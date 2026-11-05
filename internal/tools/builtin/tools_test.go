package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/sandbox"
	"github.com/adaop/o/internal/tools"
)

func testEC(t *testing.T, mode sandbox.Mode) (*tools.ExecContext, string) {
	t.Helper()
	dir := t.TempDir()
	sb, err := sandbox.New(dir, mode)
	if err != nil {
		t.Fatal(err)
	}
	ec := &tools.ExecContext{
		SessionID:     "test",
		CWD:           dir,
		SB:            sb,
		Todos:         &core.TodoStore{},
		MaxToolOutput: 20000,
		AllowOnce:     tools.NewAllowSet(),
	}
	return ec, dir
}

func TestWriteReadRoundtrip(t *testing.T) {
	ec, dir := testEC(t, sandbox.WorkspaceWrite)
	w := WriteTool{}
	res, err := w.Execute(context.Background(), map[string]any{"file_path": "sub/a.txt", "content": "hello"}, ec)
	if err != nil || res.IsError {
		t.Fatalf("write failed: %v %v", res, err)
	}
	r := ReadTool{}
	res, err = r.Execute(context.Background(), map[string]any{"file_path": filepath.Join(dir, "sub", "a.txt")}, ec)
	if err != nil || res.Content != "hello" {
		t.Fatalf("read = %q, %v", res.Content, err)
	}
}

func TestWriteOutsideWorkspaceDenied(t *testing.T) {
	ec, dir := testEC(t, sandbox.WorkspaceWrite)
	res, _ := WriteTool{}.Execute(context.Background(), map[string]any{"file_path": "../evil.txt", "content": "x"}, ec)
	if !res.IsError {
		t.Error("write outside workspace must be an error result")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil.txt")); err == nil {
		t.Error("file must not have been created outside workspace")
	}
}

func TestEditExactReplace(t *testing.T) {
	ec, dir := testEC(t, sandbox.WorkspaceWrite)
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("a := 1\nb := 2\na := 1\n"), 0o644)
	e := EditTool{}
	// not found
	res, _ := e.Execute(context.Background(), map[string]any{"file_path": p, "old_string": "zzz", "new_string": "q"}, ec)
	if !res.IsError {
		t.Error("missing old_string must error")
	}
	// ambiguous without replace_all
	res, _ = e.Execute(context.Background(), map[string]any{"file_path": p, "old_string": "a := 1", "new_string": "a := 9"}, ec)
	if !res.IsError || !strings.Contains(res.Content, "2 times") {
		t.Errorf("ambiguous edit must error with count, got %q", res.Content)
	}
	// replace_all
	res, _ = e.Execute(context.Background(), map[string]any{"file_path": p, "old_string": "a := 1", "new_string": "a := 9", "replace_all": true}, ec)
	if res.IsError {
		t.Fatalf("replace_all failed: %q", res.Content)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "a := 9\nb := 2\na := 9\n" {
		t.Errorf("content = %q", string(data))
	}
	// unique replace
	os.WriteFile(p, []byte("x\ny\n"), 0o644)
	res, _ = e.Execute(context.Background(), map[string]any{"file_path": p, "old_string": "x", "new_string": "z"}, ec)
	if res.IsError {
		t.Fatalf("unique edit failed: %q", res.Content)
	}
	data, _ = os.ReadFile(p)
	if string(data) != "z\ny\n" {
		t.Errorf("content = %q", string(data))
	}
}

func TestEditReadOnlyDenied(t *testing.T) {
	ec, dir := testEC(t, sandbox.ReadOnly)
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("a\n"), 0o644)
	res, _ := EditTool{}.Execute(context.Background(), map[string]any{"file_path": p, "old_string": "a", "new_string": "b"}, ec)
	if !res.IsError {
		t.Error("edit must be denied in read-only sandbox")
	}
}

func TestGlob(t *testing.T) {
	ec, dir := testEC(t, sandbox.WorkspaceWrite)
	os.MkdirAll(filepath.Join(dir, "src", "deep"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "src", "deep", "b.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "src", "c.ts"), []byte("x"), 0o644)
	res, err := GlobTool{}.Execute(context.Background(), map[string]any{"pattern": "**/*.go"}, ec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "src/a.go") || !strings.Contains(res.Content, "src/deep/b.go") {
		t.Errorf("glob **/*.go = %q", res.Content)
	}
	if strings.Contains(res.Content, "c.ts") {
		t.Error("glob must not match c.ts")
	}
	res, _ = GlobTool{}.Execute(context.Background(), map[string]any{"pattern": "src/*.ts"}, ec)
	if !strings.Contains(res.Content, "src/c.ts") {
		t.Errorf("glob src/*.ts = %q", res.Content)
	}
}

func TestGrep(t *testing.T) {
	ec, dir := testEC(t, sandbox.WorkspaceWrite)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	res, err := GrepTool{}.Execute(context.Background(), map[string]any{"pattern": "func main", "include": "*.go"}, ec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "main.go:2: func main() {}") {
		t.Errorf("grep = %q", res.Content)
	}
}

func TestBashReadOnlyDenied(t *testing.T) {
	ec, _ := testEC(t, sandbox.ReadOnly)
	res, _ := BashTool{}.Execute(context.Background(), map[string]any{"command": "echo hi"}, ec)
	if !res.IsError {
		t.Error("bash must be denied in read-only sandbox")
	}
}

func TestBashRuns(t *testing.T) {
	ec, _ := testEC(t, sandbox.WorkspaceWrite)
	res, err := BashTool{}.Execute(context.Background(), map[string]any{"command": "echo hello-o"}, ec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "hello-o") {
		t.Errorf("bash output = %q", res.Content)
	}
}

func TestTodo(t *testing.T) {
	ec, _ := testEC(t, sandbox.WorkspaceWrite)
	tt := TodoTool{}
	res, _ := tt.Execute(context.Background(), map[string]any{"action": "write", "todos": []any{
		map[string]any{"content": "step one", "status": "in_progress"},
		map[string]any{"content": "step two", "status": "pending"},
	}}, ec)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if len(ec.Todos.Items) != 2 || ec.Todos.Items[0].Status != "in_progress" {
		t.Errorf("todos = %+v", ec.Todos.Items)
	}
	res, _ = tt.Execute(context.Background(), map[string]any{"action": "read"}, ec)
	if !strings.Contains(res.Content, "step one") {
		t.Errorf("todo read = %q", res.Content)
	}
}

func TestTruncate(t *testing.T) {
	ec, _ := testEC(t, sandbox.WorkspaceWrite)
	ec.MaxToolOutput = 10
	res := tools.Truncate(ec, strings.Repeat("x", 100))
	if len(res.Content) >= 100 {
		t.Error("output should be truncated")
	}
	if !strings.Contains(res.Content, "truncated") {
		t.Errorf("truncation notice missing: %q", res.Content)
	}
}
