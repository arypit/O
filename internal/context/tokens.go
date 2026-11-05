package context

import "github.com/adaop/o/internal/core"

// msgOverhead approximates the per-message token overhead (role tags etc.).
const msgOverhead = 4

// EstimateTokens approximates token count as ceil(chars/4), the standard
// English-text heuristic for current LLM tokenizers.
func EstimateTokens(s string) int {
	n := (len(s) + 3) / 4
	if n < 1 {
		n = 1
	}
	return n
}

func partTokens(p core.Part) int {
	n := EstimateTokens(p.Text) + EstimateTokens(p.Content) + EstimateTokens(string(p.Input))
	if p.Type == "tool_use" {
		n += EstimateTokens(p.Name) + EstimateTokens(p.ID)
	}
	return n
}

// UsageTokens is the estimated total tokens of the store.
func (s *Store) UsageTokens() int {
	total := 0
	for _, m := range s.Messages {
		total += msgOverhead + EstimateTokens(string(m.Role))
		for _, p := range m.Parts {
			total += partTokens(p)
		}
	}
	return total
}
