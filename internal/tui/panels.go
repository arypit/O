package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// seg is one styled piece of a panel line.
type seg struct {
	text  string
	style lipgloss.Style
}

// segf builds a styled segment.
func segf(style lipgloss.Style, text string) seg { return seg{text: text, style: style} }

// boxLines draws a bordered panel of exactly the given width. The panel has
// no heavy page frame: only the border around this one component.
func (m *Model) boxLines(title string, titleStyle lipgloss.Style, rows [][]seg, width int) []string {
	if width < 14 {
		width = 14
	}
	inner := width - 4
	out := make([]string, 0, len(rows)+2)

	t := clipText(title, inner-2)
	fill := width - 5 - visWidth(t)
	if fill < 0 {
		fill = 0
	}
	out = append(out, m.st.Base.Render("╭─ ")+titleStyle.Render(t)+m.st.Base.Render(" "+strings.Repeat("─", fill)+"╮"))

	for _, row := range rows {
		var b strings.Builder
		for _, s := range row {
			b.WriteString(s.style.Render(s.text))
		}
		line := b.String()
		if visWidth(line) > inner {
			line = truncTo(line, inner)
		}
		if pad := inner - visWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out = append(out, m.st.Base.Render("│ ")+line+m.st.Base.Render(" │"))
	}

	out = append(out, m.st.Base.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return out
}

// paletteLines renders the command palette: every matching command with a
// short description of what it does.
func (m *Model) paletteLines(width int) []string {
	rows := make([][]seg, 0, len(m.paletteRows)+1)
	nameW, argsW := 0, 0
	for _, r := range m.paletteRows {
		if n := len([]rune(r.cmd.name)); n > nameW {
			nameW = n
		}
		if n := len([]rune(r.cmd.args)); n > argsW {
			argsW = n
		}
	}
	if len(m.paletteRows) == 0 {
		rows = append(rows, []seg{
			segf(m.st.Dim, "  no command matches "),
			segf(m.st.Faint, clipText(string(m.input), 40)),
		})
	}
	for i, r := range m.paletteRows {
		marker := "  "
		nameStyle := m.st.Dim
		if i == m.paletteSel {
			marker = m.st.Accent.Render(IconArrow) + " "
			nameStyle = m.st.AccentBold
		}
		row := []seg{
			segf(m.st.Base, marker),
			segf(nameStyle, padRunes(r.cmd.name, nameW)),
		}
		if argsW > 0 {
			row = append(row, segf(m.st.Faint, " "+padRunes(r.cmd.args, argsW)+" "))
		} else {
			row = append(row, segf(m.st.Base, " "))
		}
		row = append(row, segf(m.st.Dim, r.cmd.description))
		rows = append(rows, row)
	}
	hint := []seg{
		segf(m.st.Faint, "↑↓ choose · enter run · esc close"),
	}
	rows = append(rows, hint)
	return m.boxLines("Commands", m.st.AccentBold, rows, width)
}

// permissionLines renders the approval dialog: what will happen, why it is
// needed, which access it needs and which choices are available.
func (m *Model) permissionLines(width int) []string {
	if m.perm == nil {
		return nil
	}
	info := m.perm.action
	rows := [][]seg{
		{segf(m.st.AccentBold, info.Title)},
		{},
		{segf(m.st.Faint, "Command "), segf(m.st.Dim, clipText(info.Detail, 80))},
		{segf(m.st.Faint, "Why     "), segf(m.st.Dim, clipText(info.Why, 80))},
		{segf(m.st.Faint, "Access  "), segf(m.st.Dim, clipText(info.Level, 80))},
		{},
		{
			segf(m.st.AccentBold, "[y]"), segf(m.st.Dim, " allow once   "),
			segf(m.st.AccentBold, "[a]"), segf(m.st.Dim, " always allow   "),
			segf(m.st.AccentBold, "[n]"), segf(m.st.Dim, " deny"),
		},
	}
	return m.boxLines("Permission required", m.st.Warn, rows, width)
}

// infoLines renders the command-output panel: the result of /help,
// /sessions, /notifications and /version.
func (m *Model) infoLines(width int) []string {
	if m.info == nil {
		return nil
	}
	rows := make([][]seg, 0, len(m.info.lines)+1)
	for _, l := range m.info.lines {
		rows = append(rows, []seg{segf(m.st.Dim, l)})
	}
	rows = append(rows, []seg{segf(m.st.Faint, "esc close")})
	return m.boxLines(m.info.title, m.st.AccentBold, rows, width)
}

// noteStyle returns the style used for a notification level.
func (m *Model) noteStyle(level string) lipgloss.Style {
	switch level {
	case "error":
		return m.st.Err
	case "warn":
		return m.st.Warn
	case "success":
		return m.st.OK
	default:
		return m.st.Dim
	}
}
