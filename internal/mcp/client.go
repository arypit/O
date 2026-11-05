// Package mcp is a minimal MCP (Model Context Protocol) client over stdio:
// newline-delimited JSON-RPC 2.0. It performs the initialize handshake,
// lists tools and calls them; MCP tools are wrapped as regular O tools named
// "server__tool".
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/adaop/o/internal/tools"
)

type Server struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
}

type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"inputSchema"`
}

type Client struct {
	server  Server
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	nextID  int
	pending map[int]chan json.RawMessage
	dec     *json.Decoder
}

// Start launches the server process and performs the MCP initialize handshake.
func Start(ctx context.Context, srv Server) (*Client, error) {
	cmd := exec.CommandContext(ctx, srv.Command, srv.Args...)
	cmd.Env = append(cmd.Env, srv.Env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp server %s: %w", srv.Name, err)
	}
	c := &Client{
		server:  srv,
		cmd:     cmd,
		stdin:   stdin,
		pending: map[int]chan json.RawMessage{},
		dec:     json.NewDecoder(stdout),
	}
	go c.readLoop()
	_, err = c.call("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "o", "version": "0.0.0"},
	})
	if err != nil {
		return nil, fmt.Errorf("mcp initialize %s: %w", srv.Name, err)
	}
	c.notify("notifications/initialized", map[string]any{})
	return c, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) readLoop() {
	for {
		var resp rpcResponse
		if err := c.dec.Decode(&resp); err != nil {
			c.mu.Lock()
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
		c.mu.Lock()
		ch := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- resp.Result
			if resp.Error != nil {
				ch <- nil
			}
			close(ch)
		}
	}
}

func (c *Client) call(method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan json.RawMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	b, _ := json.Marshal(req)
	c.mu.Lock()
	_, err := c.stdin.Write(append(b, '\n'))
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	result := <-ch
	if result == nil {
		return nil, fmt.Errorf("mcp %s: call %s failed", c.server.Name, method)
	}
	return result, nil
}

func (c *Client) notify(method string, params any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	c.mu.Lock()
	c.stdin.Write(append(b, '\n'))
	c.mu.Unlock()
}

// Tools lists the server's tools.
func (c *Client) Tools() ([]MCPTool, error) {
	res, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(res, &parsed); err != nil {
		return nil, err
	}
	return parsed.Tools, nil
}

// CallTool invokes a server tool and returns its text content.
func (c *Client) CallTool(name string, args map[string]any) (string, error) {
	res, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &parsed); err != nil {
		return "", err
	}
	out := ""
	for _, ct := range parsed.Content {
		if ct.Type == "text" {
			out += ct.Text + "\n"
		}
	}
	if parsed.IsError {
		return out, fmt.Errorf("mcp tool %s error: %s", name, out)
	}
	return out, nil
}

// Close shuts the server process down.
func (c *Client) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

// mcpTool wraps one remote tool as a local tools.Tool.
type mcpTool struct {
	serverName string
	tool       MCPTool
	client     *Client
}

func (t mcpTool) Name() string { return t.serverName + "__" + t.tool.Name }
func (t mcpTool) Description() string {
	if t.tool.Description != "" {
		return t.tool.Description
	}
	return "MCP tool " + t.tool.Name + " from server " + t.serverName
}
func (t mcpTool) Schema() map[string]any {
	if t.tool.Schema != nil {
		return t.tool.Schema
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t mcpTool) Mutating() bool { return true } // conservative: MCP tools can do anything
func (t mcpTool) Summarize(args map[string]any) string {
	b, _ := json.Marshal(args)
	s := string(b)
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
func (t mcpTool) Execute(ctx context.Context, args map[string]any, ec *tools.ExecContext) (tools.ToolResult, error) {
	out, err := t.client.CallTool(t.tool.Name, args)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return tools.Truncate(ec, out), nil
}

// RegisterTools connects to the server and adds all its tools to reg.
func RegisterTools(ctx context.Context, srv Server, reg *tools.Registry) (*Client, error) {
	client, err := Start(ctx, srv)
	if err != nil {
		return nil, err
	}
	list, err := client.Tools()
	if err != nil {
		client.Close()
		return nil, err
	}
	for _, t := range list {
		reg.Add(mcpTool{serverName: srv.Name, tool: t, client: client})
	}
	return client, nil
}
