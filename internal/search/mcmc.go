// Metropolis-Hastings MCMC.
//
// Given a target density pi(x) (known up to a constant, via LogDensity) and a
// proposal q(x'|x), each step proposes x' ~ q(.|x) and accepts with
//
//	alpha(x, x') = min(1, pi(x') q(x|x') / (pi(x) q(x'|x)))
//
// In log space: accept when log(u) < logpi(x') + logq(x|x') - logpi(x) - logq(x'|x)
// for u ~ Uniform(0,1). The chain's stationary distribution is pi.
package search

import (
	"errors"
	"math"
	"math/rand"
)

// Density is a target distribution known up to a normalizing constant.
type Density interface {
	LogDensity(x []float64) float64
}

// Proposal is a Markov kernel q(to|from).
type Proposal interface {
	Propose(from []float64, rng *rand.Rand) []float64
	LogProb(from, to []float64) float64
}

// Chain is the output of a sampler.
type Chain struct {
	Samples    [][]float64
	AcceptRate float64
}

// GaussianProposal is a random-walk proposal to = from + L*z, z ~ N(0, I),
// where L is the lower Cholesky factor of the covariance Sigma.
type GaussianProposal struct {
	L      [][]float64
	LogDet float64 // log det(Sigma) = 2 * sum(log(diag(L)))
	Dim    int
}

// Cholesky computes the lower triangular L with A = L*L^T (A must be SPD).
func Cholesky(a [][]float64) ([][]float64, error) {
	n := len(a)
	l := make([][]float64, n)
	for i := range l {
		l[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum := a[i][j]
			for k := 0; k < j; k++ {
				sum -= l[i][k] * l[j][k]
			}
			if i == j {
				if sum <= 0 {
					return nil, errors.New("matrix not positive definite")
				}
				l[i][j] = math.Sqrt(sum)
			} else {
				l[i][j] = sum / l[j][j]
			}
		}
	}
	return l, nil
}

func NewGaussianProposal(sigma [][]float64) (*GaussianProposal, error) {
	l, err := Cholesky(sigma)
	if err != nil {
		return nil, err
	}
	logDet := 0.0
	for i := range l {
		logDet += math.Log(l[i][i])
	}
	return &GaussianProposal{L: l, LogDet: 2 * logDet, Dim: len(l)}, nil
}

func (g *GaussianProposal) Propose(from []float64, rng *rand.Rand) []float64 {
	to := make([]float64, g.Dim)
	for i := 0; i < g.Dim; i++ {
		to[i] = from[i]
		for j := 0; j <= i; j++ {
			to[i] += g.L[i][j] * randNormal(rng)
		}
	}
	return to
}

// LogProb is log N(to; from, Sigma) for the Gaussian random walk.
// = -0.5 * (d*log(2pi) + logdet(Sigma) + (to-from)^T Sigma^-1 (to-from))
func (g *GaussianProposal) LogProb(from, to []float64) float64 {
	d := make([]float64, g.Dim)
	for i := range d {
		d[i] = to[i] - from[i]
	}
	// solve L y = d, then maha^2 = y.y  (since Sigma^-1 = L^-T L^-1)
	y := make([]float64, g.Dim)
	for i := 0; i < g.Dim; i++ {
		sum := d[i]
		for j := 0; j < i; j++ {
			sum -= g.L[i][j] * y[j]
		}
		y[i] = sum / g.L[i][i]
	}
	maha2 := 0.0
	for _, v := range y {
		maha2 += v * v
	}
	return -0.5 * (float64(g.Dim)*math.Log(2*math.Pi) + g.LogDet + maha2)
}

func randNormal(rng *rand.Rand) float64 {
	// Box-Muller transform
	u1 := rng.Float64()
	if u1 < 1e-12 {
		u1 = 1e-12
	}
	u2 := rng.Float64()
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

// MetropolisHastings runs iters steps, discards the first burnin samples.
func MetropolisHastings(d Density, q Proposal, start []float64, iters, burnin int, rng *rand.Rand) Chain {
	x := append([]float64(nil), start...)
	logp := d.LogDensity(x)
	accepted := 0
	var samples [][]float64
	for i := 0; i < iters; i++ {
		x2 := q.Propose(x, rng)
		logp2 := d.LogDensity(x2)
		logAlpha := logp2 + q.LogProb(x2, x) - logp - q.LogProb(x, x2)
		if math.Log(rng.Float64()+1e-300) < logAlpha {
			x = x2
			logp = logp2
			accepted++
		}
		if i >= burnin {
			samples = append(samples, append([]float64(nil), x...))
		}
	}
	return Chain{Samples: samples, AcceptRate: float64(accepted) / float64(iters)}
}
