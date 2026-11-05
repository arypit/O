package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run is the entry point for `o tui`: it opens the native full-screen TUI
// (Bubble Tea owns the console: raw mode, alternate screen, keyboard and
// mouse reporting) and runs until the user quits.
func Run(args []string) error {
	theme := ""
	for i, a := range args {
		if a == "--theme" && i+1 < len(args) {
			theme = args[i+1]
		}
	}
	cwd, _ := os.Getwd()
	m := NewModel(cwd)
	if theme == "light" || theme == "dark" {
		m.SetTheme(theme)
	}

	p := tea.NewProgram(m,
		tea.WithAltScreen(),       // own the whole screen
		tea.WithMouseCellMotion(), // mouse reporting: clicks + wheel
		tea.WithFPS(m.refreshHz),  // UI refresh rate (60/120/144/440)
	)
	_, err := p.Run()
	if m.eng != nil {
		m.eng.Close()
	}
	return err
}
