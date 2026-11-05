// Package agent implements the O agent loop:
//
//	user prompt -> build context -> LLM stream -> tool calls
//	  -> validate/permission/sandbox/execute -> tool results
//	  -> append to context -> repeat until end_turn or max turns
//
// Every message is persisted to the session JSONL immediately.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/adaop/o/internal/config"
	octx "github.com/adaop/o/internal/context"
	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/hitl"
	"github.com/adaop/o/internal/hooks"
	"github.com/adaop/o/internal/llm"
	"github.com/adaop/o/internal/sandbox"
	"github.com/adaop/o/internal/session"
	"github.com/adaop/o/internal/tools"
)

type Agent struct {
	Cfg      *config.Config
	Provider llm.Provider
	Model    string
	Reg      *tools.Registry
	Policy   *tools.Policy
	SB       *sandbox.Sandbox
	Store    *octx.Store
	Sess     *session.Session
	Decider  hitl.Decider
	Learner  *hitl.Learner
	Hooks    *hooks.Hooks
	Todos    *core.TodoStore
	Emit     func(core.AgentEvent)
	MaxTurns int
	Depth    int
	CWD      string
	Home     string
	// AllowOnce remembers "always" approvals for this process.
	AllowOnce *tools.AllowSet
	// PauseBetweenSteps stops the agent after every tool round so the user
	// can continue it by hand.
	PauseBetweenSteps bool
	// PlanFirst asks the agent to write a plan before it changes anything.
	PlanFirst bool
	// NoProjectContext leaves the repository snapshot and memory files out
	// of the system prompt.
	NoProjectContext bool
}

func (a *Agent) emit(ev core.AgentEvent) {
	if a.Emit != nil {
		a.Emit(ev)
	}
}

func (a *Agent) systemPrompt() string {
	p := octx.BuildSystemPromptWith(*a.Cfg, a.CWD, a.Home, a.Todos.Items, time.Now().UTC(), !a.NoProjectContext)
	if a.PlanFirst {
		p += "\nBefore you change anything, write a short plan with the todo tool and work through it step by step.\n"
	}
	return p
}

func (a *Agent) sessID() string {
	if a.Sess == nil {
		return ""
	}
	return a.Sess.ID
}

func (a *Agent) execCtx() *tools.ExecContext {
	return &tools.ExecContext{
		SessionID:     a.sessID(),
		CWD:           a.CWD,
		SB:            a.SB,
		Decider:       a.Decider,
		Learner:       a.Learner,
		Hooks:         a.Hooks,
		Todos:         a.Todos,
		Emit:          a.emit,
		MaxToolOutput: a.Cfg.Context.MaxToolOutputChars,
		Depth:         a.Depth,
		AllowOnce:     a.AllowOnce,
	}
}

// Run executes one user prompt to completion and returns the final text.
// A panic anywhere in the loop is recovered and returned as an error.
func (a *Agent) Run(ctx context.Context, prompt string) (final string, err error) {
	return a.run(ctx, prompt, true)
}

// Resume continues a paused run without adding a new user message.
func (a *Agent) Resume(ctx context.Context) (final string, err error) {
	return a.run(ctx, "", false)
}

func (a *Agent) run(ctx context.Context, prompt string, addPrompt bool) (final string, err error) {
	defer func() {
		if r := recover(); r != nil {
			final, err = "", fmt.Errorf("agent panic: %v", r)
		}
	}()
	if a.MaxTurns <= 0 {
		a.MaxTurns = 50
	}
	if a.Todos == nil {
		a.Todos = &core.TodoStore{}
	}
	if a.Store == nil {
		a.Store = octx.NewStore()
	}
	appendMsg := func(m core.Message) {
		if a.Sess != nil {
			_ = a.Sess.Append(m)
		}
	}
	if a.Hooks != nil {
		_ = a.Hooks.Run("SessionStart", map[string]string{"O_EVENT": "SessionStart", "O_SESSION": a.sessID()})
	}
	if addPrompt {
		a.Store.Add(core.NewMessage(core.RoleUser, core.TextPart(prompt)))
		appendMsg(a.Store.Messages[len(a.Store.Messages)-1])
	}

	var lastText string
	for turn := 0; turn < a.MaxTurns; turn++ {
		msgs := a.Store.ChatMessages(a.systemPrompt())
		stream, err := a.Provider.StreamChat(ctx, msgs, a.Reg.Specs(), a.Model)
		if err != nil {
			return lastText, fmt.Errorf("provider: %w", err)
		}
		asst := core.NewMessage(core.RoleAssistant)
		var calls []llm.ToolCall
		stop := "end_turn"
		var usage *llm.Usage
		for ev := range stream {
			switch ev.Type {
			case "text":
				asst.Parts = append(asst.Parts, core.TextPart(ev.Text))
				lastText += ev.Text
				a.emit(core.AgentEvent{Type: "text", Text: ev.Text})
			case "reasoning":
				asst.Parts = append(asst.Parts, core.Part{Type: "reasoning", Text: ev.Reasoning})
				a.emit(core.AgentEvent{Type: "reasoning", Text: ev.Reasoning})
			case "tool_call":
				if ev.Call != nil {
					calls = append(calls, *ev.Call)
					input := []byte(ev.Call.Arguments)
					asst.Parts = append(asst.Parts, core.Part{Type: "tool_use", ID: ev.Call.ID, Name: ev.Call.Name, Input: input})
				}
			case "finish":
				stop = ev.StopReason
				usage = ev.Usage
			case "error":
				return lastText, fmt.Errorf("stream error: %s", ev.Text)
			}
		}
		a.Store.Add(asst)
		appendMsg(asst)

		if stop != "tool_use" || len(calls) == 0 {
			a.emit(core.AgentEvent{Type: "done", StopReason: stop, Usage: usageToCore(usage)})
			if a.Hooks != nil {
				_ = a.Hooks.Run("Stop", map[string]string{"O_EVENT": "Stop", "O_SESSION": a.sessID()})
			}
			return lastText, nil
		}
		for _, call := range calls {
			part := tools.ExecuteCall(ctx, a.Reg, a.Policy, call, a.execCtx())
			toolMsg := core.NewMessage(core.RoleTool, part)
			a.Store.Add(toolMsg)
			appendMsg(toolMsg)
		}
		if u := usageToCore(usage); u != nil {
			a.emit(core.AgentEvent{Type: "turn_end", Usage: u})
		}
		if a.PauseBetweenSteps {
			// automatic continuation is off: hand control back to the user
			a.emit(core.AgentEvent{Type: "done", StopReason: "paused", Usage: usageToCore(usage)})
			return lastText, nil
		}
		compacted, err := a.Store.MaybeCompact(ctx, a.Provider, a.Model, a.Cfg.Context.MaxTokens, a.Cfg.Context.CompactThreshold, 6)
		if err != nil {
			a.emit(core.AgentEvent{Type: "error", Message: "compaction failed: " + err.Error()})
		}
		if compacted {
			a.emit(core.AgentEvent{Type: "text", Text: "[context compacted]"})
		}
	}
	return lastText, errors.New("max turns reached without completion")
}

func usageToCore(u *llm.Usage) *core.Usage {
	if u == nil {
		return nil
	}
	return &core.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens}
}

// HomeDir returns the user home directory (helper for wiring).
func HomeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}
