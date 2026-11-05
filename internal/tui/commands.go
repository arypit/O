package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adaop/o/internal/session"
)

// command is one slash command. Every command carries a short description
// so it can be discovered in the palette without memorising anything.
type command struct {
	name        string
	args        string
	description string
	aliases     []string
	run         func(m *Model, arg string) tea.Cmd
}

// paletteRow is one command shown in the palette.
type paletteRow struct {
	cmd command
}

// allCommands returns the command registry.
func allCommands() []command {
	return []command{
		{
			name:        "/changelog",
			description: "Read what changed in this release",
			run: func(m *Model, _ string) tea.Cmd {
				m.tab = tabChangelog
				m.chlog = loadChangelog(m.cwd)
				return nil
			},
		},
		{
			name:        "/chat",
			description: "Go back to the conversation",
			run: func(m *Model, _ string) tea.Cmd {
				m.tab = tabChat
				return nil
			},
		},
		{
			name:        "/clear",
			description: "Clear the conversation view",
			run: func(m *Model, _ string) tea.Cmd {
				m.turns = nil
				m.cur = nil
				m.scroll = 0
				return nil
			},
		},
		{
			name:        "/exit",
			args:        "",
			description: "Quit O",
			aliases:     []string{"/quit"},
			run: func(m *Model, _ string) tea.Cmd {
				m.quitting = true
				return tea.Quit
			},
		},
		{
			name:        "/fullaccess",
			description: "Turn full access on or off (all tool actions auto-run)",
			run: func(m *Model, _ string) tea.Cmd {
				return m.toggleFullAccess()
			},
		},
		{
			name:        "/help",
			description: "Show the available commands and keyboard shortcuts",
			run: func(m *Model, _ string) tea.Cmd {
				m.showHelp()
				return nil
			},
		},
		{
			name:        "/login",
			description: "Manage model providers and API keys",
			aliases:     []string{"/providers"},
			run: func(m *Model, _ string) tea.Cmd {
				m.tab = tabProviders
				m.prov = LoadProvRows(m.cwd)
				return nil
			},
		},
		{
			name:        "/model",
			description: "Show the active model and provider",
			run: func(m *Model, _ string) tea.Cmd {
				m.openSettingsTo("Model")
				return nil
			},
		},
		{
			name:        "/new",
			description: "Start a new conversation",
			run: func(m *Model, _ string) tea.Cmd {
				if m.eng == nil {
					m.fail(m.engErr)
					return nil
				}
				if _, err := m.eng.NewSession(); err != nil {
					m.fail(err.Error())
					return nil
				}
				m.turns = nil
				m.cur = nil
				m.scroll = 0
				m.notify("success", "New conversation started", "")
				return nil
			},
		},
		{
			name:        "/notifications",
			description: "Show recent notifications",
			run: func(m *Model, _ string) tea.Cmd {
				m.showNotifications()
				return nil
			},
		},
		{
			name:        "/providers",
			description: "Manage model providers and API keys",
			run: func(m *Model, _ string) tea.Cmd {
				m.tab = tabProviders
				m.prov = LoadProvRows(m.cwd)
				return nil
			},
		},
		{
			name:        "/sessions",
			description: "List saved conversations",
			run: func(m *Model, _ string) tea.Cmd {
				m.showSessions()
				return nil
			},
		},
		{
			name:        "/settings",
			description: "Open the settings workspace",
			run: func(m *Model, _ string) tea.Cmd {
				m.openSettings()
				return nil
			},
		},
		{
			name:        "/theme",
			args:        "dark|light",
			description: "Switch the colour theme",
			run: func(m *Model, arg string) tea.Cmd {
				switch strings.ToLower(strings.TrimSpace(arg)) {
				case "dark":
					m.SetTheme("dark")
					m.notify("info", "Theme: dark", "")
				case "light":
					m.SetTheme("light")
					m.notify("info", "Theme: light", "")
				default:
					m.fail("usage: /theme dark|light")
				}
				return nil
			},
		},
		{
			name:        "/version",
			description: "Show the O version",
			run: func(m *Model, _ string) tea.Cmd {
				m.showInfo("O", []string{"version " + readVersion(m.cwd) + " · built by Adaop"})
				return nil
			},
		},
	}
}

// lookupCommand finds a command by name or alias.
func lookupCommand(name string) (command, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, c := range allCommands() {
		if c.name == name {
			return c, true
		}
		for _, a := range c.aliases {
			if a == name {
				return c, true
			}
		}
	}
	return command{}, false
}

