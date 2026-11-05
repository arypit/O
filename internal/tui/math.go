package tui

import (
	"strings"
)

// Math rendering for the TUI.
//
// Chat text can contain LaTeX/KaTeX style math. In a terminal the math is
// rendered as Unicode text (KaTeX itself is a browser library, so it cannot
// run here): fractions, powers, subscripts, roots, Greek letters, operators,
// delimiters and the common environments are converted to their Unicode
// equivalents.
//
// Supported delimiters:
//   $...$      inline math
//   $$...$$    display math (rendered on its own line)
//   \( ... \)  inline math
//   \[ ... \]  display math

// mathSymbols maps LaTeX commands to Unicode.
var mathSymbols = map[string]string{
	// greek
	"alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ", "epsilon": "ε",
	"varepsilon": "ε", "zeta": "ζ", "eta": "η", "theta": "θ", "vartheta": "ϑ",
	"iota": "ι", "kappa": "κ", "lambda": "λ", "mu": "μ", "nu": "ν", "xi": "ξ",
	"pi": "π", "varpi": "ϖ", "rho": "ρ", "varrho": "ϱ", "sigma": "σ",
	"varsigma": "ς", "tau": "τ", "upsilon": "υ", "phi": "φ", "varphi": "ϕ",
	"chi": "χ", "psi": "ψ", "omega": "ω",
	"Gamma": "Γ", "Delta": "Δ", "Theta": "Θ", "Lambda": "Λ", "Xi": "Ξ",
	"Pi": "Π", "Sigma": "Σ", "Upsilon": "Υ", "Phi": "Φ", "Psi": "Ψ", "Omega": "Ω",
	// operators and relations
	"times": "×", "div": "÷", "cdot": "·", "pm": "±", "mp": "∓",
	"leq": "≤", "le": "≤", "geq": "≥", "ge": "≥", "neq": "≠", "ne": "≠",
	"approx": "≈", "equiv": "≡", "sim": "∼", "simeq": "≃", "cong": "≅",
	"propto": "∝", "ll": "≪", "gg": "≫", "subset": "⊂", "supset": "⊃",
	"subseteq": "⊆", "supseteq": "⊇", "in": "∈", "notin": "∉", "ni": "∋",
	"cup": "∪", "cap": "∩", "setminus": "∖", "wedge": "∧", "land": "∧",
	"vee": "∨", "lor": "∨", "neg": "¬", "lnot": "¬", "forall": "∀",
	"exists": "∃", "nexists": "∄", "emptyset": "∅", "varnothing": "∅",
	"infty": "∞", "partial": "∂", "nabla": "∇", "surd": "√", "sqrt": "√",
	"angle": "∠", "perp": "⊥", "parallel": "∥", "therefore": "∴",
	"because": "∵", "int": "∫", "iint": "∬", "iiint": "∭", "oint": "∮",
	"sum": "∑", "prod": "∏", "coprod": "∐",
	"to": "→", "rightarrow": "→", "leftarrow": "←", "leftrightarrow": "↔",
	"Rightarrow": "⇒", "Leftarrow": "⇐", "Leftrightarrow": "⇔",
	"mapsto": "↦", "implies": "⟹", "iff": "⟺", "mid": "|", "vert": "|",
	"lvert": "|", "rvert": "|", "Vert": "‖", "lVert": "‖", "rVert": "‖",
	"circ": "∘", "bullet": "•", "star": "⋆", "ast": "∗", "oplus": "⊕",
	"ominus": "⊖", "otimes": "⊗", "odot": "⊙", "dagger": "†", "ddagger": "‡",
	"lceil": "⌈", "rceil": "⌉", "lfloor": "⌊", "rfloor": "⌋",
	"langle": "⟨", "rangle": "⟩", "aleph": "ℵ", "hbar": "ℏ", "ell": "ℓ",
	"Re": "ℜ", "Im": "ℑ", "wp": "℘", "prime": "′", "degree": "°",
	"ellipsis": "…", "dots": "…", "cdots": "⋯", "ldots": "…", "vdots": "⋮",
	"ddots": "⋱", "backslash": "\\",
	// functions (rendered as words)
	"sin": "sin", "cos": "cos", "tan": "tan", "cot": "cot", "sec": "sec",
	"csc": "csc", "arcsin": "arcsin", "arccos": "arccos", "arctan": "arctan",
	"sinh": "sinh", "cosh": "cosh", "tanh": "tanh", "log": "log", "ln": "ln",
	"exp": "exp", "lim": "lim", "limsup": "lim sup", "liminf": "lim inf",
	"max": "max", "min": "min", "sup": "sup", "inf": "inf", "arg": "arg",
	"deg": "deg", "det": "det", "dim": "dim", "gcd": "gcd", "ker": "ker",
	"hom": "hom", "mod": "mod",
}

