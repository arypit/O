// Package session persists conversations as JSONL (one message per line),
// so sessions are crash-safe, resumable and forkable.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adaop/o/internal/core"
)

// ExpandDir resolves a leading "~/" against the user's home directory.
func ExpandDir(dir string) string {
	if strings.HasPrefix(dir, "~/") || dir == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			if dir == "~" {
				return home
			}
			return filepath.Join(home, dir[2:])
		}
	}
	return dir
}

type Session struct {
	ID   string
	Dir  string
	Path string
}

// New creates a new session id and its file.
func New(dir string) (*Session, error) {
	dir = ExpandDir(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%s-%06d", time.Now().UTC().Format("20060102T150405Z"), rand.Intn(1000000))
	s := &Session{ID: id, Dir: dir, Path: filepath.Join(dir, id+".jsonl")}
	f, err := os.Create(s.Path)
	if err != nil {
		return nil, err
	}
	f.Close()
	return s, nil
}

// Open attaches to an existing session id.
func Open(dir, id string) (*Session, error) {
	dir = ExpandDir(dir)
	p := filepath.Join(dir, id+".jsonl")
	if _, err := os.Stat(p); err != nil {
		return nil, fmt.Errorf("session %s not found: %w", id, err)
	}
	return &Session{ID: id, Dir: dir, Path: p}, nil
}

// Append writes one message as a JSON line.
func (s *Session) Append(m core.Message) error {
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

// Load reads all messages of a session.
func Load(dir, id string) ([]core.Message, error) {
	s, err := Open(dir, id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []core.Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var m core.Message
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, fmt.Errorf("corrupt line in %s: %w", s.Path, err)
		}
		out = append(out, m)
	}
	return out, sc.Err()
}

type Info struct {
	ID      string
	Updated time.Time
	Preview string
}

// List returns all sessions in dir, newest first.
func List(dir string) ([]Info, error) {
	dir = ExpandDir(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		info := Info{ID: strings.TrimSuffix(e.Name(), ".jsonl"), Updated: fi.ModTime()}
		if msgs, err := Load(dir, info.ID); err == nil {
			for _, m := range msgs {
				if m.Role == core.RoleUser {
					for _, part := range m.Parts {
						if part.Type == "text" {
							info.Preview = strings.ReplaceAll(part.Text, "\n", " ")
							if len(info.Preview) > 80 {
								info.Preview = info.Preview[:80] + "..."
							}
							break
						}
					}
					break
				}
			}
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}
