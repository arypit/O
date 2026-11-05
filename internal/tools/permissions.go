// Permission policy for tool calls.
//
// A rule pattern looks like "bash(git *)" (tool plus a glob over the argument
// summary) or just "edit" (whole tool). Rules are evaluated in order; the
// first matching rule wins. If no rule matches, the mode decides:
//
//   default   - mutating tools ask, read-only tools allow
//   auto-edit - file edits allow, shell asks
//   plan      - everything mutating is denied (read-only planning)
//   full-auto - everything allows (still inside the sandbox)
//
// When the decision is "ask" and the human-in-the-loop learner has enough
// samples with a high acceptance rate, the call is auto-approved.
package tools

import (
	"regexp"
	"strings"
	"sync"
)

type Mode string

const (
	ModeDefault  Mode = "default"
	ModeAutoEdit Mode = "auto-edit"
	ModePlan     Mode = "plan"
	ModeFullAuto Mode = "full-auto"
)

type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)

type Rule struct {
	Pattern  string
	Decision Decision
}

type Policy struct {
	Mode  Mode
	Rules map[string][]Rule // tool name -> ordered rules
	// LearnedAutoApprove: minimum samples and acceptance probability to
	// auto-approve an "ask" decision from human feedback history.
	LearnedMinSamples int
	LearnedMinProb    float64
	// mu guards Mode: the UI can switch the mode while the agent runs.
	mu sync.RWMutex
	// SensitiveProtection forces an approval for destructive operations,
	// even while full access is active.
	sensitive bool
}

// SetMode switches the permission mode at runtime.
func (p *Policy) SetMode(m Mode) {
	p.mu.Lock()
	p.Mode = m
	p.mu.Unlock()
}

// SetSensitiveProtection turns the extra confirmation for destructive
// operations on or off.
func (p *Policy) SetSensitiveProtection(on bool) {
	p.mu.Lock()
	p.sensitive = on
	p.mu.Unlock()
}

// SensitiveProtection reports whether destructive operations always ask.
func (p *Policy) SensitiveProtection() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sensitive
}

// destructiveCommands are command fragments that always need an approval
// when sensitive operations protection is on.
var destructiveCommands = []string{
	"rm -rf", "rm -fr", "rm -r ", "mkfs", "format ", "diskpart", "del /s", "rd /s",
	"remove-item", "rmdir /s", "shutdown", "reg delete", "git push --force", "git push -f",
	"git reset --hard", "chmod 777", "dd if=", "curl ", "wget ", "invoke-expression",
}

// IsDestructive reports whether a command summary looks destructive.
func IsDestructive(summary string) bool {
	s := strings.ToLower(summary)
	for _, p := range destructiveCommands {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// CurrentMode returns the mode in effect.
func (p *Policy) CurrentMode() Mode {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Mode
}

func NewPolicy(mode Mode, rules map[string][]Rule) *Policy {
	if mode == "" {
		mode = ModeDefault
	}
	return &Policy{Mode: mode, Rules: rules, LearnedMinSamples: 10, LearnedMinProb: 0.9}
}

// ParseRuleList converts config rules (string, or list mixing strings and
// {"pattern","decision"} objects) into ordered Rules for one tool.
func ParseRuleList(v any) []Rule {
	var out []Rule
	switch t := v.(type) {
	case string:
		out = append(out, ruleFromString(t))
	case []any:
		for _, item := range t {
			switch it := item.(type) {
			case string:
				out = append(out, ruleFromString(it))
			case map[string]any:
				pat, _ := it["pattern"].(string)
				dec, _ := it["decision"].(string)
				if pat != "" && dec != "" {
					out = append(out, Rule{Pattern: pat, Decision: Decision(dec)})
				}
			}
		}
	}
	return out
}

// ruleFromString maps a shorthand string to a rule:
// "allow"/"deny"/"ask" apply to the whole tool (pattern "*"), anything
// else is treated as a pattern.
func ruleFromString(s string) Rule {
	switch s {
	case "allow", "deny", "ask":
		return Rule{Pattern: "*", Decision: Decision(s)}
	default:
		return Rule{Pattern: s, Decision: Ask}
	}
}

// GlobMatch matches s against a glob where '*' matches any sequence.
func GlobMatch(pattern, s string) bool {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		if r == '*' {
			b.WriteString(".*")
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

// Matches reports whether a rule pattern matches a tool call. Patterns with
// parentheses match "tool(summary)"; bare patterns match the tool name.
func (r Rule) Matches(tool, summary string) bool {
	p := r.Pattern
	if idx := strings.Index(p, "("); idx >= 0 && strings.HasSuffix(p, ")") {
		name := p[:idx]
		inner := p[idx+1 : len(p)-1]
		if name != tool {
			return false
		}
		return GlobMatch(inner, summary)
	}
	if p == "*" {
		return true
	}
	return p == tool
}

// Check decides what to do with a tool call.
// learnedProb/learnedSamples come from the HITL Beta-Bernoulli learner.
func (p *Policy) Check(tool, summary string, mutating bool, learnedProb float64, learnedSamples int) Decision {
	// a destructive operation always asks when the protection is on
	if p.SensitiveProtection() && tool == "bash" && IsDestructive(summary) {
		return Ask
	}
	for _, rule := range p.Rules[tool] {
		if rule.Matches(tool, summary) {
			return rule.Decision
		}
	}
	switch p.CurrentMode() {
	case ModePlan:
		if mutating {
			return Deny
		}
		return Allow
	case ModeFullAuto:
		return Allow
	case ModeAutoEdit:
		if mutating {
			return Ask
		}
		return Allow
	default: // default
		if mutating {
			d := Ask
			if learnedSamples >= p.LearnedMinSamples && learnedProb >= p.LearnedMinProb {
				d = Allow
			}
			return d
		}
		return Allow
	}
}

// SummaryFor builds the argument summary used in rules like "bash(git *)".
func SummaryFor(tool string, args map[string]any) string {
	if c, ok := args["command"].(string); ok {
		return c
	}
	for _, key := range []string{"file_path", "path", "url", "pattern"} {
		if v, ok := args[key].(string); ok {
			return v
		}
	}
	return ""
}