// mathBB maps \mathbb letters to double-struck Unicode.
var mathBB = map[rune]rune{
	'R': 'ℝ', 'Z': 'ℤ', 'N': 'ℕ', 'Q': 'ℚ', 'C': 'ℂ', 'P': 'ℙ',
	'E': '𝔼', 'H': 'ℍ', 'F': '𝔽',
}

// mathCal maps \mathcal letters to script Unicode.
var mathCal = map[rune]rune{
	'A': '𝒜', 'B': 'ℬ', 'C': '𝒞', 'D': '𝒟', 'E': 'ℰ', 'F': 'ℱ', 'G': '𝒢',
	'H': 'ℋ', 'I': 'ℐ', 'J': '𝒥', 'K': '𝒦', 'L': 'ℒ', 'M': 'ℳ', 'N': '𝒩',
	'O': '𝒪', 'P': '𝒫', 'Q': '𝒬', 'R': 'ℛ', 'S': '𝒮', 'T': '𝒯', 'U': '𝒰',
	'V': '𝒱', 'W': '𝒲', 'X': '𝒳', 'Y': '𝒴', 'Z': '𝒵',
}

// superscript and subscript tables.
var superMap = map[rune]string{
	'0': "⁰", '1': "¹", '2': "²", '3': "³", '4': "⁴", '5': "⁵", '6': "⁶",
	'7': "⁷", '8': "⁸", '9': "⁹", '+': "⁺", '-': "⁻", '=': "⁼", '(': "⁽",
	')': "⁾", 'a': "ᵃ", 'b': "ᵇ", 'c': "ᶜ", 'd': "ᵈ", 'e': "ᵉ", 'f': "ᶠ",
	'g': "ᵍ", 'h': "ʰ", 'i': "ⁱ", 'j': "ʲ", 'k': "ᵏ", 'l': "ˡ", 'm': "ᵐ",
	'n': "ⁿ", 'o': "ᵒ", 'p': "ᵖ", 'r': "ʳ", 's': "ˢ", 't': "ᵗ", 'u': "ᵘ",
	'v': "ᵛ", 'w': "ʷ", 'x': "ˣ", 'y': "ʸ", 'z': "ᶻ", 'T': "ᵀ",
	'α': "ᵅ", 'β': "ᵝ", 'γ': "ᵞ", 'δ': "ᵟ", 'θ': "ᶿ", 'μ': "ᵙ", 'φ': "ᶠ",
	'ω': "ᵚ",
}

var subMap = map[rune]string{
	'0': "₀", '1': "₁", '2': "₂", '3': "₃", '4': "₄", '5': "₅", '6': "₆",
	'7': "₇", '8': "₈", '9': "₉", '+': "₊", '-': "₋", '=': "₌", '(': "₍",
	')': "₎", 'a': "ₐ", 'e': "ₑ", 'o': "ₒ", 'x': "ₓ", 'h': "ₕ", 'k': "ₖ",
	'l': "ₗ", 'm': "ₘ", 'n': "ₙ", 'p': "ₚ", 's': "ₛ", 't': "ₜ", 'i': "ᵢ",
	'j': "ⱼ", 'r': "ᵣ", 'u': "ᵤ", 'v': "ᵥ", 'β': "ᵦ", 'γ': "ᵧ", 'ρ': "ᵨ",
	'φ': "ᵩ", 'χ': "ᵪ",
}

// splitMath splits text into plain and math parts.
type mathPart struct {
	text    string
	math    bool
	display bool
}

