package hitl

import (
	"math"
	"testing"
)

func TestBetaBernoulliPosterior(t *testing.T) {
	l := NewLearner("") // no persistence
	// prior Beta(1,1): mean 0.5, var = 1*1/(2^2*3) = 1/12
	p0 := BetaStat{Alpha: 1, Beta: 1}
	if math.Abs(p0.Mean()-0.5) > 1e-12 {
		t.Errorf("prior mean = %v, want 0.5", p0.Mean())
	}
	if math.Abs(p0.Variance()-1.0/12) > 1e-12 {
		t.Errorf("prior var = %v, want 1/12", p0.Variance())
	}
	// 9 accepts, 1 reject -> Beta(10, 2): mean 10/12, var 10*2/(12^2*13)
	for i := 0; i < 9; i++ {
		l.Record("bash", true)
	}
	l.Record("bash", false)
	if got := l.Prob("bash"); math.Abs(got-10.0/12) > 1e-12 {
		t.Errorf("posterior mean = %v, want 10/12", got)
	}
	s := l.Stats()["bash"]
	wantVar := 10.0 * 2 / (12 * 12 * 13)
	if math.Abs(s.Variance()-wantVar) > 1e-12 {
		t.Errorf("posterior var = %v, want %v", s.Variance(), wantVar)
	}
	if l.Samples("bash") != 10 {
		t.Errorf("samples = %d, want 10", l.Samples("bash"))
	}
	// unseen key: uniform prior mean 0.5, 0 samples
	if got := l.Prob("edit"); got != 0.5 {
		t.Errorf("unseen key mean = %v, want 0.5", got)
	}
}

func TestAutoDecider(t *testing.T) {
	l := NewLearner("")
	d := NewAutoDecider(l)
	d.MinSamples = 10
	d.MinProb = 0.9
	ok, _ := d.Ask("write", "a.go")
	if ok {
		t.Error("auto decider must deny without enough samples")
	}
	for i := 0; i < 12; i++ {
		l.Record("write", true)
	}
	ok, _ = d.Ask("write", "a.go")
	if !ok {
		t.Error("auto decider should allow after 12 accepts (mean 13/14 >= 0.9)")
	}
}
