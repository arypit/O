package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adaop/o/internal/tools"
)

// WebFetchTool fetches a URL and returns its text body (truncated).
type WebFetchTool struct{}

func (WebFetchTool) Name() string { return "webfetch" }
func (WebFetchTool) Description() string {
	return "Fetch a http(s) URL and return the response body (truncated)."
}
func (WebFetchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string"},
		},
		"required":             []any{"url"},
		"additionalProperties": false,
	}
}
func (WebFetchTool) Mutating() bool { return false }
func (t WebFetchTool) Summarize(args map[string]any) string {
	s, _ := args["url"].(string)
	return s
}

func (t WebFetchTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	url, _ := args["url"].(string)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return tools.ToolResult{Content: "webfetch: only http(s) URLs are allowed", IsError: true}, nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	req.Header.Set("User-Agent", "O-agent/0.0 (adaop.sbs)")
	resp, err := client.Do(req)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("webfetch %s: %v", url, err), IsError: true}, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024))
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return tools.Truncate(ec, fmt.Sprintf("HTTP %d %s\n%s", resp.StatusCode, resp.Header.Get("Content-Type"), string(body))), nil
}