func splitMath(s string) []mathPart {
	var out []mathPart
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			out = append(out, mathPart{text: plain.String()})
			plain.Reset()
		}
	}
	for i := 0; i < len(s); {
		// \( ... \) and \[ ... \]
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '(':
				if end := strings.Index(s[i+2:], "\\)"); end >= 0 {
					flush()
					out = append(out, mathPart{text: s[i+2 : i+2+end], math: true})
					i += 2 + end + 2
					continue
				}
			case '[':
				if end := strings.Index(s[i+2:], "\\]"); end >= 0 {
					flush()
					out = append(out, mathPart{text: s[i+2 : i+2+end], math: true, display: true})
					i += 2 + end + 2
					continue
				}
			}
		}
		if s[i] == '$' {
			if i+1 < len(s) && s[i+1] == '$' { // display math
				if end := strings.Index(s[i+2:], "$$"); end >= 0 {
					flush()
					out = append(out, mathPart{text: s[i+2 : i+2+end], math: true, display: true})
					i += 2 + end + 2
					continue
				}
			} else { // inline math
				if end := strings.IndexByte(s[i+1:], '$'); end >= 0 {
					flush()
					out = append(out, mathPart{text: s[i+1 : i+1+end], math: true})
					i += 1 + end + 1
					continue
				}
			}
		}
		plain.WriteByte(s[i])
		i++
	}
	flush()
	return out
}

// mathParser walks a LaTeX expression and produces Unicode text.
type mathParser struct {
	src string
	pos int
}

// renderMath converts a LaTeX/KaTeX expression into Unicode text.
func renderMath(src string) string {
	p := &mathParser{src: src}
	out := p.group(false)
	return strings.TrimSpace(out)
}

// mathLine is one rendered line of chat text; display math is marked so it
// can be centered.
type mathLine struct {
	text    string
	display bool
}

// renderMathLines renders every math part of a chat line. Display math is
// returned as separate lines so it can be centered.
func renderMathLines(s string) []mathLine {
	parts := splitMath(s)
	if len(parts) == 0 {
		return []mathLine{{text: s}}
	}
	var lines []mathLine
	var cur strings.Builder
	for _, p := range parts {
		if !p.math {
			cur.WriteString(p.text)
			continue
		}
		if p.display {
			if strings.TrimSpace(cur.String()) != "" {
				lines = append(lines, mathLine{text: cur.String()})
				cur.Reset()
			}
			for _, l := range strings.Split(renderMath(p.text), "\n") {
				lines = append(lines, mathLine{text: l, display: true})
			}
			continue
		}
		cur.WriteString(renderMath(p.text))
	}
	if cur.Len() > 0 || len(lines) == 0 {
		lines = append(lines, mathLine{text: cur.String()})
	}
	return lines
}

func (p *mathParser) group(inArg bool) string {
	var b strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch c {
		case '{':
			p.pos++
			b.WriteString(p.group(true))
		case '}':
			if inArg {
				p.pos++ // consume the closing brace
				return b.String()
			}
			p.pos++
		case '\\':
			b.WriteString(p.command())
		case '^':
			p.pos++
			b.WriteString(superscript(p.argument()))
		case '_':
			p.pos++
			b.WriteString(subscript(p.argument()))
		case '&':
			p.pos++
			b.WriteString(", ")
		case '~':
			p.pos++
			b.WriteString(" ")
		case '\n':
			p.pos++
			b.WriteString("\n")
		default:
			p.pos++
			b.WriteByte(c)
		}
	}
	return b.String()
}

// argument reads a {...} group or a single character.
func (p *mathParser) argument() string {
	if p.pos >= len(p.src) {
		return ""
	}
	if p.src[p.pos] == '{' {
		p.pos++
		return p.group(true)
	}
	c := p.src[p.pos]
	p.pos++
	return string(c)
}

// command reads a LaTeX command (backslash + name or a single symbol).
func (p *mathParser) command() string {
	p.pos++ // consume the backslash
	if p.pos >= len(p.src) {
		return ""
	}
	c := p.src[p.pos]
	if !isLetter(c) {
		p.pos++
		switch c {
		case '{':
			return "{"
		case '}':
			return "}"
		case '$':
			return "$"
		case '&':
			return ", "
		case '%':
			return "%"
		case '#':
			return "#"
		case '_':
			return "_"
		case '\\':
			return "\n"
		case ' ':
			return " "
		case ',':
			return " "
		case ';':
			return " "
		case ':':
			return " "
		case '!':
			return ""
		default:
			return string(c)
		}
	}
	start := p.pos
	for p.pos < len(p.src) && isLetter(p.src[p.pos]) {
		p.pos++
	}
	name := p.src[start:p.pos]
	return p.apply(name)
}

