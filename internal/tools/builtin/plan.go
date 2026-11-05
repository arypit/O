package builtin

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/adaop/o/internal/search"
	"github.com/adaop/o/internal/tools"
)

// PlanTool searches the space of candidate tool actions for a good plan.
// Method "mcts" runs Monte Carlo Tree Search over plan sequences (UCB1);
// method "amcmc" samples plans with adaptive MCMC over a continuous
// relaxation of the action-subset selection. See internal/search.
type PlanTool struct {
	Candidates []search.PlanStep
}

func (PlanTool) Name() string { return "plan" }
func (PlanTool) Description() string {
	return "Search for a good sequence of actions for a goal, using MCTS or adaptive MCMC, and return a ranked plan."
}
func (t PlanTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal":       map[string]any{"type": "string"},
			"method":     map[string]any{"type": "string", "enum": []any{"mcts", "amcmc"}},
			"iterations": map[string]any{"type": "integer"},
		},
		"required":             []any{"goal"},
		"additionalProperties": false,
	}
}
func (PlanTool) Mutating() bool { return false }
func (t PlanTool) Summarize(args map[string]any) string {
	g, _ := args["goal"].(string)
	if len(g) > 60 {
		g = g[:60]
	}
	return g
}

func (t PlanTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	goal, _ := args["goal"].(string)
	method, _ := args["method"].(string)
	if method == "" {
		method = "mcts"
	}
	iterations := 200
	if v, ok := args["iterations"].(float64); ok && v > 0 {
		iterations = int(v)
	}
	rng := rand.New(rand.NewSource(42))
	var plans []search.Plan
	if method == "amcmc" {
		plans = search.SamplePlansAMCMC(goal, t.Candidates, iterations, iterations/2, rng)
	} else {
		p := search.PlanWithMCTS(goal, t.Candidates, iterations, rng)
		plans = []search.Plan{p}
		plans = append(plans, search.SamplePlansAMCMC(goal, t.Candidates, iterations/4, iterations/8, rng)...)
	}
	plans = search.RankPlans(plans, goal)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("plan for %q (method=%s):\n", goal, method))
	for i, p := range plans {
		if i >= 3 {
			break
		}
		fmt.Fprintf(&b, "option %d (score %.3f):\n", i+1, search.ScorePlan(p, goal))
		for j, step := range p {
			fmt.Fprintf(&b, "  %d. %s\n", j+1, step.String())
		}
	}
	return tools.ToolResult{Content: b.String()}, nil
}
