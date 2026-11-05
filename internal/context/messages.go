// Package context manages the conversation: message store, conversion to
// chat messages, token accounting, compaction, and memory injection.
package context

import (
	"strings"

	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/llm"
)

type Store struct {
	Messages []core.Message
}

func NewStore(msgs ...core.Message) *Store {
	return &Store{Messages: append([]core.Message(nil), msgs...)}
}

func (s *Store) Add(m core.Message) {
	s.Messages = append(s.Messages, m)
}

// ChatMessages converts the store into provider chat messages, prepending the
// system prompt. Tool results become role="tool" messages linked by id.
func (s *Store) ChatMessages(system string) []llm.ChatMessage {
	out := make([]llm.ChatMessage, 0, len(s.Messages)+1)
	if system != "" {
		out = append(out, llm.ChatMessage{Role: "system", Content: system})
	}
	for _, m := range s.Messages {
		switch m.Role {
		case core.RoleTool:
			for _, p := range m.Parts {
				if p.Type == "tool_result" {
					out = append(out, llm.ChatMessage{
						Role:       "tool",
						Content:    p.Content,
						ToolCallID: p.ToolUseID,
						Name:       p.Name,
					})
				}
			}
		case core.RoleAssistant:
			var text strings.Builder
			var calls []llm.ToolCall
			for _, p := range m.Parts {
				switch p.Type {
				case "text":
					text.WriteString(p.Text)
				case "summary":
					text.WriteString("[compacted summary] " + p.Text)
				case "tool_use":
					calls = append(calls, llm.ToolCall{ID: p.ID, Name: p.Name, Arguments: string(p.Input)})
				}
			}
			out = append(out, llm.ChatMessage{Role: "assistant", Content: text.String(), ToolCalls: calls})
		default:
			var text strings.Builder
			for _, p := range m.Parts {
				if p.Type == "text" || p.Type == "summary" {
					text.WriteString(p.Text)
				}
			}
			out = append(out, llm.ChatMessage{Role: string(m.Role), Content: text.String()})
		}
	}
	return out
}
