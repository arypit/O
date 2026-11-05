package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type RGB struct{ R, G, B uint8 }

func rgb(hex string) RGB {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return RGB{0, 0, 0}
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return RGB{0, 0, 0}
	}
	return RGB{uint8(v >> 16), uint8(v >> 8), uint8(v)}
}

// Theme is the brand palette. Dark fills the whole screen with ink,
// light with warm white; the accent is signal lime in both.
type Theme struct {
	Name                                string
	BG, FG, Accent, AccentFG, AccentInk RGB
	Dim, Faint, Err, Warn, OK           RGB
}

func DarkTheme() Theme {
	return Theme{
		Name: "dark",
		BG:   rgb("#171B20"), FG: rgb("#F4F3EF"),
		Accent: rgb("#C8EE59"), AccentFG: rgb("#C8EE59"), AccentInk: rgb("#171B20"),
		Dim: rgb("#D5D5CE"), Faint: rgb("#5C6470"),
		Err: rgb("#E5484D"), Warn: rgb("#F0B429"), OK: rgb("#C8EE59"),
	}
}

func LightTheme() Theme {
	return Theme{
		Name: "light",
		BG:   rgb("#F4F3EF"), FG: rgb("#171B20"),
		Accent: rgb("#C8EE59"), AccentFG: rgb("#5F7A0A"), AccentInk: rgb("#171B20"),
		Dim: rgb("#6E6E64"), Faint: rgb("#B3B1A6"),
		Err: rgb("#C62F35"), Warn: rgb("#9A6A00"), OK: rgb("#5F7A0A"),
	}
}

func themePath() string {
	if p := os.Getenv("O_TUI_STATE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".o", "tui.json")
	}
	return filepath.Join(home, ".o", "tui.json")
}

// uiState is the persisted TUI state (~/.o/tui.json).
type uiState struct {
	Theme     string `json:"theme,omitempty"`
	RefreshHz int    `json:"refreshHz,omitempty"`

	Animations           *bool  `json:"animations,omitempty"`
	FontScale            int    `json:"fontScale,omitempty"`
	Density              string `json:"density,omitempty"`
	StatusIndicators     *bool  `json:"statusIndicators,omitempty"`
	ActivityTracking     *bool  `json:"activityTracking,omitempty"`
	AutoContinue         *bool  `json:"autoContinue,omitempty"`
	ExecutionTransparent *bool  `json:"executionTransparency,omitempty"`
	PlanFirst            *bool  `json:"planFirst,omitempty"`
	NotificationsOn      *bool  `json:"notificationsOn,omitempty"`
	NoteTTLSeconds       int    `json:"notificationSeconds,omitempty"`
	RememberDecisions    *bool  `json:"rememberDecisions,omitempty"`
	SensitiveProtection  *bool  `json:"sensitiveProtection,omitempty"`
	DataCollection       *bool  `json:"dataCollection,omitempty"`
	Telemetry            *bool  `json:"telemetry,omitempty"`
	ClearOnExit          *bool  `json:"clearOnExit,omitempty"`
	FileDiscovery        *bool  `json:"fileDiscovery,omitempty"`
	ProjectContext       *bool  `json:"projectContext,omitempty"`
	WorkspaceIsolation   *bool  `json:"workspaceIsolation,omitempty"`

	// Sessions maps a session id to the workspace it belongs to, so
	// workspace isolation can hide the conversations of other projects.
	Sessions map[string]string `json:"sessions,omitempty"`
}

// boolPtr returns a pointer to v.
func boolPtr(v bool) *bool { return &v }

// boolVal reads a possibly unset boolean.
func boolVal(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// LoadState reads the persisted TUI state; missing fields stay zero.
func LoadState() uiState {
	var st uiState
	if b, err := os.ReadFile(themePath()); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// SaveState writes the TUI state file (owner-only).
func SaveState(st uiState) error {
	p := themePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(st)
	return os.WriteFile(p, b, 0o600)
}

// LoadTheme reads ~/.o/tui.json {"theme":"dark"|"light"}; default dark.
func LoadTheme() Theme {
	if LoadState().Theme == "light" {
		return LightTheme()
	}
	return DarkTheme()
}

func SaveTheme(name string) error {
	st := LoadState()
	st.Theme = name
	return SaveState(st)
}

// LoadRefreshHz returns the saved refresh rate, or 0 when unset.
func LoadRefreshHz() int { return LoadState().RefreshHz }

// SaveRefreshHz stores the refresh rate in the TUI state file.
func SaveRefreshHz(hz int) error {
	st := LoadState()
	st.RefreshHz = hz
	return SaveState(st)
}

func (t Theme) fg(c RGB) string              { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B) }
func (t Theme) bg(c RGB) string              { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c.R, c.G, c.B) }
func (t Theme) reset() string                { return "\x1b[0m" + t.bg(t.BG) }
func (t Theme) paint(s string, c RGB) string { return t.fg(c) + s + t.reset() }
func (t Theme) bold(s string) string         { return "\x1b[1m" + s + "\x1b[22m" + t.bg(t.BG) }
func (t Theme) dim(s string) string          { return t.paint(s, t.Dim) }
func (t Theme) faint(s string) string        { return t.paint(s, t.Faint) }

// chip is an inverted accent label (active tab / selected row).
func (t Theme) chip(s string) string {
	return t.bg(t.Accent) + t.fg(t.AccentInk) + "\x1b[1m" + s + "\x1b[22m" + t.fg(t.FG) + t.bg(t.BG)
}
