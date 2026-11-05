// Monte Carlo Tree Search with the UCB1 selection rule.
//
// Each node is a state. Selection descends by UCB1:
//
//	score(child) = value(child)/visits(child) + C * sqrt( ln(visits(parent)) / visits(child) )
//
// The first term is exploitation (mean reward), the second is exploration.
// C = sqrt(2) ~ 1.41 is the classic constant for rewards in [0, 1].
// Expansion adds one untried child, simulation rolls out a random policy to
// a terminal state, and backpropagation adds the reward to every node on the
// path. The recommended action is the root child with the most visits
// (the robust-child rule), not the one with the highest mean.
package search

import (
	"math"
	"math/rand"
)

// Game is the search space. S is the state type, actions are strings.
type Game[S any] interface {
	Actions(s S) []string
	Apply(s S, action string) S
	Terminal(s S) bool
	Reward(s S) float64
}

type Node[S any] struct {
	State    S
	Action   string
	Parent   *Node[S]
	Children map[string]*Node[S]
	Untried  []string
	Visits   int
	Value    float64
}

const maxRolloutDepth = 50

// UCB1 is the selection score of node n relative to its parent.
func UCB1[S any](n *Node[S], c float64) float64 {
	if n.Visits == 0 {
		return math.Inf(1)
	}
	exploit := n.Value / float64(n.Visits)
	explore := c * math.Sqrt(math.Log(float64(n.Parent.Visits))/float64(n.Visits))
	return exploit + explore
}

// RunMCTS runs iterations of select/expand/simulate/backpropagate from root
// and returns the root node with populated statistics.
func RunMCTS[S any](root S, g Game[S], iterations int, c float64, rng *rand.Rand) *Node[S] {
	rootNode := &Node[S]{State: root, Children: map[string]*Node[S]{}, Untried: g.Actions(root)}
	for i := 0; i < iterations; i++ {
		node := rootNode
		// selection: descend while fully expanded
		for len(node.Untried) == 0 && len(node.Children) > 0 {
			node = bestChild(node, c)
		}
		// expansion: add one untried child
		if len(node.Untried) > 0 {
			idx := rng.Intn(len(node.Untried))
			action := node.Untried[idx]
			node.Untried = append(node.Untried[:idx], node.Untried[idx+1:]...)
			child := &Node[S]{
				State:    g.Apply(node.State, action),
				Action:   action,
				Parent:   node,
				Children: map[string]*Node[S]{},
				Untried:  g.Actions(g.Apply(node.State, action)),
			}
			node.Children[action] = child
			node = child
		}
		// simulation: random rollout from node.State
		s := node.State
		for depth := 0; !g.Terminal(s) && depth < maxRolloutDepth; depth++ {
			acts := g.Actions(s)
			if len(acts) == 0 {
				break
			}
			s = g.Apply(s, acts[rng.Intn(len(acts))])
		}
		reward := g.Reward(s)
		// backpropagation
		for n := node; n != nil; n = n.Parent {
			n.Visits++
			n.Value += reward
		}
	}
	return rootNode
}

func bestChild[S any](n *Node[S], c float64) *Node[S] {
	var best *Node[S]
	bestScore := math.Inf(-1)
	for _, child := range n.Children {
		s := UCB1(child, c)
		if s > bestScore {
			bestScore = s
			best = child
		}
	}
	return best
}

// BestChildByVisits returns the child with the most visits (robust child).
func BestChildByVisits[S any](n *Node[S]) *Node[S] {
	var best *Node[S]
	for _, child := range n.Children {
		if best == nil || child.Visits > best.Visits {
			best = child
		}
	}
	return best
}
