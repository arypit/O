// Plan search: use MCTS and adaptive MCMC to find good sequences of tool
// actions for a goal.
//
// A plan is a sequence of PlanStep. The heuristic score of a plan is
//
//	score = 2*coverage + 0.2*reads - 0.1*writes - 0.05*bash - 0.02*len
//
// where coverage is the fraction of goal keywords (lowercase tokens of
// length >= 3) that appear in the plan's actions. Reads help coverage,
// writes and shell calls are mildly penalized (risk), length is penalized
// to keep plans short.
//
// MCTS searches plan space directly (discrete). AMCMC samples plans through
// a continuous relaxation: x in R^n (n = number of candidates), a candidate
// i is included when sigmoid(x_i) > 0.5; the plan is the included candidates
// sorted by sigmoid(x_i) descending, capped at MaxLen. The target density is
// pi(x) proportional to exp(score(plan(x))), so high-scoring plans are
// sampled more often.
package search

import (
	"math"
	"math/rand"
	"sort"
	"strings"
)

type PlanStep struct {
	Tool    string `json:"tool"`
	Summary string `json:"summary,omitempty"`
}

func (p PlanStep) String() string {
	if p.Summary == "" {
		return p.Tool
	}
	return p.Tool + "(" + p.Summary + ")"
}

type Plan []PlanStep

func (p Plan) String() string {
	var parts []string
	for _, s := range p {
		parts = append(parts, s.String())
	}
	return strings.Join(parts, " -> ")
}

func goalWords(goal string) []string {
	var words []string
	for _, w := range strings.Fields(strings.ToLower(goal)) {
		w = strings.Trim(w, ".,;:!?()[]{}<>\"'`")
		if len(w) >= 3 {
			words = append(words, w)
		}
	}
	return words
}

// ScorePlan is the heuristic reward of a plan for a goal.
func ScorePlan(p Plan, goal string) float64 {
	words := goalWords(goal)
	if len(words) == 0 {
		return 0
	}
	var hay strings.Builder
	reads, writes, bash := 0, 0, 0
	for _, s := range p {
		hay.WriteString(" ")
		hay.WriteString(strings.ToLower(s.Tool + " " + s.Summary))
		switch s.Tool {
		case "read", "glob", "grep", "webfetch":
			reads++
		case "write", "edit":
			writes++
		case "bash":
			bash++
		}
	}
	h := hay.String()
	hit := 0
	for _, w := range words {
		if strings.Contains(h, w) {
			hit++
		}
	}
	coverage := float64(hit) / float64(len(words))
	return 2*coverage + 0.2*float64(reads) - 0.1*float64(writes) - 0.05*float64(bash) - 0.02*float64(len(p))
}

// PlanGame adapts plan sequences to the MCTS Game interface.
type PlanGame struct {
	Goal       string
	Candidates []PlanStep
	MaxLen     int
}

func (g PlanGame) Actions(s Plan) []string {
	if g.Terminal(s) {
		return nil
	}
	used := map[string]bool{}
	for _, st := range s {
		used[st.String()] = true
	}
	var out []string
	for _, c := range g.Candidates {
		if !used[c.String()] {
			out = append(out, c.String())
		}
	}
	return out
}

func (g PlanGame) Apply(s Plan, action string) Plan {
	for _, c := range g.Candidates {
		if c.String() == action {
			return append(append(Plan(nil), s...), c)
		}
	}
	return s
}

func (g PlanGame) Terminal(s Plan) bool { return len(s) >= g.MaxLen }

func (g PlanGame) Reward(s Plan) float64 { return ScorePlan(s, g.Goal) }

// PlanWithMCTS searches for the best plan with MCTS and follows the most
// visited children greedily from the root.
func PlanWithMCTS(goal string, candidates []PlanStep, iterations int, rng *rand.Rand) Plan {
	g := PlanGame{Goal: goal, Candidates: candidates, MaxLen: 8}
	root := RunMCTS(Plan(nil), g, iterations, math.Sqrt2, rng)
	var plan Plan
	node := root
	for {
		child := BestChildByVisits(node)
		if child == nil {
			break
		}
		plan = append(plan, g.Apply(plan, child.Action)[len(plan):]...)
		node = child
	}
	if len(plan) == 0 && len(candidates) > 0 {
		plan = Plan{candidates[0]}
	}
	return plan
}

// PlanDensity is the continuous relaxation used by AMCMC: x in R^n,
// candidate i included when sigmoid(x_i) > 0.5.
type PlanDensity struct {
	Goal       string
	Candidates []PlanStep
	MaxLen     int
}

func sigmoid(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

func (d PlanDensity) decode(x []float64) Plan {
	type scored struct {
		s    float64
		step PlanStep
	}
	var inc []scored
	for i, c := range d.Candidates {
		if i < len(x) && sigmoid(x[i]) > 0.5 {
			inc = append(inc, scored{sigmoid(x[i]), c})
		}
	}
	sort.SliceStable(inc, func(i, j int) bool { return inc[i].s > inc[j].s })
	maxLen := d.MaxLen
	if maxLen <= 0 {
		maxLen = 8
	}
	if len(inc) > maxLen {
		inc = inc[:maxLen]
	}
	out := make(Plan, 0, len(inc))
	for _, v := range inc {
		out = append(out, v.step)
	}
	return out
}

// LogDensity is log pi(x) = score(plan(x)) (up to an additive constant).
func (d PlanDensity) LogDensity(x []float64) float64 {
	return ScorePlan(d.decode(x), d.Goal)
}

// SamplePlansAMCMC samples diverse plans with adaptive MCMC.
func SamplePlansAMCMC(goal string, candidates []PlanStep, iters, burnin int, rng *rand.Rand) []Plan {
	d := PlanDensity{Goal: goal, Candidates: candidates, MaxLen: 8}
	start := make([]float64, len(candidates))
	ch := AdaptiveMetropolis(d, start, iters, burnin, iters/2, rng)
	seen := map[string]bool{}
	var out []Plan
	for _, s := range ch.Samples {
		p := d.decode(s)
		key := p.String()
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

// RankPlans sorts plans by score descending.
func RankPlans(plans []Plan, goal string) []Plan {
	out := append([]Plan(nil), plans...)
	sort.SliceStable(out, func(i, j int) bool { return ScorePlan(out[i], goal) > ScorePlan(out[j], goal) })
	return out
}
