package search

import (
	"math"
	"math/rand"
	"testing"
)

// simpleGame: 3 picks, action "good" adds 1 to the score, "bad" adds 0.
// The optimal policy is always "good", so MCTS must visit it most.
type simpleGame struct{}

type simpleState struct {
	steps int
	score float64
}

func (simpleGame) Actions(s simpleState) []string {
	if s.steps >= 3 {
		return nil
	}
	return []string{"good", "bad"}
}
func (g simpleGame) Apply(s simpleState, a string) simpleState {
	if a == "good" {
		s.score++
	}
	s.steps++
	return s
}
func (simpleGame) Terminal(s simpleState) bool  { return s.steps >= 3 }
func (simpleGame) Reward(s simpleState) float64 { return s.score / 3 }

func TestMCTSUcb1PicksBestAction(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	root := RunMCTS(simpleState{}, simpleGame{}, 500, math.Sqrt2, rng)
	best := BestChildByVisits(root)
	if best == nil || best.Action != "good" {
		t.Fatalf("MCTS best action = %v, want good", best)
	}
	// UCB1 sanity: unvisited node has infinite score
	n := &Node[simpleState]{Visits: 0, Value: 0, Parent: &Node[simpleState]{Visits: 10}}
	if !math.IsInf(UCB1(n, 1.41), 1) {
		t.Error("unvisited node should have +Inf UCB1")
	}
	// UCB1 formula check: value/visits + c*sqrt(ln(parent)/visits)
	n = &Node[simpleState]{Visits: 4, Value: 2, Parent: &Node[simpleState]{Visits: 20}}
	want := 0.5 + 1.41*math.Sqrt(math.Log(20)/4)
	if math.Abs(UCB1(n, 1.41)-want) > 1e-12 {
		t.Errorf("UCB1 = %v, want %v", UCB1(n, 1.41), want)
	}
}

// stdNormal is the N(0,1) density up to a constant.
type stdNormal struct{}

func (stdNormal) LogDensity(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v * v
	}
	return -0.5 * s
}

func TestCholesky(t *testing.T) {
	a := [][]float64{{4, 2}, {2, 3}}
	l, err := Cholesky(a)
	if err != nil {
		t.Fatal(err)
	}
	// L*L^T must equal A
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			sum := 0.0
			for k := 0; k < 2; k++ {
				sum += l[i][k] * l[j][k]
			}
			if math.Abs(sum-a[i][j]) > 1e-12 {
				t.Errorf("L*L^T[%d][%d] = %v, want %v", i, j, sum, a[i][j])
			}
		}
	}
}

func TestMetropolisHastingsSamplesNormal(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	prop, err := NewGaussianProposal([][]float64{{1.0}})
	if err != nil {
		t.Fatal(err)
	}
	ch := MetropolisHastings(stdNormal{}, prop, []float64{5.0}, 30000, 3000, rng)
	if len(ch.Samples) == 0 {
		t.Fatal("no samples")
	}
	mean, variance := 0.0, 0.0
	for _, s := range ch.Samples {
		mean += s[0]
	}
	mean /= float64(len(ch.Samples))
	for _, s := range ch.Samples {
		variance += (s[0] - mean) * (s[0] - mean)
	}
	variance /= float64(len(ch.Samples))
	if math.Abs(mean) > 0.1 {
		t.Errorf("MH mean = %v, want ~0", mean)
	}
	if math.Abs(variance-1) > 0.2 {
		t.Errorf("MH variance = %v, want ~1", variance)
	}
	if ch.AcceptRate < 0.2 || ch.AcceptRate > 0.9 {
		t.Errorf("MH acceptance = %v, want in [0.2, 0.9]", ch.AcceptRate)
	}
}

func TestAdaptiveMetropolis(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	ch := AdaptiveMetropolis(stdNormal2{}, []float64{3, -3}, 30000, 3000, 15000, rng)
	if len(ch.Samples) == 0 {
		t.Fatal("no samples")
	}
	dim := 2
	mean := make([]float64, dim)
	for _, s := range ch.Samples {
		for i := 0; i < dim; i++ {
			mean[i] += s[i]
		}
	}
	for i := range mean {
		mean[i] /= float64(len(ch.Samples))
		if math.Abs(mean[i]) > 0.15 {
			t.Errorf("AMCMC mean[%d] = %v, want ~0", i, mean[i])
		}
	}
	if ch.AcceptRate < 0.1 || ch.AcceptRate > 0.6 {
		t.Errorf("AMCMC acceptance = %v, want near 0.234-ish range", ch.AcceptRate)
	}
	if ch.FinalCov == nil {
		t.Fatal("final covariance not set")
	}
	for i := 0; i < dim; i++ {
		if ch.FinalCov[i][i] <= 0 {
			t.Errorf("FinalCov[%d][%d] = %v, must be positive", i, i, ch.FinalCov[i][i])
		}
		// adapted covariance should be in a sane range around the optimal 2.38^2/2 ~ 2.83 times target var 1
		if ch.FinalCov[i][i] > 10 {
			t.Errorf("FinalCov[%d][%d] = %v, too large", i, i, ch.FinalCov[i][i])
		}
	}
}

type stdNormal2 struct{}

func (stdNormal2) LogDensity(x []float64) float64 { return stdNormal{}.LogDensity(x) }

func TestPlannerMCTS(t *testing.T) {
	candidates := []PlanStep{
		{Tool: "read", Summary: "read(config file)"},
		{Tool: "grep", Summary: "grep(needle in config)"},
		{Tool: "bash", Summary: "bash(rm files)"},
		{Tool: "write", Summary: "write(config file)"},
	}
	rng := rand.New(rand.NewSource(3))
	plan := PlanWithMCTS("read the config file to understand settings", candidates, 400, rng)
	if len(plan) == 0 {
		t.Fatal("empty plan")
	}
	bashPlan := Plan{{Tool: "bash", Summary: "bash(rm files)"}}
	if ScorePlan(plan, "read the config file to understand settings") <= ScorePlan(bashPlan, "read the config file to understand settings") {
		t.Errorf("MCTS plan %v should outscore bash-only plan", plan)
	}
}

func TestPlannerAMCMC(t *testing.T) {
	candidates := []PlanStep{
		{Tool: "read", Summary: "read(config file)"},
		{Tool: "glob", Summary: "glob(**/*.json)"},
		{Tool: "bash", Summary: "bash(rm files)"},
	}
	rng := rand.New(rand.NewSource(5))
	plans := SamplePlansAMCMC("read the config file", candidates, 3000, 500, rng)
	if len(plans) == 0 {
		t.Fatal("AMCMC produced no plans")
	}
	ranked := RankPlans(plans, "read the config file")
	top := ranked[0]
	foundRead := false
	for _, s := range top {
		if s.Tool == "read" {
			foundRead = true
		}
	}
	if !foundRead {
		t.Errorf("top AMCMC plan %v should include read for a read goal", top)
	}
}
