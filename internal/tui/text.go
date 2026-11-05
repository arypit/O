package tui

import (
	"strings"
	"unicode/utf8"
)

// visWidth returns the visible cell width of s, ignoring ANSI escape
// sequences (CSI: ESC [ ... final-byte, plus OSC/other short escapes).
// East-Asian-wide runes count 2; icons/glyphs (✦ ✔ ✖ ▲ ● ◆ ◇ ■ □ • · › ▸ < > ─ ϟ braille)
// count 1 — they are text-presentation glyphs, not emoji.
func visWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = ansiEnd(s, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if isWideRune(r) {
			w += 2
		} else {
			w++
		}
		i += size
	}
	return w
}

// ansiEnd returns the index just past the escape sequence starting at s[i]
// (s[i] == ESC), or -1 if s[i] does not start one.
func ansiEnd(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	switch s[i+1] {
	case '[': // CSI: params/intermediates then a final byte 0x40-0x7E
		j := i + 2
		for j < len(s) {
			c := s[j]
			if c >= 0x40 && c <= 0x7e {
				return j + 1
			}
			j++
		}
		return len(s)
	case ']': // OSC: terminated by BEL or ESC \
		j := i + 2
		for j < len(s) {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return len(s)
	default: // two-byte escape (ESC c) or ESC + intermediates + final
		j := i + 1
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j < len(s) {
			return j + 1
		}
		return len(s)
	}
}

func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x20000 && r <= 0x3FFFD:
		return true
	}
	return false
}

// truncTo keeps ANSI sequences but cuts the string to w visible cells.
func truncTo(s string, w int) string {
	if visWidth(s) <= w {
		return s
	}
	var b strings.Builder
	vis := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			end := ansiEnd(s, i)
			b.WriteString(s[i:end])
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		cw := 1
		if isWideRune(r) {
			cw = 2
		}
		if vis+cw > w {
			break
		}
		vis += cw
		b.WriteString(s[i : i+size])
		i += size
	}
	return b.String()
}

// padTo truncates or right-pads s with spaces to exactly w visible cells.
func padTo(s string, w int) string {
	vw := visWidth(s)
	if vw >= w {
		return truncTo(s, w)
	}
	return s + strings.Repeat(" ", w-vw)
}

// centerIn centers s within w visible cells.
func centerIn(s string, w int) string {
	vw := visWidth(s)
	if vw >= w {
		return truncTo(s, w)
	}
	left := (w - vw) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-vw-left)
}

// wrapText word-wraps plain text (no ANSI) to width cells; continuation
// lines are prefixed with indent. Long words are hard-broken.
func wrapText(s string, width int, indent string) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		var lines []string
		cur := ""
		for _, w := range words {
			cand := w
			if cur != "" {
				cand = cur + " " + w
			}
			if cur == "" || visWidth(cand) <= width {
				cur = cand
			} else {
				lines = append(lines, cur)
				cur = w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		// hard-break lines still too long
		var brk []string
		for _, l := range lines {
			for visWidth(l) > width {
				brk = append(brk, truncTo(l, width))
				l = l[byteLen(l, width):]
			}
			if l != "" {
				brk = append(brk, l)
			}
		}
		for i, l := range brk {
			if i == 0 {
				out = append(out, l)
			} else {
				out = append(out, indent+l)
			}
		}
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// byteLen returns the byte length of the first w visible cells of s (plain text).
func byteLen(s string, w int) int {
	vis := 0
	for i, r := range s {
		cw := 1
		if isWideRune(r) {
			cw = 2
		}
		if vis+cw > w {
			return i
		}
		vis += cw
	}
	return len(s)
}
