// Package llm abstracts LLM backends. A Provider streams chat completions
// with tool calling; providers are registered by id and selected by config.
package llm

import (
	"context"
	"encoding/json"
)

// ToolCall is one tool call of an assistant message.
//
// On the wire it uses the OpenAI-compatible shape
//
//	{"id":"call_1","type":"function","function":{"name":"read","arguments":"{...}"}}
//
// Sending the flat form {"id":..,"name":..,"arguments":..} makes the
// provider answer with a "missing function field" error, because the
// function object is what carries the name and the arguments.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded arguments string from the model
}

// wireToolCall is the JSON shape of a tool call.
type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MarshalJSON writes the tool call in the shape the provider expects.
func (t ToolCall) MarshalJSON() ([]byte, error) {
	w := wireToolCall{ID: t.ID, Type: "function"}
	w.Function.Name = t.Name
	w.Function.Arguments = t.Arguments
	return json.Marshal(w)
}

// UnmarshalJSON reads a tool call. The nested shape is the expected one;
// the flat shape is accepted as a fallback for endpoints that use it.
func (t *ToolCall) UnmarshalJSON(data []byte) error {
	var w wireToolCall
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	t.ID = w.ID
	t.Name = w.Function.Name
	t.Arguments = w.Function.Arguments
	if t.Name == "" {
		var flat struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := json.Unmarshal(data, &flat); err == nil && flat.Name != "" {
			t.ID, t.Name, t.Arguments = flat.ID, flat.Name, flat.Arguments
		}
	}
	return nil
}

type ChatMessage struct {
	Role string `json:"role"` // system | user | assistant | tool
	// Content is always sent, even when it is empty: an assistant message
	// that only carries tool calls still needs the field, and endpoints
	// that validate it reject a message without it.
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolSpec is one tool definition sent to the provider.
//
// The JSON key for the schema is "parameters": OpenAI-compatible endpoints
// require that exact name. Sending it as "schema" makes the provider reject
// the request with a malformed-function error.
type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent is one event of a streamed completion.
type StreamEvent struct {
	Type       string    `json:"type"` // text | reasoning | tool_call | finish
	Text       string    `json:"text,omitempty"`
	Reasoning  string    `json:"reasoning,omitempty"`
	Call       *ToolCall `json:"call,omitempty"`
	StopReason string    `json:"stopReason,omitempty"` // end_turn | tool_use | max_tokens | stop
	Usage      *Usage    `json:"usage,omitempty"`
}

type Provider interface {
	// ID is the registry id, e.g. "adaop".
	ID() string
	// StreamChat streams one completion. tools are the model-facing tool schemas.
	StreamChat(ctx context.Context, messages []ChatMessage, tools []ToolSpec, model string) (<-chan StreamEvent, error)
	// Complete is a single non-streamed completion, used for compaction summaries.
	Complete(ctx context.Context, messages []ChatMessage, model string) (string, error)
}

// Registry maps provider id to Provider.
type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry { return &Registry{providers: map[string]Provider{}} }

func (r *Registry) Add(p Provider) { r.providers[p.ID()] = p }

func (r *Registry) Get(id string) (Provider, bool) {
	p, ok := r.providers[id]
	return p, ok
}

func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	return ids
}

// ToolSpecFromSchema builds the wire spec for one tool.
func ToolSpecFromSchema(name, description string, schema map[string]any) ToolSpec {
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	if _, ok := schema["properties"]; !ok {
		schema["properties"] = map[string]any{}
	}
	return ToolSpec{Name: name, Description: description, Parameters: schema}
}

// ParseToolArgs decodes the model's JSON arguments string.
func ParseToolArgs(raw string) (map[string]any, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	return args, nil
}
