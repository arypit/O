package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// renderThread renders the conversation from top to bottom. Every turn is a
// block: the request with its timestamp, the workflow objects the agent
// went through and the final answer.
func (m *Model) renderThread(w int) ([]string, []clickRegion) {
	var lines []string
	var regions []clickRegion

	for ti, t := range m.turns {
		if ti > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.st.Faint.Render(strings.Repeat(IconBar, w)))

		req := wrapText(t.request, w-12, "")
		for i, l := range req {
			if i > 0 {
				lines = append(lines, "  "+m.st.Base.Render(l))
				continue
			}
			// remember where the message starts, so the timestamp tooltip
			// can be placed next to it
			m.turnRows[t.id] = len(lines)
			ts := m.st.Faint.Render(compactTime(t.at))
			gap := w - visWidth(l) - visWidth(ts) - 2
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, m.st.AccentBold.Render(IconPrompt+" ")+m.st.Base.Render(l)+strings.Repeat(" ", gap)+ts)
			regions = append(regions, clickRegion{
				kind: "time", id: t.id,
				y0: len(lines), y1: len(lines),
				x0: w - visWidth(ts) + 1, x1: w,
			})
		}

		for _, s := range t.steps {
			sl, sr := m.renderStep(s, w)
			for _, r := range sr {
				r.y0 += len(lines)
				r.y1 += len(lines)
				regions = append(regions, r)
			}
			lines = append(lines, sl...)
		}

		if strings.TrimSpace(t.response) != "" {
			out := m.renderAnswer(t.response, w)
			for i, l := range out {
				if i == 0 {
					lines = append(lines, " "+m.st.Accent.Render(IconBot)+" "+l)
				} else {
					lines = append(lines, l)
				}
			}
		}
		if t.usage != nil {
			lines = append(lines, m.st.Faint.Render("  "+fmt.Sprintf("tokens · in %s · out %s",
				formatCount(t.usage.InputTokens), formatCount(t.usage.OutputTokens))))
		}
	}
	return lines, regions
}

// renderStep renders one workflow object: a compact summary that can be
// opened to see the details behind it. Every kind of work has its own icon
// and colour, so reading, searching, coding and testing never look alike.
func (m *Model) renderStep(s *step, w int) ([]string, []clickRegion) {
	icon := stepIcon(s.kind)
	style := m.st.Dim
	switch s.kind {
	case stepError:
		style = m.st.Err
	case stepCode:
		style = m.st.Accent
	case stepThought:
		style = m.st.Faint
	case stepTest:
		style = m.st.OK
	case stepPlan:
		style = m.st.AccentBold
	case stepSearch:
		style = m.st.Warn
	}
	summary := s.summary
	if summary == "" {
		summary = s.title
	}
	marker := "  "
	if s.state == stateRunning {
		marker = "  " + m.st.Accent.Render(SpinFrames[m.spin%len(SpinFrames)]) + " "
	} else {
		marker = "  " + style.Render(icon) + " "
	}
	line := marker + style.Render(summary)
	lines := []string{line}
	regions := []clickRegion{{
		kind: "step", id: s.id,
		y0: 1, y1: 1,
		x0: 3, x1: w,
	}}
	if !s.open {
		return lines, regions
	}

	// expanded: the details stay attached to their parent object
	indent := "    "
	lines = append(lines, m.st.Faint.Render(indent+"└─ "+formatDuration(s.duration())))
	if s.detail != "" {
		for _, l := range wrapText(s.detail, w-8, "") {
			lines = append(lines, indent+"  "+m.st.Dim.Render(l))
		}
	}
	for _, sec := range s.sections {
		lines = append(lines, indent+"  "+m.st.Dim.Render("└─ "+sec.title))
		for _, item := range sec.items {
			for i, l := range wrapText(item, w-12, "") {
				if i == 0 {
					lines = append(lines, indent+"    "+m.st.Faint.Render("└─ "+l))
				} else {
					lines = append(lines, indent+"      "+m.st.Faint.Render(l))
				}
			}
		}
	}
	return lines, regions
}

// stepIcon returns the glyph of a workflow object.
func stepIcon(kind string) string {
	switch kind {
	case stepThought:
		return IconSpark
	case stepSearch:
		return IconTool
	case stepCode:
		return IconSquare
	case stepTest:
		return IconOK
	case stepPlan:
		return IconSqOpen
	case stepError:
		return IconErr
	default:
		return IconTool
	}
}

