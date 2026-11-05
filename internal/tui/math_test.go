package tui

import (
	"strings"
	"testing"
)

func TestRenderMathSymbols(t *testing.T) {
	cases := []struct{ in, want string }{
		{`\alpha + \beta`, "α + β"},
		{`x^2 + y^2 = z^2`, "x² + y² = z²"},
		{`a_{n}`, "aₙ"},
		{`\frac{1}{2}`, "1/2"},
		{`\frac{a+b}{c}`, "(a+b)/(c)"},
		{`\sqrt{x}`, "√(x)"},
		{`\leq \geq \neq`, "≤ ≥ ≠"},
		{`\sum_{i=1}^{n} i`, "∑ᵢ₌₁ⁿ i"},
		{`\mathbb{R}`, "ℝ"},
		{`\text{hello}`, "hello"},
		{`\left(x+1\right)`, "(x+1)"},
		{`\times \cdot \pm`, "× · ±"},
	}
	for _, c := range cases {
		if got := renderMath(c.in); got != c.want {
			t.Errorf("renderMath(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestSplitMathInlineAndDisplay(t *testing.T) {
	parts := splitMath("cost is $x^2$ dollars")
	if len(parts) != 3 {
		t.Fatalf("parts=%d want 3: %+v", len(parts), parts)
	}
	if parts[1].text != "x^2" || !parts[1].math {
		t.Fatalf("inline math part wrong: %+v", parts[1])
	}
	dparts := splitMath("before $$a+b$$ after")
	if len(dparts) != 3 {
		t.Fatalf("parts=%d want 3: %+v", len(dparts), dparts)
	}
	if !dparts[1].display || dparts[1].text != "a+b" {
		t.Fatalf("display math part wrong: %+v", dparts[1])
	}
}

func TestRenderMathLines(t *testing.T) {
	lines := renderMathLines("value $\\alpha$ here")
	if len(lines) != 1 {
		t.Fatalf("lines=%d want 1: %+v", len(lines), lines)
	}
	if !strings.Contains(lines[0].text, "α") {
		t.Fatalf("math not rendered: %q", lines[0].text)
	}
	disp := renderMathLines("$$E = mc^2$$")
	if len(disp) != 1 || !disp[0].display {
		t.Fatalf("display math not marked: %+v", disp)
	}
	if !strings.Contains(disp[0].text, "E = mc²") {
		t.Fatalf("display math wrong: %q", disp[0].text)
	}
}
