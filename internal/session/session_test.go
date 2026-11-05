package session

import (
	"testing"

	"github.com/adaop/o/internal/core"
)

func TestSessionRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	m1 := core.NewMessage(core.RoleUser, core.TextPart("hello"))
	m2 := core.NewMessage(core.RoleAssistant, core.TextPart("hi there"))
	if err := s.Append(m1); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(m2); err != nil {
		t.Fatal(err)
	}
	msgs, err := Load(dir, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("loaded %d messages, want 2", len(msgs))
	}
	if msgs[0].Parts[0].Text != "hello" || msgs[1].Role != core.RoleAssistant {
		t.Errorf("messages = %+v", msgs)
	}
	infos, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].ID != s.ID || infos[0].Preview != "hello" {
		t.Errorf("list = %+v", infos)
	}
	// resume: open existing
	s2, err := Open(dir, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s2.ID != s.ID {
		t.Errorf("reopened id = %s", s2.ID)
	}
}