// renderAnswer renders the final answer: markdown, tables, code blocks and
// native charts.
func (m *Model) renderAnswer(text string, w int) []string {
	var out []string
	lines := strings.Split(text, "\n")
	var code []string
	inCode := false
	codeKind := ""
	codeInfo := ""

	flushCode := func() {
		switch codeKind {
		case "chart":
			out = append(out, m.renderChart(code, w)...)
		case "file", "preview":
			out = append(out, m.renderFilePreview(codeInfo, code, w)...)
		case "sources":
			out = append(out, m.renderSources(code, w)...)
		default:
			for _, l := range code {
				out = append(out, m.st.Dim.Render("  │ "+l))
			}
		}
		code = nil
	}

	for i := 0; i < len(lines); i++ {
		l := lines[i]
		trimmed := strings.TrimSpace(l)

		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				flushCode()
				inCode = false
				codeKind = ""
				codeInfo = ""
			} else {
				inCode = true
				info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				fields := strings.Fields(info)
				codeKind = ""
				codeInfo = ""
				if len(fields) > 0 {
					codeKind = strings.ToLower(fields[0])
					if len(fields) > 1 {
						codeInfo = strings.TrimSpace(strings.TrimPrefix(info, fields[0]))
					}
				}
			}
			continue
		}
		if inCode {
			code = append(code, l)
			continue
		}

		// markdown table
		if strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && isTableRule(lines[i+1]) {
			table, consumed := collectTable(lines[i:])
			out = append(out, m.renderTable(table, w)...)
			i += consumed - 1
			continue
		}

		switch {
		case trimmed == "":
			out = append(out, "")
		case strings.HasPrefix(trimmed, "#"):
			level := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
			text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			style := m.st.AccentBold
			if level > 1 {
				style = m.st.Accent
			}
			out = append(out, style.Render(text))
		case strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "***"):
			out = append(out, m.st.Faint.Render(strings.Repeat(IconBar, w-4)))
		case strings.HasPrefix(trimmed, "> "):
			for j, wl := range wrapText(strings.TrimPrefix(trimmed, "> "), w-6, "") {
				if j == 0 {
					out = append(out, m.st.Faint.Render("│ "+wl))
				} else {
					out = append(out, m.st.Faint.Render("│ "+wl))
				}
			}
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || isOrderedItem(trimmed):
			marker := m.st.Accent.Render("•")
			body := trimmed
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
				body = trimmed[2:]
			} else {
				body = trimmed[strings.Index(trimmed, ". ")+2:]
			}
			for j, wl := range wrapText(body, w-6, "") {
				if j == 0 {
					out = append(out, "  "+marker+" "+inlineMarkup(m.st, wl))
				} else {
					out = append(out, "    "+inlineMarkup(m.st, wl))
				}
			}
		default:
			for _, wl := range wrapText(trimmed, w-4, "") {
				out = append(out, inlineMarkup(m.st, wl))
			}
		}
	}
	if inCode {
		flushCode()
	}
	return out
}

