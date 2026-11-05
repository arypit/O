// Human-in-the-loop learning.
//
// Every time a human approves or rejects a tool call, we record the outcome
// under a key (tool name, or tool|summary). Approval of a call with prior
// Beta(a, b) is modeled as a Bernoulli trial, so the posterior is conjugate:
//
//	approve -> Beta(a+1, b)
//	reject  -> Beta(a, b+1)
//
// Posterior mean     = a / (a + b)
// Posterior variance = a*b / ((a+b)^2 * (a+b+1))
//
// The permission policy uses the posterior mean (with a minimum sample
// count) to auto-approve calls the human has reliably accepted before.
// The prior is Beta(1, 1) (uniform), i.e. we start undecided.
package hitl

import (
	"encoding/json"
	"os"
	"sync"
)

type BetaStat struct {
	Alpha float64 `json:"alpha"`
	Beta  float64 `json:"beta"`
}

// Mean is the posterior acceptance probability E[p] = a/(a+b).
func (s BetaStat) Mean() float64 {
	d := s.Alpha + s.Beta
	if d == 0 {
		return 0.5
	}
	return s.Alpha / d
}

// Variance is Var[p] = a*b / ((a+b)^2*(a+b+1)).
func (s BetaStat) Variance() float64 {
	a, b := s.Alpha, s.Beta
	d := a + b
	if d == 0 {
		return 0
	}
	return a * b / (d * d * (d + 1))
}

// Samples is the number of observations behind the posterior.
func (s BetaStat) Samples() float64 { return s.Alpha + s.Beta - 2 } // prior Beta(1,1) contributes 2

type Learner struct {
	mu    sync.Mutex
	path  string
	stats map[string]BetaStat
}

func NewLearner(path string) *Learner {
	l := &Learner{path: path, stats: map[string]BetaStat{}}
	l.load()
	return l
}

func (l *Learner) load() {
	if l.path == "" {
		return
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &l.stats)
}

// Save persists the learner state.
func (l *Learner) Save() error {
	if l.path == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := json.MarshalIndent(l.stats, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.path, data, 0o644)
}

// Record notes a human decision for key. accepted=true counts as alpha+1.
func (l *Learner) Record(key string, accepted bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.stats[key]
	if s.Alpha == 0 && s.Beta == 0 {
		s = BetaStat{Alpha: 1, Beta: 1} // uniform prior
	}
	if accepted {
		s.Alpha++
	} else {
		s.Beta++
	}
	l.stats[key] = s
}

// Prob returns the posterior mean acceptance probability for key.
func (l *Learner) Prob(key string) float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.stats[key]
	if s.Alpha == 0 && s.Beta == 0 {
		return 0.5
	}
	return s.Mean()
}

// Samples returns how many human decisions were recorded for key.
func (l *Learner) Samples(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.stats[key]
	n := int(s.Samples())
	if n < 0 {
		return 0
	}
	return n
}

// Stats returns a snapshot of all keys (for inspection/tests).
func (l *Learner) Stats() map[string]BetaStat {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]BetaStat, len(l.stats))
	for k, v := range l.stats {
		out[k] = v
	}
	return out
}
