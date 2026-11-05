// Package core holds the wire types shared by every O package.
// Nothing in this package imports other internal packages, so there are no cycles.
package core

import (
	"encoding/json"
	"time"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Part is one typed piece of a message: text, reasoning, a tool call, a tool
// result, a todo update or a compaction summary.
type Part struct {
	Type      string          `json:"type"` // text | reasoning | tool_use | tool_result | todo | summary
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`        // tool_use id
	Name      string          `json:"name,omitempty"`      // tool name for tool_use
	Input     json.RawMessage `json:"input,omitempty"`     // tool_use arguments
	ToolUseID string          `json:"toolUseId,omitempty"` // tool_result link
	Content   string          `json:"content,omitempty"`   // tool_result content
	IsError   bool            `json:"isError,omitempty"`
	Todos     []TodoItem      `json:"todos,omitempty"`
}

type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"` // pending | in_progress | completed
	ActiveForm string `json:"activeForm,omitempty"`
}

// TodoStore is the shared todo list for a session.
type TodoStore struct {
	Items []TodoItem `json:"items"`
}

func (t *TodoStore) Set(items []TodoItem) {
	t.Items = items
	for i := range t.Items {
		if t.Items[i].Status == "" {
			t.Items[i].Status = "pending"
		}
	}
}

type Message struct {
	Role      Role      `json:"role"`
	Parts     []Part    `json:"parts"`
	Timestamp time.Time `json:"timestamp"`
}

func NewMessage(role Role, parts ...Part) Message {
	return Message{Role: role, Parts: parts, Timestamp: time.Now().UTC()}
}

func TextPart(s string) Part  { return Part{Type: "text", Text: s} }
func SummaryPart(s string) Part { return Part{Type: "summary", Text: s} }

// Usage is token accounting for one model call.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// AgentEvent is one line of the JSONL event stream the engine emits to the TUI.
type AgentEvent struct {
	Type       string          `json:"type"` // text | reasoning | tool_call | tool_result | permission_request | todo | turn_end | done | error
	Text       string          `json:"text,omitempty"`
	Tool       string          `json:"tool,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Content    string          `json:"content,omitempty"`
	IsError    bool            `json:"isError,omitempty"`
	ID         string          `json:"id,omitempty"`
	Allow      *bool           `json:"allow,omitempty"`
	StopReason string          `json:"stopReason,omitempty"`
	Usage      *Usage          `json:"usage,omitempty"`
	Todos      []TodoItem      `json:"todos,omitempty"`
	Message    string          `json:"message,omitempty"`
}
