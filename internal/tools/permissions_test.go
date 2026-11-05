package tools

import "testing"

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, s string
		want       bool
	}{
		{"git *", "git status", true},
		{"git *", "git status --porcelain", true},
		{"git *", "npm install", false},
		{"*", "anything", true},
		{"edit", "edit", true},
		{"edit", "editx", false},
		{"*.go", "main.go", true},
		{"*.go", "main.ts", false},
	}
	for _, c := range cases {
		if got := GlobMatch(c.pattern, c.s); got != c.want {
			t.Errorf("GlobMatch(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

func TestRuleMatches(t *testing.T) {
	r := Rule{Pattern: "bash(git *)", Decision: Allow}
	if !r.Matches("bash", "git status") {
		t.Error("expected bash(git *) to match bash with 'git status'")
	}
	if r.Matches("bash", "rm -rf /") {
		t.Error("expected bash(git *) to not match 'rm -rf /'")
	}
	if r.Matches("edit", "git status") {
		t.Error("tool name mismatch should not match")
	}
	whole := Rule{Pattern: "edit", Decision: Allow}
	if !whole.Matches("edit", "anything") || whole.Matches("write", "x") {
		t.Error("bare tool pattern should match only that tool")
	}
}

func TestPolicyModes(t *testing.T) {
	p := NewPolicy(ModePlan, nil)
	if got := p.Check("bash", "ls", true, 0.5, 0); got != Deny {
		t.Errorf("plan mode: mutating bash = %v, want deny", got)
	}
	if got := p.Check("read", "x.go", false, 0.5, 0); got != Allow {
		t.Errorf("plan mode: read = %v, want allow", got)
	}
	p = NewPolicy(ModeFullAuto, nil)
	if got := p.Check("bash", "rm -rf /", true, 0, 0); got != Allow {
		t.Errorf("full-auto: bash = %v, want allow (sandbox still applies)", got)
	}
	p = NewPolicy(ModeAutoEdit, nil)
	if got := p.Check("edit", "a.go", true, 0, 0); got != Ask {
		t.Errorf("auto-edit: edit = %v, want ask", got)
	}
	p = NewPolicy(ModeDefault, nil)
	if got := p.Check("read", "a.go", false, 0, 0); got != Allow {
		t.Errorf("default: read = %v, want allow", got)
	}
	if got := p.Check("write", "a.go", true, 0, 0); got != Ask {
		t.Errorf("default: write = %v, want ask", got)
	}
}

func TestPolicyRulesFirst(t *testing.T) {
	rules := map[string][]Rule{
		"bash": {{Pattern: "bash(git *)", Decision: Allow}, {Pattern: "*", Decision: Ask}},
	}
	p := NewPolicy(ModeDefault, rules)
	if got := p.Check("bash", "git status", true, 0, 0); got != Allow {
		t.Errorf("rule bash(git *) = %v, want allow", got)
	}
	if got := p.Check("bash", "npm test", true, 0, 0); got != Ask {
		t.Errorf("rule fallback = %v, want ask", got)
	}
}

func TestPolicyLearnedPrior(t *testing.T) {
	p := NewPolicy(ModeDefault, nil)
	p.LearnedMinSamples = 10
	p.LearnedMinProb = 0.9
	if got := p.Check("write", "a.go", true, 0.95, 20); got != Allow {
		t.Errorf("learned prior should auto-allow: got %v", got)
	}
	if got := p.Check("write", "a.go", true, 0.95, 5); got != Ask {
		t.Errorf("too few samples should stay ask: got %v", got)
	}
	if got := p.Check("write", "a.go", true, 0.5, 20); got != Ask {
		t.Errorf("low prob should stay ask: got %v", got)
	}
}

func TestParseRuleList(t *testing.T) {
	rules := ParseRuleList([]any{"git *", map[string]any{"pattern": "rm *", "decision": "deny"}, "allow"})
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(rules))
	}
	if rules[2].Decision != Allow || rules[2].Pattern != "*" {
		t.Errorf("shorthand allow = %+v", rules[2])
	}
	if rules[1].Decision != Deny {
		t.Errorf("object rule = %+v", rules[1])
	}
}
