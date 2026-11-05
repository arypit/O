package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Styles holds the lipgloss styles for one theme. Every style carries the
// screen background so lines compose into a solid full-screen canvas.
type Styles struct {
	Base       lipgloss.Style
	Accent     lipgloss.Style
	AccentBold lipgloss.Style
	Dim        lipgloss.Style
	Faint      lipgloss.Style
	Err        lipgloss.Style
	Warn       lipgloss.Style
	OK         lipgloss.Style
	Chip       lipgloss.Style
	Logo       lipgloss.Style
	Cursor     lipgloss.Style
}

func hex(c RGB) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func stylesFor(t Theme) Styles {
	bg := lipgloss.Color(hex(t.BG))
	fg := lipgloss.Color(hex(t.FG))
	return Styles{
		Base:       lipgloss.NewStyle().Foreground(fg).Background(bg),
		Accent:     lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.AccentFG))).Background(bg),
		AccentBold: lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.AccentFG))).Background(bg).Bold(true),
		Dim:        lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.Dim))).Background(bg),
		Faint:      lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.Faint))).Background(bg),
		Err:        lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.Err))).Background(bg),
		Warn:       lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.Warn))).Background(bg),
		OK:         lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.OK))).Background(bg),
		Chip:       lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.AccentInk))).Background(lipgloss.Color(hex(t.Accent))).Bold(true),
		Logo:       lipgloss.NewStyle().Foreground(lipgloss.Color(hex(t.Accent))).Background(bg).Bold(true),
		Cursor:     lipgloss.NewStyle().Foreground(bg).Background(lipgloss.Color(hex(t.Accent))).Bold(true),
	}
}
