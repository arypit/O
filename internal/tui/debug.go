package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// debugf appends a diagnostic line to ~/.o/tui-debug.log when O_TUI_DEBUG=1.
// It is the first thing to check when input does not behave as expected.
var debugFile *os.File

func debugf(format string, args ...any) {
	if os.Getenv("O_TUI_DEBUG") == "" {
		return
	}
	if debugFile == nil {
		home, _ := os.UserHomeDir()
		_ = os.MkdirAll(filepath.Join(home, ".o"), 0o700)
		debugFile, _ = os.Create(filepath.Join(home, ".o", "tui-debug.log"))
	}
	if debugFile != nil {
		fmt.Fprintf(debugFile, format+"\n", args...)
		debugFile.Sync()
	}
}

// writeDebugFrame dumps the last rendered frame (plain text) next to the
// debug log so the UI can be inspected without a visible terminal.
func writeDebugFrame(frame string) {
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".o", "tui-frame.txt")
	b, err := os.Create(p)
	if err != nil {
		return
	}
	defer b.Close()
	_, _ = b.WriteString(stripEscape(frame))
}

// stripEscape removes ANSI escape sequences from s.
func stripEscape(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = ansiEnd(s, i)
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}