// inlineMarkup styles inline code and bold text inside a paragraph.
func inlineMarkup(st Styles, s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '`' {
			if j := indexRune(runes, '`', i+1); j > i {
				b.WriteString(st.Accent.Render(string(runes[i+1 : j])))
				i = j
				continue
			}
		}
		if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '*' {
			if j := indexSeq(runes, "**", i+2); j > i {
				b.WriteString(st.AccentBold.Render(string(runes[i+2 : j])))
				i = j + 1
				continue
			}
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

// indexRune finds the next occurrence of r from i.
func indexRune(runes []rune, r rune, from int) int {
	for i := from; i < len(runes); i++ {
		if runes[i] == r {
			return i
		}
	}
	return -1
}

// indexSeq finds the next occurrence of a sequence from i.
func indexSeq(runes []rune, seq string, from int) int {
	s := []rune(seq)
	for i := from; i+len(s) <= len(runes); i++ {
		match := true
		for j := range s {
			if runes[i+j] != s[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// isTableRule reports whether a line is the separator of a markdown table.
func isTableRule(l string) bool {
	t := strings.TrimSpace(l)
	if !strings.HasPrefix(t, "|") {
		return false
	}
	for _, r := range t {
		if r != '|' && r != '-' && r != ':' && r != ' ' {
			return false
		}
	}
	return strings.Contains(t, "-")
}

// isOrderedItem reports whether a line starts a numbered list item.
func isOrderedItem(l string) bool {
	i := 0
	for i < len(l) && l[i] >= '0' && l[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(l) && l[i] == '.' && l[i+1] == ' '
}

// collectTable gathers the lines of a markdown table.
func collectTable(lines []string) ([][]string, int) {
	var rows [][]string
	consumed := 0
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "|") {
			break
		}
		if isTableRule(t) {
			consumed++
			continue
		}
		rows = append(rows, splitTableRow(t))
		consumed++
	}
	return rows, consumed
}

// splitTableRow splits a table row into its cells.
func splitTableRow(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	l = strings.TrimSuffix(l, "|")
	parts := strings.Split(l, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// renderTable draws a markdown table as aligned columns.
func (m *Model) renderTable(rows [][]string, w int) []string {
	if len(rows) == 0 {
		return nil
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for i, c := range r {
			if n := visWidth(c); n > widths[i] {
				widths[i] = n
			}
		}
	}
	var out []string
	for ri, r := range rows {
		var b strings.Builder
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			b.WriteString(padRunes(cell, widths[i]))
			if i < cols-1 {
				b.WriteString("  ")
			}
		}
		line := strings.TrimRight(b.String(), " ")
		if ri == 0 {
			out = append(out, m.st.AccentBold.Render(line))
			var sep strings.Builder
			for i := 0; i < cols; i++ {
				sep.WriteString(strings.Repeat(IconBar, widths[i]))
				if i < cols-1 {
					sep.WriteString("  ")
				}
			}
			out = append(out, m.st.Faint.Render(sep.String()))
			continue
		}
		out = append(out, m.st.Base.Render(line))
	}
	return out
}

// renderFilePreview draws a file preview: the real content of the file
// when it can be read, with line numbers, so the answer shows the code
// instead of only naming it.
func (m *Model) renderFilePreview(path string, content []string, w int) []string {
	lines := content
	label := path
	if path != "" {
		full := path
		if !filepath.IsAbs(full) {
			full = filepath.Join(m.cwd, full)
		}
		if data, err := os.ReadFile(full); err == nil {
			lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
	}
	if label == "" {
		label = "file preview"
	}
	out := []string{m.st.AccentBold.Render(label)}
	const maxLines = 12
	for i, l := range lines {
		if i >= maxLines {
			out = append(out, m.st.Faint.Render(fmt.Sprintf("  … %d more lines", len(lines)-maxLines)))
			break
		}
		gutter := m.st.Faint.Render(fmt.Sprintf("%4d │ ", i+1))
		out = append(out, gutter+m.st.Base.Render(truncTo(l, maxInt(w-8, 20))))
	}
	if len(lines) == 0 {
		out = append(out, m.st.Dim.Render("  (the file is empty)"))
	}
	return out
}

// renderSources draws a source panel: the places the answer came from.
func (m *Model) renderSources(lines []string, w int) []string {
	out := []string{m.st.AccentBold.Render("Sources")}
	n := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		n++
		out = append(out, m.st.Faint.Render(fmt.Sprintf("%2d. ", n))+m.st.Accent.Render(clipText(l, maxInt(w-6, 20))))
	}
	if n == 0 {
		out = append(out, m.st.Dim.Render("  no sources listed"))
	}
	return out
}

// renderChart draws a native chart from a fenced chart block.
//
//	title <text>
//	bar <label> <value>
//	line <label> <v1,v2,...>
//	stat <label> <value>
func (m *Model) renderChart(spec []string, w int) []string {
	title := ""
	var bars []chartBar
	var stats []chartStat
	for _, l := range spec {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		parts := strings.Fields(t)
		switch parts[0] {
		case "title":
			title = strings.TrimSpace(strings.TrimPrefix(t, "title"))
		case "bar":
			if len(parts) >= 3 {
				v, _ := strconv.ParseFloat(parts[len(parts)-1], 64)
				bars = append(bars, chartBar{label: strings.Join(parts[1:len(parts)-1], " "), value: v})
			}
		case "line":
			if len(parts) >= 3 {
				label := parts[1]
				var vals []float64
				for _, p := range strings.Split(parts[2], ",") {
					v, _ := strconv.ParseFloat(strings.TrimSpace(p), 64)
					vals = append(vals, v)
				}
				bars = append(bars, chartBar{label: label, values: vals})
			}
		case "stat":
			if len(parts) >= 3 {
				stats = append(stats, chartStat{label: strings.Join(parts[1:len(parts)-1], " "), value: parts[len(parts)-1]})
			}
		}
	}
	var out []string
	if title != "" {
		out = append(out, m.st.AccentBold.Render(title))
	}
	max := 0.0
	for _, b := range bars {
		if b.value > max {
			max = b.value
		}
	}
	barW := w - 26
	if barW < 8 {
		barW = 8
	}
	if barW > 40 {
		barW = 40
	}
	labelW := 0
	for _, b := range bars {
		if n := visWidth(b.label); n > labelW {
			labelW = n
		}
	}
	for _, b := range bars {
		if len(b.values) > 0 {
			out = append(out, m.st.Dim.Render(padRunes(b.label, labelW))+"  "+m.st.Accent.Render(sparkline(b.values)))
			continue
		}
		n := 0
		if max > 0 {
			n = int(float64(barW) * (b.value / max))
		}
		if n < 1 && b.value > 0 {
			n = 1
		}
		out = append(out, m.st.Dim.Render(padRunes(b.label, labelW))+"  "+m.st.Accent.Render(strings.Repeat("█", n))+
			m.st.Faint.Render("  "+formatCount(int(b.value))))
	}
	for _, s := range stats {
		out = append(out, m.st.Dim.Render(padRunes(s.label, labelW))+"  "+m.st.Base.Render(s.value))
	}
	return out
}

// chartBar is one bar or line of a chart.
type chartBar struct {
	label  string
	value  float64
	values []float64
}

// chartStat is one key/value row of a chart.
type chartStat struct {
	label string
	value string
}

// sparkline renders values as a unicode sparkline.
func sparkline(values []float64) string {
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	if len(values) == 0 {
		return ""
	}
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	span := max - min
	var b strings.Builder
	for _, v := range values {
		idx := 0
		if span > 0 {
			idx = int((v - min) / span * float64(len(blocks)-1))
		}
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

// formatCount renders a number with thousands separators.
func formatCount(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