// apply executes a named command.
func (p *mathParser) apply(name string) string {
	switch name {
	case "frac", "dfrac", "tfrac", "cfrac":
		num := p.argument()
		den := p.argument()
		return fraction(num, den)
	case "binom":
		top := p.argument()
		bot := p.argument()
		return "C(" + top + "," + bot + ")"
	case "sqrt":
		if p.pos < len(p.src) && p.src[p.pos] == '[' {
			p.pos++
			start := p.pos
			for p.pos < len(p.src) && p.src[p.pos] != ']' {
				p.pos++
			}
			idx := p.src[start:p.pos]
			if p.pos < len(p.src) {
				p.pos++
			}
			return superscript(idx) + "√(" + p.argument() + ")"
		}
		return "√(" + p.argument() + ")"
	case "text", "textbf", "textit", "textrm", "textnormal", "mbox", "operatorname", "mathsf", "texttt":
		return p.argument()
	case "mathrm", "mathbf", "mathit", "mathnormal", "boldsymbol":
		return p.argument()
	case "mathbb":
		return mapRunes(p.argument(), mathBB)
	case "mathcal", "mathscr":
		return mapRunes(p.argument(), mathCal)
	case "hat", "widehat", "bar", "overline", "underline", "vec", "tilde", "dot":
		s := p.argument()
		switch name {
		case "vec":
			return s + "→"
		case "hat", "widehat":
			return s + "̂"
		case "bar", "overline":
			return s + "̄"
		case "underline":
			return s + "̲"
		case "tilde":
			return s + "̃"
		default:
			return s + "̇"
		}
	case "left", "right", "big", "Big", "bigg", "Bigg", "bigl", "bigr", "Bigl", "Bigr", "displaystyle", "textstyle", "limits", "nolimits":
		if name == "left" || name == "right" {
			// the delimiter itself follows; keep it, drop the command
			return p.delimiter()
		}
		return ""
	case "quad":
		return "   "
	case "qquad":
		return "      "
	case "thinspace":
		return " "
	case "enspace":
		return "  "
	case "begin", "end":
		return p.environment()
	case "over", "atop":
		return " "
	}
	if s, ok := mathSymbols[name]; ok {
		return s
	}
	return ""
}

// delimiter reads the character after \left or \right.
func (p *mathParser) delimiter() string {
	if p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		switch c {
		case '(':
			return "("
		case '[':
			return "["
		case '{':
			return "{"
		case '|':
			return "|"
		case '.':
			return ""
		default:
			return string(c)
		}
	}
	return ""
}

// environment renders matrix-like environments as rows of values.
func (p *mathParser) environment() string {
	env := ""
	if p.pos < len(p.src) && p.src[p.pos] == '{' {
		p.pos++
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != '}' {
			p.pos++
		}
		env = p.src[start:p.pos]
		if p.pos < len(p.src) {
			p.pos++
		}
	}
	if env != "end" {
		// skip the body up to \end{...}
		for p.pos < len(p.src) {
			if p.src[p.pos] == '\\' && p.pos+4 < len(p.src) && p.src[p.pos+1:p.pos+4] == "end" {
				p.pos++
				break
			}
			p.pos++
		}
		// consume \end{...}
		if p.pos < len(p.src) && p.src[p.pos] == '{' {
			p.pos++
			for p.pos < len(p.src) && p.src[p.pos] != '}' {
				p.pos++
			}
			p.pos++
		}
		return ""
	}
	return ""
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// fraction renders a fraction as a/b, with parentheses when needed.
func fraction(num, den string) string {
	if num == "" {
		num = "0"
	}
	if den == "" {
		den = "0"
	}
	if strings.ContainsAny(num, " +-/") || strings.ContainsAny(den, " +-/") {
		return "(" + num + ")/(" + den + ")"
	}
	return num + "/" + den
}

// superscript converts text to Unicode superscripts where possible.
func superscript(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if v, ok := superMap[r]; ok {
			b.WriteString(v)
			continue
		}
		return "^(" + s + ")"
	}
	return b.String()
}

// subscript converts text to Unicode subscripts where possible.
func subscript(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if v, ok := subMap[r]; ok {
			b.WriteString(v)
			continue
		}
		return "_(" + s + ")"
	}
	return b.String()
}

// mapRunes maps letters through a table (used for \mathbb and \mathcal).
func mapRunes(s string, table map[rune]rune) string {
	var b strings.Builder
	for _, r := range s {
		if v, ok := table[r]; ok {
			b.WriteRune(v)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
