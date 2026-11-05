package llm

import (
	"context"
	"fmt"
	"strings"
)

// Fallback tries providers in order: if one is rate-limited (429) or
// unreachable after its own retries, the next configured provider is used,
// so the model keeps working even when one endpoint is down.
type Fallback struct {
	providers []Provider
}

func NewFallback(ps ...Provider) *Fallback { return &Fallback{providers: ps} }

func (f *Fallback) ID() string {
	ids := make([]string, 0, len(f.providers))
	for _, p := range f.providers {
		ids = append(ids, p.ID())
	}
	return strings.Join(ids, "+")
}

func (f *Fallback) StreamChat(ctx context.Context, messages []ChatMessage, tools []ToolSpec, model string) (<-chan StreamEvent, error) {
	var errs []string
	for _, p := range f.providers {
		out, err := p.StreamChat(ctx, messages, tools, model)
		if err == nil {
			return out, nil
		}
		errs = append(errs, err.Error())
	}
	return nil, fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}

func (f *Fallback) Complete(ctx context.Context, messages []ChatMessage, model string) (string, error) {
	var errs []string
	for _, p := range f.providers {
		s, err := p.Complete(ctx, messages, model)
		if err == nil {
			return s, nil
		}
		errs = append(errs, err.Error())
	}
	return "", fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}

// Warning returns the notice of the first provider that has one, so the
// interface can tell the user that a provider answered without tools.
func (f *Fallback) Warning() string {
	for _, p := range f.providers {
		if w, ok := p.(interface{ Warning() string }); ok {
			if msg := w.Warning(); msg != "" {
				return msg
			}
		}
	}
	return ""
}
