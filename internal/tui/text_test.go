package tui

import "testing"

func TestVisWidthIconsAreOneCell(t *testing.T) {
	for _, s := range []string{IconBot, IconSquare, IconSqOpen, IconDot, IconLDot, IconLeft, IconRight, IconPrompt, IconArrow, IconOK, IconErr, IconWarn, IconTool, IconBar, IconEnergy, IconSpark, SpinFrames[0]} {
		if w := visWidth(s); w != 1 {
			t.Errorf("visWidth(%q) = %d, want 1", s, w)
		}
	}
}

func TestVisWidthIgnoresANSI(t *testing.T) {
	th := DarkTheme()
	s := th.paint("hello", th.Accent)
	if w := visWidth(s); w != 5 {
		t.Fatalf("visWidth(styled) = %d, want 5", w)
	}
}

func TestPadTruncCenter(t *testing.T) {
	if got := padTo("hi", 5); got != "hi   " {
		t.Fatalf("padTo = %q", got)
	}
	if got := padTo("hello world", 5); got != "hello" {
		t.Fatalf("padTo long = %q", got)
	}
	if got := visWidth(centerIn("ab", 6)); got != 6 {
		t.Fatalf("center width = %d", got)
	}
	c := centerIn("ab", 6)
	if c != "  ab  " {
		t.Fatalf("centerIn = %q", c)
	}
}

func TestWrapText(t *testing.T) {
	got := wrapText("hello world foo", 8, "  ")
	want := []string{"hello", "  world", "  foo"}
	if len(got) != len(want) {
		t.Fatalf("wrap = %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wrap[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// long word hard-breaks
	got = wrapText("abcdefghij", 4, "")
	if len(got) != 3 || got[0] != "abcd" {
		t.Fatalf("hard break = %q", got)
	}
}

func TestTruncToKeepsANSI(t *testing.T) {
	th := DarkTheme()
	s := th.paint("hello", th.Err)
	got := truncTo(s, 3)
	if visWidth(got) != 3 {
		t.Fatalf("trunc vis = %d (%q)", visWidth(got), got)
	}
	if got[:1] != "\x1b" {
		t.Fatalf("trunc lost ANSI prefix: %q", got)
	}
}