// filterCommands narrows the registry down to the commands matching the
// typed query. The query keeps its leading slash.
func filterCommands(query string) []command {
	q := strings.ToLower(strings.TrimSpace(query))
	q = strings.TrimPrefix(q, "/")
	all := allCommands()
	var prefix, contains, described []command
	for _, c := range all {
		names := append([]string{c.name}, c.aliases...)
		matched := false
		prefixHit := false
		for _, n := range names {
			n = strings.TrimPrefix(n, "/")
			if q == "" {
				prefixHit = true
				matched = true
				break
			}
			if strings.HasPrefix(n, q) {
				prefixHit = true
				matched = true
				break
			}
			if strings.Contains(n, q) {
				matched = true
				break
			}
		}
		if !matched && q != "" && strings.Contains(strings.ToLower(c.description), q) {
			matched = true
		}
		if !matched {
			continue
		}
		if prefixHit {
			prefix = append(prefix, c)
		} else if contains == nil && len(prefix) == 0 && strings.Contains(strings.ToLower(c.name), q) {
			contains = append(contains, c)
		} else {
			described = append(described, c)
		}
	}
	out := append(prefix, contains...)
	out = append(out, described...)
	return out
}

// showHelp opens the command list with the key bindings.
func (m *Model) showHelp() {
	lines := []string{}
	for _, c := range allCommands() {
		args := ""
		if c.args != "" {
			args = " " + c.args
		}
		lines = append(lines, c.name+args+" — "+c.description)
	}
	lines = append(lines, "")
	lines = append(lines, "Keys: enter send · ↑↓ scroll · ctrl+o open a step · ctrl+c stop · esc back")
	lines = append(lines, "Mouse: click a workflow object to expand it, hover a timestamp for details")
	m.showInfo("Commands", lines)
}

// showSessions lists the saved conversations.
func (m *Model) showSessions() {
	infos, err := session.List(m.sessionDir())
	if err != nil {
		m.fail(err.Error())
		return
	}
	if len(infos) == 0 {
		m.showInfo("Sessions", []string{"No saved conversations yet."})
		return
	}
	st := LoadState()
	var lines []string
	for _, in := range infos {
		if m.workspaceIsolation && st.Sessions != nil {
			if ws, ok := st.Sessions[in.ID]; ok && ws != m.workspacePath() {
				continue
			}
		}
		lines = append(lines, in.Updated.Format("2006-01-02 15:04")+"  "+in.ID+"  "+in.Preview)
	}
	m.showInfo("Sessions", lines)
}

// showNotifications opens the notification history.
func (m *Model) showNotifications() {
	if len(m.noteHistory) == 0 {
		m.showInfo("Notifications", []string{"No notifications yet."})
		return
	}
	var lines []string
	for _, n := range m.noteHistory {
		line := n.at.Format("15:04:05") + "  " + n.title
		if n.body != "" {
			line += "  " + n.body
		}
		lines = append(lines, line)
	}
	m.showInfo("Notifications", lines)
}

// showInfo opens the command-output panel.
func (m *Model) showInfo(title string, lines []string) {
	m.info = &infoPanel{title: title, lines: lines}
}

// toggleFullAccess switches the permission mode between full-auto and the
// default approval flow and announces it.
func (m *Model) toggleFullAccess() tea.Cmd {
	if m.eng == nil {
		m.fail(m.engErr)
		return nil
	}
	if m.eng.Mode() == string(fullAutoMode) {
		m.setMode(defaultMode)
		m.notify("info", "Full access OFF", "tool actions ask for approval again")
		return nil
	}
	m.setMode(fullAutoMode)
	m.notify("warn", "Full access ON", "all tool actions auto-run")
	return nil
}

// setMode switches the permission mode of the running agent.
func (m *Model) setMode(mode string) {
	if m.eng == nil {
		m.fail(m.engErr)
		return
	}
	if err := m.eng.SetMode(mode); err != nil {
		m.fail(err.Error())
		return
	}
	m.status = "permission mode: " + mode
	m.statusErr = false
}

// openSettings opens the settings workspace.
func (m *Model) openSettings() {
	m.tab = tabSettings
	m.settings.query = ""
	m.settings.sel = 0
	m.settings.prompt = ""
	m.settings.scroll = 0
}

// openSettingsTo opens the settings workspace focused on one setting.
func (m *Model) openSettingsTo(title string) {
	m.openSettings()
	rows := m.visibleSettings()
	for i, r := range rows {
		if strings.EqualFold(r.title, title) {
			m.settings.sel = i
			return
		}
	}
}
