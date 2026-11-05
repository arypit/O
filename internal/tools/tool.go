// Package tools defines the tool execution model of O.
//
// Pipeline for every tool call: validate args against the JSON schema,
// check the permission policy (allow/deny/ask, possibly human-in-the-loop),
// enforce the sandbox, run the tool, normalize the result (truncate or
// offload huge outputs) and return it to the model as a tool_result.
package tools

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/hitl"
	"github.com/adaop/o/internal/hooks"
	"github.com/adaop/o/internal/llm"
	"github.com/adaop/o/internal/sandbox"
)

// ToolResult is what goes back to the model.
type ToolResult struct {
	Content     string `json:"content"`
	IsError     bool   `json:"isError,omitempty"`
	OffloadedTo string `json:"offloadedTo,omitempty"` // set when output was moved to a file
}

// ExecContext carries everything a tool needs at execution time.
type ExecContext struct {
	SessionID     string
	CWD           string
	SB            *sandbox.Sandbox
	Decider       hitl.Decider
	Learner       *hitl.Learner
	Hooks         *hooks.Hooks
	Todos         *core.TodoStore
	Emit          func(core.AgentEvent)
	MaxToolOutput int
	Depth         int // 0 = primary agent, >0 = subagent
	// AllowOnce holds the tools the human approved for the whole session.
	AllowOnce *AllowSet
}

// AllowSet remembers the tools the human approved for the whole session.
// It is safe for concurrent use: the UI writes while the agent reads.
type AllowSet struct {
	mu sync.Mutex
	m  map[string]bool
}

// NewAllowSet returns an empty approval set.
func NewAllowSet() *AllowSet { return &AllowSet{m: map[string]bool{}} }

// Allow reports whether the tool was approved for the whole session.
func (s *AllowSet) Allow(tool string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[tool]
}

// Set approves the tool for the whole session.
func (s *AllowSet) Set(tool string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]bool{}
	}
	s.m[tool] = true
}

// Clear forgets every approval of this session.
func (s *AllowSet) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = map[string]bool{}
}

// Tool is one callable capability.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	// Mutating reports whether the tool changes the filesystem or runs shell.
	Mutating() bool
	// Summarize builds the short string used in permission rules, e.g. "bash(git status)".
	Summarize(args map[string]any) string
	Execute(ctx context.Context, args map[string]any, ec *ExecContext) (ToolResult, error)
}

// Registry holds the tools available to an agent.
type Registry struct {
	tools    map[string]Tool
	order    []string
	disabled map[string]bool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}, disabled: map[string]bool{}}
}

func (r *Registry) Add(t Tool) {
	if _, exists := r.tools[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

// SetEnabled turns a tool on or off without losing its definition.
func (r *Registry) SetEnabled(name string, enabled bool) {
	if r.disabled == nil {
		r.disabled = map[string]bool{}
	}
	if enabled {
		delete(r.disabled, name)
		return
	}
	r.disabled[name] = true
}

// Enabled reports whether a tool is currently offered to the agent.
func (r *Registry) Enabled(name string) bool {
	return !r.disabled[name]
}

func (r *Registry) Get(name string) (Tool, bool) {
	if r.disabled[name] {
		return nil, false
	}
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		if r.disabled[name] {
			continue
		}
		out = append(out, r.tools[name])
	}
	return out
}

// Specs returns the model-facing tool schemas.
func (r *Registry) Specs() []llm.ToolSpec {
	all := r.All()
	specs := make([]llm.ToolSpec, 0, len(all))
	for _, t := range all {
		specs = append(specs, llm.ToolSpecFromSchema(t.Name(), t.Description(), t.Schema()))
	}
	return specs
}

// ValidateArgs checks args against a JSON schema (subset of JSON Schema:
// type object, properties with type, required, additionalProperties).
func ValidateArgs(schema map[string]any, args map[string]any) error {
	if schema == nil {
		return nil
	}
	props, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	addl, hasAddl := schema["additionalProperties"].(bool)
	if hasAddl && !addl && props != nil {
		for k := range args {
			if _, ok := props[k]; !ok {
				return &ValidationError{Field: k, Reason: "unknown property (additionalProperties: false)"}
			}
		}
	}
	for _, req := range required {
		name, ok := req.(string)
		if !ok {
			continue
		}
		if _, ok := args[name]; !ok {
			return &ValidationError{Field: name, Reason: "required property missing"}
		}
	}
	for name, specAny := range props {
		spec, ok := specAny.(map[string]any)
		if !ok {
			continue
		}
		val, ok := args[name]
		if !ok {
			continue
		}
		want, _ := spec["type"].(string)
		if err := checkType(name, want, val); err != nil {
			return err
		}
	}
	return nil
}

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return "invalid argument " + e.Field + ": " + e.Reason
}

func checkType(field, want string, val any) error {
	switch want {
	case "string":
		if _, ok := val.(string); !ok {
			return &ValidationError{Field: field, Reason: "expected string"}
		}
	case "number":
		switch val.(type) {
		case float64, int, int64:
		default:
			return &ValidationError{Field: field, Reason: "expected number"}
		}
	case "integer":
		f, ok := val.(float64)
		if !ok || f != float64(int64(f)) {
			return &ValidationError{Field: field, Reason: "expected integer"}
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return &ValidationError{Field: field, Reason: "expected boolean"}
		}
	case "array":
		if _, ok := val.([]any); !ok {
			return &ValidationError{Field: field, Reason: "expected array"}
		}
	case "object":
		if _, ok := val.(map[string]any); !ok {
			return &ValidationError{Field: field, Reason: "expected object"}
		}
	}
	return nil
}

// Truncate caps tool output length; when truncated it writes the full output
// to a file under the session offload dir and returns the path.
func Truncate(ec *ExecContext, content string) ToolResult {
	limit := ec.MaxToolOutput
	if limit <= 0 {
		limit = 20000
	}
	if len(content) <= limit {
		return ToolResult{Content: content}
	}
	return ToolResult{
		Content:     content[:limit] + "\n... [truncated, full output saved to " + offloadPath(ec) + "]",
		OffloadedTo: offloadPath(ec),
	}
}

func offloadPath(ec *ExecContext) string {
	return ec.SessionID + ".tooloutput.txt"
}

// ArgsJSON marshals args for events and hook env.
func ArgsJSON(args map[string]any) json.RawMessage {
	b, _ := json.Marshal(args)
	return b
}
