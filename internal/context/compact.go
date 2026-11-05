package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/llm"
)

// MaybeCompact summarizes old messages when estimated usage crosses
// threshold*maxTokens. The most recent keepRecent messages are kept
// verbatim; everything older is replaced by one summary part.
func (s *Store) MaybeCompact(ctx context.Context, p llm.Provider, model string, maxTokens int, threshold float64, keepRecent int) (bool, error) {
	usage := s.UsageTokens()
	if float64(usage) < threshold*float64(maxTokens) {
		return false, nil
	}
	if len(s.Messages) <= keepRecent+1 {
		return false, nil
	}
	cut := len(s.Messages) - keepRecent
	old := s.Messages[:cut]
	var sb strings.Builder
	for _, m := range old {
		fmt.Fprintf(&sb, "[%s]\n", m.Role)
		for _, part := range m.Parts {
			switch part.Type {
			case "text", "summary":
				sb.WriteString(part.Text + "\n")
			case "reasoning":
				sb.WriteString("(reasoning) " + part.Text + "\n")
			case "tool_use":
				sb.WriteString(fmt.Sprintf("(tool_use %s %s)\n", part.Name, string(part.Input)))
			case "tool_result":
				c := part.Content
				if len(c) > 500 {
					c = c[:500] + "..."
				}
				sb.WriteString("(tool_result) " + c + "\n")
			}
		}
	}
	prompt := "Summarize the following agent conversation compactly. Preserve: the user's goals, decisions made, file paths touched, tool outcomes, pending todos, and any errors. Write 1-3 short paragraphs, no decoration.\n\n" + sb.String()
	summary, err := p.Complete(ctx, []llm.ChatMessage{{Role: "user", Content: prompt}}, model)
	if err != nil {
		return false, err
	}
	kept := append([]core.Message{core.NewMessage(core.RoleAssistant, core.SummaryPart(summary))}, s.Messages[cut:]...)
	s.Messages = kept
	return true, nil
}
