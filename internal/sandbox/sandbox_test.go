package sandbox

import (
	"path/filepath"
	"testing"
)

func TestWorkspaceWriteConfinement(t *testing.T) {
	dir := t.TempDir()
	sb, err := New(dir, WorkspaceWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := sb.AllowWrite(filepath.Join(dir, "a.go")); err != nil {
		t.Errorf("write inside workspace should be allowed: %v", err)
	}
	outside := filepath.Join(filepath.Dir(dir), "outside.go")
	if err := sb.AllowWrite(outside); err == nil {
		t.Error("write outside workspace must be denied in workspace-write mode")
	}
	// path traversal must not escape
	if err := sb.AllowWrite(filepath.Join(dir, "..", "escape.go")); err == nil {
		t.Error("traversal path must be denied")
	}
}

func TestReadOnly(t *testing.T) {
	dir := t.TempDir()
	sb, _ := New(dir, ReadOnly)
	if err := sb.AllowWrite(filepath.Join(dir, "a.go")); err == nil {
		t.Error("read-only must deny all writes")
	}
	if err := sb.AllowExec(); err == nil {
		t.Error("read-only must deny shell execution")
	}
}

func TestFullAccess(t *testing.T) {
	sb, _ := New(t.TempDir(), FullAccess)
	if err := sb.AllowWrite(filepath.Join("C:\\", "temp", "x.txt")); err != nil {
		t.Errorf("full-access should allow writes: %v", err)
	}
	if err := sb.AllowExec(); err != nil {
		t.Errorf("full-access should allow exec: %v", err)
	}
}

func TestWithin(t *testing.T) {
	sb, _ := New(t.TempDir(), WorkspaceWrite)
	ws := sb.Workspace
	if !sb.Within(ws) {
		t.Error("workspace itself should be within")
	}
	if !sb.Within(filepath.Join(ws, "sub", "f.go")) {
		t.Error("nested path should be within")
	}
	if sb.Within(filepath.Dir(ws)) {
		t.Error("parent dir must not be within")
	}
}
