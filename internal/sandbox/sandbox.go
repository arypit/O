// Package sandbox confines file writes and shell execution.
//
// Modes:
//   read-only       - no writes, no shell execution
//   workspace-write - writes only inside the workspace; shell allowed
//   full-access     - everything allowed (still logged)
package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Mode string

const (
	ReadOnly       Mode = "read-only"
	WorkspaceWrite Mode = "workspace-write"
	FullAccess     Mode = "full-access"
)

type Sandbox struct {
	Mode      Mode
	Workspace string // absolute, cleaned
}

func New(workspace string, mode Mode) (*Sandbox, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	if mode == "" {
		mode = WorkspaceWrite
	}
	return &Sandbox{Mode: mode, Workspace: filepath.Clean(abs)}, nil
}

// Resolve returns the absolute cleaned path for p relative to the workspace.
func (s *Sandbox) Resolve(p string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.Workspace, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// Within reports whether abs is inside the workspace directory.
func (s *Sandbox) Within(abs string) bool {
	rel, err := filepath.Rel(s.Workspace, abs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

var ErrBlocked = errors.New("blocked by sandbox")

// AllowWrite returns nil if writing path is permitted.
func (s *Sandbox) AllowWrite(path string) error {
	switch s.Mode {
	case ReadOnly:
		return fmt.Errorf("%w: read-only mode denies writes to %s", ErrBlocked, path)
	case WorkspaceWrite:
		abs, err := s.Resolve(path)
		if err != nil {
			return err
		}
		if !s.Within(abs) {
			return fmt.Errorf("%w: %s is outside workspace %s", ErrBlocked, abs, s.Workspace)
		}
		return nil
	default: // full-access
		return nil
	}
}

// AllowRead permits reads anywhere by default; kept as a hook for stricter modes.
func (s *Sandbox) AllowRead(path string) error { return nil }

// AllowExec returns nil if shell execution is permitted.
func (s *Sandbox) AllowExec() error {
	if s.Mode == ReadOnly {
		return fmt.Errorf("%w: read-only mode denies shell execution", ErrBlocked)
	}
	return nil
}

// EnsureDir makes sure the parent directory of path exists (used before writes).
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
