package tui

import (
	"os"
	"strings"
)

var logoLines = []string{
	" ██████╗ ",
	"██╔═══██╗",
	"██║   ██║",
	"██║   ██║",
	"╚██████╔╝",
	" ╚═════╝ ",
}

// View renders the full-screen frame. The whole frame is one styled block:
// every cell is painted with the theme background (no black gaps), and the
// block is exactly the terminal size, so it scales to any window.
func (m *Model) View() string {
	if m.width <= 0 {
		m.width, m.height = 80, 24
	}
	if m.quitting {
		return ""
	}
	lines, regions := m.layout()
	m.regions = regions
	frame := m.st.Base.Width(m.width).Height(m.height).Render(strings.Join(lines, "\n"))
	// lipgloss resets the style at the end of every inner segment; re-apply
	// the background after each reset so the canvas stays solid.
	frame = strings.ReplaceAll(frame, "\x1b[0m", "\x1b[0m"+m.bgSeq())
	if os.Getenv("O_TUI_DEBUG") != "" {
		writeDebugFrame(frame)
	}
	return frame
}

// bgSeq is the escape sequence that paints the theme background.
func (m *Model) bgSeq() string {
	return "\x1b[48;2;" + itoa(int(m.th.BG.R)) + ";" + itoa(int(m.th.BG.G)) + ";" + itoa(int(m.th.BG.B)) + "m"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func (m *Model) layout() ([]string, []clickRegion) {
	if m.view == viewStartup {
		return m.startupLayout()
	}
	return m.mainLayout()
}

// startupLayout centers the logo block both horizontally and vertically.
func (m *Model) startupLayout() ([]string, []clickRegion) {
	w, h := m.width, m.height
	var block []string
	for _, l := range logoLines {
		block = append(block, m.st.Logo.Render(l))
	}
	block = append(block, "")
	block = append(block, m.st.Dim.Render("autonomous coding agent · Adaop · v"+readVersion(m.cwd)))
	block = append(block, "")

	maxW := 0
	for _, item := range menuItems {
		if n := len([]rune(item)); n > maxW {
			maxW = n
		}
	}
	menuAt := len(block)
	for i, item := range menuItems {
		label := padRunes(item, maxW)
		if i == m.menu {
			block = append(block, m.st.Accent.Render(IconArrow+" ")+m.st.AccentBold.Render(label))
		} else {
			block = append(block, "  "+m.st.Dim.Render(label))
		}
	}
	if m.engErr != "" {
		block = append(block, "")
		block = append(block, m.st.Err.Render(IconErr+" "+m.engErr))
	}
	block = append(block, "")
	block = append(block, m.st.Faint.Render("↑/↓ choose · enter open · ctrl+c quit · mouse click"))

	top := (h - len(block)) / 2
	if top < 0 {
		top = 0
	}
	lines := make([]string, 0, h)
	for i := 0; i < top; i++ {
		lines = append(lines, "")
	}
	var regions []clickRegion
	for i, l := range block {
		pad := (w - visWidth(l)) / 2
		if pad < 0 {
			pad = 0
		}
		lines = append(lines, strings.Repeat(" ", pad)+l)
		if i >= menuAt && i < menuAt+len(menuItems) {
			item := menuItems[i-menuAt]
			x0 := pad + 2
			regions = append(regions, clickRegion{
				y0: top + i + 1, y1: top + i + 1,
				x0: x0 + 1, x1: x0 + len([]rune(item)),
				kind: "menu", index: i - menuAt,
			})
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines, regions
}

// mainLayout renders the workspace: the conversation on top, the composer
// at the bottom and the activity bar underneath it.
func (m *Model) mainLayout() ([]string, []clickRegion) {
	w, h := m.width, m.height
	if w < 30 {
		w = 30
	}
	if h < 12 {
		h = 12
	}
	contentH := h - 7
	if contentH < 1 {
		contentH = 1
	}
	// the display scale changes how much content fits on one line
	if m.fontScale > 0 && m.fontScale != 100 {
		w = int(float64(w) * float64(m.fontScale) / 100)
		if w < 30 {
			w = 30
		}
	}

	lines := make([]string, 0, h)
	regions := []clickRegion{}

	// empty header: every view is reached through the / commands
	lines = append(lines, "")
	lines = append(lines, m.st.Faint.Render(strings.Repeat(IconBar, w)))

	content, contentRegions := m.content(w, contentH)
	for _, r := range contentRegions {
		r.y0, r.y1 = 3+r.y0, 3+r.y1
		regions = append(regions, r)
	}
	lines = append(lines, content...)

	// footer: rule / composer / rule / activity bar
	rule := m.st.Faint.Render(strings.Repeat(IconBar, w))
	lines = append(lines, rule)
	lines = append(lines, m.composer(w))
	lines = append(lines, rule)
	bar, stopRegion := m.activityBar(w)
	lines = append(lines, bar)
	if stopRegion != nil {
		stopRegion.y0, stopRegion.y1 = h, h
		regions = append(regions, *stopRegion)
	}
	return lines, regions
}

// content renders the active workspace and its overlays.
func (m *Model) content(w, contentH int) ([]string, []clickRegion) {
	var out []string
	var regions []clickRegion

	chatW := w

	switch m.tab {
	case tabProviders:
		out = m.provContent(w, contentH)
		regions = m.providerRegions(contentH)
	case tabSettings:
		out, regions = m.settingsContent(w, contentH)
	case tabChangelog:
		out = m.chlogContent(w, contentH)
	default:
		out, regions = m.renderThread(chatW)
	}

	// keep the visible window on the tail of the conversation
	scrollStart := 0
	if m.tab == tabChat {
		maxScroll := len(out) - contentH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.scroll > maxScroll {
			m.scroll = maxScroll
		}
		end := len(out) - m.scroll
		if end < 0 {
			end = 0
		}
		start := end - contentH
		if start < 0 {
			start = 0
		}
		scrollStart = start
		visible := out[start:end]
		for len(visible) < contentH {
			visible = append(visible, "")
		}
		out = visible
		for i := range regions {
			regions[i].y0 -= start
			regions[i].y1 -= start
		}
		kept := regions[:0]
		for _, r := range regions {
			if r.y1 >= 1 && r.y0 <= contentH {
				kept = append(kept, r)
			}
		}
		regions = kept
	}

	// the command palette sits right above the composer
	if m.palette && m.tab == tabChat {
		panel := m.paletteLines(minInt(chatW-4, 74))
		top := contentH - len(panel)
		if top < 0 {
			top = 0
		}
		out = overlayPanel(out, panel, top, 2, w)
	}

	// the command-output panel (help, sessions, notification history) sits
	// above the composer as well, so it never mixes into the conversation
	if m.info != nil && !m.palette && m.tab == tabChat {
		panel := m.infoLines(minInt(chatW-4, 74))
		top := contentH - len(panel)
		if top < 0 {
			top = 0
		}
		out = overlayPanel(out, panel, top, 2, w)
	}

	// the permission dialog is a dedicated approval interface
	if m.perm != nil && m.tab == tabChat {
		panel := m.permissionLines(minInt(chatW-8, 76))
		left := (chatW - visWidth(panel[0])) / 2
		if left < 0 {
			left = 0
		}
		top := (contentH - len(panel)) / 2
		if top < 0 {
			top = 0
		}
		out = overlayPanel(out, panel, top, left, w)
	}

	// A notification is one line in the upper right of the working area.
	// It hides itself after a few seconds; the history stays available
	// through /notifications.
	if len(m.notes) > 0 && m.tab == tabChat {
		line, noteID := m.notificationToast(w)
		left := w - visWidth(line) - 1
		if left < 0 {
			left = 0
		}
		out = overlayPanel(out, []string{line}, 0, left, w)
		regions = append(regions, clickRegion{
			kind: "notice", id: noteID,
			y0: 0, y1: 0,
			x0: left + 1, x1: w,
		})
	}

	// the tooltip of a hovered timestamp
	if m.hoverTurn >= 0 && m.tab == tabChat {
		if t := m.turnByID(m.hoverTurn); t != nil {
			tip := m.tooltipLines(t, chatW)
			row := m.turnRows[t.id] - scrollStart
			top := row + 1
			if top+len(tip) > contentH {
				top = contentH - len(tip)
			}
			if top < 0 {
				top = 0
			}
			left := chatW - visWidth(tip[0]) - 2
			if left < 0 {
				left = 0
			}
			out = overlayPanel(out, tip, top, left, w)
		}
	}
	return out, regions
}

// notificationToast renders the newest notification as one line.
func (m *Model) notificationToast(w int) (string, int) {
	n := m.notes[len(m.notes)-1]
	style := m.noteStyle(n.level)
	icon := IconDot
	switch n.level {
	case "error":
		icon = IconErr
	case "warn":
		icon = IconArrow
	case "success":
		icon = IconOK
	}
	text := n.title
	if n.body != "" {
		text += " · " + n.body
	}
	line := style.Render(icon+" ") + m.st.Dim.Render(clipText(text, maxInt(w-6, 20)))
	return line, n.id
}

// overlayPanel draws a panel on top of the content.
func overlayPanel(base, panel []string, top, left, width int) []string {
	for i, pl := range panel {
		row := top + i
		if row < 0 || row >= len(base) {
			continue
		}
		base[row] = composeRow(base[row], pl, left, width)
	}
	return base
}

// composeRow puts a panel line on top of a base line.
func composeRow(base, panel string, left, width int) string {
	b := base
	if visWidth(b) > left {
		b = truncTo(b, left)
	}
	if visWidth(b) < left {
		b += strings.Repeat(" ", left-visWidth(b))
	}
	out := b + panel
	if pad := width - visWidth(out); pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

// tooltipLines renders the detailed timestamp of a message.
func (m *Model) tooltipLines(t *turn, w int) []string {
	text := fullTime(t.at)
	rows := [][]seg{
		{segf(m.st.Dim, text)},
	}
	width := visWidth(text) + 4
	if width < 14 {
		width = 14
	}
	if w > 0 && width > w-2 {
		width = w - 2
	}
	return m.boxLines("Message", m.st.Faint, rows, width)
}

// turnByID finds a turn by its id.
func (m *Model) turnByID(id int) *turn {
	for _, t := range m.turns {
		if t.id == id {
			return t
		}
	}
	return nil
}

// providerRegions maps provider rows to clickable rectangles. y0 is the
// row inside the content block (zero-based).
func (m *Model) providerRegions(contentH int) []clickRegion {
	var out []clickRegion
	for i, p := range m.prov {
		row := i + 1 // row 0 of the content block is the "providers" caption
		if row >= contentH {
			break
		}
		out = append(out, clickRegion{y0: row, y1: row, x0: 2, x1: 2 + len(p.ID), kind: "provider", index: i})
	}
	return out
}

// composer renders the input line.
func (m *Model) composer(w int) string {
	before := string(m.input[:m.cursor])
	at := " "
	if m.cursor < len(m.input) {
		at = string(m.input[m.cursor])
	}
	after := ""
	if m.cursor < len(m.input) {
		after = string(m.input[m.cursor+1:])
	}
	cursor := m.st.Cursor.Render(at)
	if !m.animations {
		cursor = m.st.AccentBold.Render(at)
	}
	line := m.st.AccentBold.Render(">") + " " + before + cursor + after
	if visWidth(line) > w {
		line = truncTo(line, w)
	}
	return line
}

// identity is the small "mode · provider · model" line of the activity bar.
func (m *Model) identity() string {
	mode, pid, model := "", "", ""
	if m.eng != nil {
		mode = m.eng.Mode()
		_, pid, model = m.eng.Info()
	}
	var parts []string
	if mode != "" {
		parts = append(parts, ModeIcon(mode)+" "+mode)
	}
	if pid != "" {
		parts = append(parts, pid)
	}
	if model != "" {
		parts = append(parts, model)
	}
	return strings.Join(parts, " · ")
}

// activityBar is the live status line: what the agent is doing, how long it
// has been doing it and how to stop it.
func (m *Model) activityBar(w int) (string, *clickRegion) {
	var left string
	var stop *clickRegion
	switch {
	case m.running:
		label := m.activity
		if label == "" {
			label = "Working"
		}
		if !m.activityTracking || !m.statusIndicators {
			label = "Working"
		}
		spinner := SpinFrames[m.spin%len(SpinFrames)]
		if !m.animations {
			spinner = IconDot
		}
		left = m.st.Accent.Render(spinner+" ") + m.st.Dim.Render(label)
	case m.statusErr:
		left = m.st.Err.Render(IconErr + " " + m.status)
	case m.status != "":
		left = m.st.Dim.Render(IconLDot + " " + m.status)
	default:
		left = m.st.Dim.Render(m.identity())
	}

	var right string
	if m.running {
		elapsed := formatDuration(timeSince(m.started))
		right = m.st.Faint.Render(elapsed+"  ")
		stopText := m.st.Accent.Render("stop")
		right += stopText
		if m.statusIndicators {
			x := w - visWidth(stopText)
			stop = &clickRegion{y0: 1, y1: 1, x0: x + 1, x1: w, kind: "stop"}
		}
	} else {
		right = m.st.Faint.Render(m.hints())
	}
	gap := w - visWidth(left) - visWidth(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right, stop
}

// hints returns the key hints of the current view.
func (m *Model) hints() string {
	switch m.tab {
	case tabProviders:
		return "t test · a add · r remove · d default · esc back"
	case tabChangelog:
		return "↑↓ scroll · esc back to chat"
	case tabSettings:
		return "type to search · ↑↓ setting · ←→ value · enter action · esc back"
	default:
		return "/ commands · ↑↓ scroll · ctrl+o expand · ctrl+c stop"
	}
}

func (m *Model) provContent(w, contentH int) []string {
	var out []string
	out = append(out, m.st.Faint.Render("providers — keys are masked"))
	if len(m.prov) == 0 {
		out = append(out, m.st.Dim.Render("none yet — press a to add one"))
	}
	for i, p := range m.prov {
		row := IconTool + " " + p.ID + "  " + p.Base + "  model=" + p.Model + "  " + p.Key
		if p.Default {
			row += "  [default]"
		}
		if i == m.provSel && m.provMode == 0 {
			out = append(out, m.st.Chip.Render(" "+row+" "))
		} else {
			out = append(out, " "+m.st.Dim.Render(row))
		}
	}
	if m.provMode == 1 {
		out = append(out, "")
		out = append(out, m.st.AccentBold.Render(IconArrow+" add provider"))
		for i, f := range m.form {
			val := ""
			if i < len(m.formVals) {
				val = m.formVals[i]
			}
			if i == m.formIdx {
				out = append(out, " "+m.st.Accent.Render(IconRight)+" "+m.st.AccentBold.Render(f+": "+val+"█"))
			} else {
				out = append(out, "   "+m.st.Dim.Render(f+": "+val))
			}
		}
		out = append(out, m.st.Faint.Render(" tab/enter next · esc cancel"))
	}
	if len(out) > contentH {
		out = out[len(out)-contentH:]
	}
	return out
}

func (m *Model) chlogContent(w, contentH int) []string {
	var wrapped []string
	for _, l := range m.chlog {
		wrapped = append(wrapped, wrapText(l, w, "")...)
	}
	maxScroll := len(wrapped) - contentH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.chScroll > maxScroll {
		m.chScroll = maxScroll
	}
	end := len(wrapped) - m.chScroll
	if end < 0 {
		end = 0
	}
	start := end - contentH
	if start < 0 {
		start = 0
	}
	return wrapped[start:end]
}

// padRunes right-pads s with spaces to n visible cells.
func padRunes(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(r))
}

// minInt returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt returns the larger of a and b.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
