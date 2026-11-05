package tui

import (
	"strings"
	"time"
)

// settingsContent renders the settings workspace: a centred, open page with
// a search field on top, categories and settings in alphabetical order and
// the description of the selected setting at the bottom.
func (m *Model) settingsContent(w, contentH int) ([]string, []clickRegion) {
	rows := m.visibleSettings()
	if m.settings.sel >= len(rows) {
		m.settings.sel = 0
	}
	if m.settings.sel < 0 {
		m.settings.sel = 0
	}
	pageW := minInt(w-4, 92)
	if pageW < 24 {
		pageW = w
	}
	left := (w - pageW) / 2
	if left < 0 {
		left = 0
	}
	divider := m.st.Faint.Render(strings.Repeat(IconBar, pageW))

	type srow struct {
		text     string
		category bool
		index    int
		option   int
		title    string
	}

	var body []srow
	body = append(body, srow{text: m.st.AccentBold.Render("Settings")})
	body = append(body, srow{text: m.searchField(pageW)})
	body = append(body, srow{text: divider})

	lastCat := ""
	for i, s := range rows {
		if s.category != lastCat {
			if lastCat != "" {
				body = append(body, srow{text: divider})
			}
			body = append(body, srow{text: m.st.Accent.Render(strings.ToUpper(s.category))})
			lastCat = s.category
		}
		value := m.settingValue(s)
		selected := i == m.settings.sel
		marker := "  "
		titleStyle := m.st.Dim
		if selected {
			marker = m.st.Accent.Render(IconArrow) + " "
			titleStyle = m.st.AccentBold
		}
		titleW := pageW - 4 - visWidth(value) - 2
		if titleW < 10 {
			titleW = 10
		}
		line := marker + titleStyle.Render(padRunes(s.title, titleW))
		valueCol := left + 2 + titleW
		if selected {
			line += m.st.Chip.Render(" " + value + " ")
		} else {
			line += m.st.Dim.Render(value)
		}
		body = append(body, srow{text: line, index: i, title: s.title})

		// clickable values
		if selected && len(s.options) > 0 {
			col := valueCol
			for oi, opt := range s.options {
				_ = oi
				col += visWidth(" " + opt + " ")
			}
		}
	}

	// the description of the selected setting
	descLines := 5
	if contentH < 12 {
		descLines = 3
	}
	var desc []string
	if len(rows) > 0 {
		s := rows[m.settings.sel]
		desc = append(desc, divider)
		desc = append(desc, m.st.AccentBold.Render(s.title)+m.st.Faint.Render(" · "+m.settingValue(s)))
		for _, l := range wrapText(s.desc, pageW, "") {
			desc = append(desc, m.st.Dim.Render(l))
			if len(desc) > descLines+1 {
				break
			}
		}
	}

	// scroll so that the selected setting stays visible
	selectedRow := -1
	for i, r := range body {
		if r.index == m.settings.sel && r.title != "" {
			selectedRow = i
			break
		}
	}
	listH := contentH - len(desc)
	if listH < 1 {
		listH = 1
	}
	if selectedRow >= listH {
		m.settings.scroll = selectedRow - listH + 1
	}
	if m.settings.scroll > len(body)-1 {
		m.settings.scroll = len(body) - 1
	}
	if m.settings.scroll < 0 {
		m.settings.scroll = 0
	}
	visible := body
	if len(body) > listH {
		visible = body[m.settings.scroll:]
		if len(visible) > listH {
			visible = visible[:listH]
		}
	}

	out := make([]string, 0, contentH)
	for _, r := range visible {
		out = append(out, strings.Repeat(" ", left)+r.text)
	}
	for len(out) < listH {
		out = append(out, "")
	}
	for _, l := range desc {
		out = append(out, strings.Repeat(" ", left)+l)
	}
	for len(out) < contentH {
		out = append(out, "")
	}
	return out, nil
}

// searchField renders the search input of the settings workspace.
func (m *Model) searchField(w int) string {
	label := "Search  "
	cursor := "█"
	if !m.animations {
		cursor = "▌"
	}
	query := m.settings.query
	line := m.st.Faint.Render(label) + m.st.Base.Render(query) + m.st.Accent.Render(cursor)
	if visWidth(line) > w {
		line = truncTo(line, w)
	}
	return line
}

// timeSince returns the time elapsed since t.
func timeSince(t time.Time) time.Duration {
	if t.IsZero() {
		return 0
	}
	return time.Since(t)
}
