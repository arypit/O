// Adaptive Metropolis MCMC (Haario, Saksman & Tamminen, 2001).
//
// The proposal covariance is adapted from the chain's own history. If the
// chain so far has empirical covariance Cov, the proposal uses
//
//	Sigma_p = (2.38^2 / d) * (Cov + eps * I)
//
// The scaling 2.38^2/d is asymptotically optimal for Gaussian targets and
// yields an acceptance rate near 0.234 in high dimensions. eps*I keeps the
// covariance positive definite early on. Adaptation stops after adaptUntil
// steps so the chain remains Markovian (diminishing adaptation).
package search

import (
	"math"
	"math/rand"
)

type AMChain struct {
	Samples    [][]float64
	AcceptRate float64
	FinalCov   [][]float64
}

// AdaptiveMetropolis runs adaptive MH. start is the initial point,
// iters total steps, burnin discarded from the output, adaptUntil freezes
// adaptation (use iters to never freeze).
func AdaptiveMetropolis(d Density, start []float64, iters, burnin, adaptUntil int, rng *rand.Rand) AMChain {
	dim := len(start)
	x := append([]float64(nil), start...)
	logp := d.LogDensity(x)

	// online mean/covariance via Welford's algorithm
	mean := make([]float64, dim)
	m2 := make([][]float64, dim)
	for i := range m2 {
		m2[i] = make([]float64, dim)
	}
	count := 0

	eps := 1e-6
	accepted := 0
	var samples [][]float64
	var finalCov [][]float64

	for i := 0; i < iters; i++ {
		var prop *GaussianProposal
		var err error
		if i < adaptUntil && count > dim+1 {
			cov := make([][]float64, dim)
			for r := 0; r < dim; r++ {
				cov[r] = make([]float64, dim)
				for c := 0; c < dim; c++ {
					cov[r][c] = m2[r][c] / float64(count-1)
				}
			}
			scale := 2.38 * 2.38 / float64(dim)
			for r := 0; r < dim; r++ {
				for c := 0; c < dim; c++ {
					cov[r][c] *= scale
				}
				cov[r][r] += eps
			}
			prop, err = NewGaussianProposal(cov)
			if err == nil {
				finalCov = cov
			}
		}
		if prop == nil {
			// initial isotropic proposal: (2.38^2/d) * I
			scale := 2.38 * 2.38 / float64(dim)
			sigma := make([][]float64, dim)
			for r := range sigma {
				sigma[r] = make([]float64, dim)
				sigma[r][r] = scale + eps
			}
			prop, _ = NewGaussianProposal(sigma)
		}
		_ = err

		x2 := prop.Propose(x, rng)
		logp2 := d.LogDensity(x2)
		logAlpha := logp2 + prop.LogProb(x2, x) - logp - prop.LogProb(x, x2)
		if math.Log(rng.Float64()+1e-300) < logAlpha {
			x = x2
			logp = logp2
			accepted++
		}
		if i < adaptUntil {
			count++
			delta := make([]float64, dim)
			for j := range x {
				delta[j] = x[j] - mean[j]
				mean[j] += delta[j] / float64(count)
			}
			for r := 0; r < dim; r++ {
				for c := 0; c < dim; c++ {
					m2[r][c] += (x[r] - mean[r]) * delta[c]
				}
			}
		}
		if i >= burnin {
			samples = append(samples, append([]float64(nil), x...))
		}
	}
	return AMChain{Samples: samples, AcceptRate: float64(accepted) / float64(iters), FinalCov: finalCov}
}
