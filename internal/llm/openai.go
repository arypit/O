// OpenAI-compatible chat completions provider with SSE streaming.
// Points at config providers.*.baseUrl (default https://api.adaop.sbs/v1).
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type OpenAIProvider struct {
	id      string
	baseURL string
	apiKey  string
	client  *http.Client
	retry   RetryConfig
	mu      sync.Mutex
	toolMode string // how the provider accepts tool definitions
}

// Tool definition modes. The provider is asked with "tools" first; when the
// endpoint refuses that format it is asked with the legacy "functions"
// field, and when it refuses that too it is asked without any tool
// definition, so the model can still answer in words.
const (
	toolModeTools  = "tools"
	toolModeLegacy = "functions"
	toolModeNone   = "none"
)

func NewOpenAI(id, baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		id:      id,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  newHTTPClient(),
		retry:   DefaultRetry(),
	}
}

// SetRetry overrides the retry policy (used by tests).
func (p *OpenAIProvider) SetRetry(rc RetryConfig) { p.retry = rc }

func (p *OpenAIProvider) ID() string { return p.id }

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []ChatMessage  `json:"messages"`
	Tools         []any          `json:"tools,omitempty"`
	Functions     []ToolSpec     `json:"functions,omitempty"`
	FunctionCall  any            `json:"function_call,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions map[string]any `json:"stream_options,omitempty"`
	Temperature   *float64       `json:"temperature,omitempty"`
}

type functionTool struct {
	Type     string   `json:"type"`
	Function ToolSpec `json:"function"`
}

// currentToolMode reports how this provider accepts tool definitions.
func (p *OpenAIProvider) currentToolMode() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.toolMode == "" {
		return toolModeTools
	}
	return p.toolMode
}

// setToolMode remembers how the provider accepts tool definitions.
func (p *OpenAIProvider) setToolMode(mode string) {
	p.mu.Lock()
	p.toolMode = mode
	p.mu.Unlock()
}

// Warning returns a notice for the interface when the provider had to be
// asked without tool definitions.
func (p *OpenAIProvider) Warning() string {
	if p.currentToolMode() == toolModeNone {
		return "This provider does not accept tool definitions, so O answers in words instead of calling tools."
	}
	return ""
}

// rejectsTools detects a provider complaint about the tool definitions of
// the request, e.g. "missing function field", "tools is not supported" or
// "function calling is not available for this model".
func rejectsTools(body string) bool {
	b := strings.ToLower(body)
	if !strings.Contains(b, "tool") && !strings.Contains(b, "function") {
		return false
	}
	for _, s := range []string{
		"missing", "not supported", "unsupported", "unknown", "not allowed",
		"unexpected", "unrecognized", "not permitted", "not available",
		"does not support", "extra inputs", "not a valid", "invalid",
	} {
		if strings.Contains(b, s) {
			return true
		}
	}
	return false
}

// providerError builds a typed provider error from a failed response.
func providerError(id string, status int, raw []byte) *Error {
	detail := SummarizeBody(raw)
	kind := StatusKind(status)
	if k := BodyKind(string(raw)); k != "" {
		kind = k
	}
	return &Error{Provider: id, Status: status, Kind: kind, Detail: detail}
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []ChatMessage, tools []ToolSpec, model string) (<-chan StreamEvent, error) {
	resp, err := p.chat(ctx, messages, tools, model, true)
	if err != nil {
		return nil, err
	}
	// a stalled low-network stream must error, not hang forever
	resp.Body = newWatchdogBody(resp.Body, 120*time.Second)
	out := make(chan StreamEvent, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		// a panic in the parser must not kill the process
		defer func() { _ = recover() }()
		p.parseSSE(ctx, resp.Body, out)
	}()
	return out, nil
}

// chat sends one request and negotiates the tool format with the endpoint:
// "tools" first, the legacy "functions" field when that is refused, and a
// request without tool definitions when the endpoint supports no tools at
// all. The negotiated format is remembered for the next request.
func (p *OpenAIProvider) chat(ctx context.Context, messages []ChatMessage, tools []ToolSpec, model string, stream bool) (*http.Response, error) {
	mode := p.currentToolMode()
	if len(tools) == 0 {
		mode = toolModeNone
	}
	resp, err := p.send(ctx, messages, tools, model, stream, mode)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || len(tools) == 0 || !rejectsTools(string(body)) {
		return nil, providerError(p.id, resp.StatusCode, body)
	}
	// the endpoint refused the tool definitions: try the next format
	next := toolModeLegacy
	switch mode {
	case toolModeTools:
		next = toolModeLegacy
	case toolModeLegacy:
		next = toolModeNone
	default:
		return nil, providerError(p.id, resp.StatusCode, body)
	}
	p.setToolMode(next)
	resp, err = p.send(ctx, messages, tools, model, stream, next)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	body2, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if next == toolModeLegacy && resp.StatusCode == http.StatusBadRequest && rejectsTools(string(body2)) {
		// the legacy format is refused as well: ask without tools
		p.setToolMode(toolModeNone)
		resp, err = p.send(ctx, messages, tools, model, stream, toolModeNone)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		body3, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, providerError(p.id, resp.StatusCode, body3)
	}
	return nil, providerError(p.id, resp.StatusCode, body2)
}

// send builds and posts one request in the given tool format.
func (p *OpenAIProvider) send(ctx context.Context, messages []ChatMessage, tools []ToolSpec, model string, stream bool, mode string) (*http.Response, error) {
	reqBody := chatRequest{Model: model, Messages: messages, Stream: stream}
	if stream {
		reqBody.StreamOptions = map[string]any{"include_usage": true}
	}
	switch mode {
	case toolModeLegacy:
		reqBody.Functions = tools
		reqBody.FunctionCall = "auto"
	case toolModeNone:
		// no tool definitions: the model answers in words
	default:
		for _, t := range tools {
			reqBody.Tools = append(reqBody.Tools, functionTool{Type: "function", Function: t})
		}
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, &Error{Provider: p.id, Kind: KindRequest, Detail: "could not encode the request", Err: err}
	}
	return p.post(ctx, body)
}

// post sends the request body with the retry policy applied.
func (p *OpenAIProvider) post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	return p.doWithRetry(ctx, req, p.retry)
}

type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
			// legacy function-calling protocol
			FunctionCall struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function_call"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

func (p *OpenAIProvider) parseSSE(ctx context.Context, r io.Reader, out chan<- StreamEvent) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	// send blocks if the consumer stopped reading; bail out on cancel instead
	send := func(ev StreamEvent) bool {
		select {
		case out <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}
	// Tool calls stream in fragments keyed by index; accumulate them.
	type acc struct {
		id, name, args string
	}
	calls := map[int]*acc{}
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var chunk sseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				if !send(StreamEvent{Type: "text", Text: ch.Delta.Content}) {
					return
				}
			}
			if ch.Delta.ReasoningContent != "" {
				if !send(StreamEvent{Type: "reasoning", Reasoning: ch.Delta.ReasoningContent}) {
					return
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				a := calls[tc.Index]
				if a == nil {
					a = &acc{}
					calls[tc.Index] = a
				}
				if tc.ID != "" {
					a.id = tc.ID
				}
				if tc.Function.Name != "" {
					a.name = tc.Function.Name
				}
				a.args += tc.Function.Arguments
			}
			// legacy protocol: a single function_call streamed in fragments
			if ch.Delta.FunctionCall.Name != "" || ch.Delta.FunctionCall.Arguments != "" {
				a := calls[0]
				if a == nil {
					a = &acc{}
					calls[0] = a
				}
				if ch.Delta.FunctionCall.Name != "" {
					a.name = ch.Delta.FunctionCall.Name
				}
				a.args += ch.Delta.FunctionCall.Arguments
			}
			if ch.FinishReason != "" {
				stop := ch.FinishReason
				if len(calls) > 0 {
					stop = "tool_use"
				}
				// the tool calls arrive keyed by their index: send them in
				// index order, even when an index is missing
				idx := make([]int, 0, len(calls))
				for i := range calls {
					idx = append(idx, i)
				}
				sort.Ints(idx)
				for _, i := range idx {
					a := calls[i]
					if a == nil || a.name == "" {
						continue
					}
					if !send(StreamEvent{Type: "tool_call", Call: &ToolCall{ID: a.id, Name: a.name, Arguments: a.args}}) {
						return
					}
				}
				send(StreamEvent{Type: "finish", StopReason: stop, Usage: chunk.Usage})
			}
		}
	}
	if err := sc.Err(); err != nil {
		send(StreamEvent{Type: "error", Text: "stream interrupted: " + err.Error()})
	}
}

func (p *OpenAIProvider) Complete(ctx context.Context, messages []ChatMessage, model string) (string, error) {
	reqBody := chatRequest{Model: model, Messages: messages, Stream: false}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", &Error{Provider: p.id, Kind: KindRequest, Detail: "could not encode the request", Err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.doWithRetry(ctx, req, p.retry)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", providerError(p.id, resp.StatusCode, b)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", &Error{Provider: p.id, Status: resp.StatusCode, Kind: KindParse, Detail: "the response was not valid JSON", Err: err}
	}
	if len(out.Choices) == 0 {
		return "", &Error{Provider: p.id, Status: resp.StatusCode, Kind: KindParse, Detail: "the response contained no completion"}
	}
	return out.Choices[0].Message.Content, nil
}
