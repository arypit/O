package tools

import (
	"context"
	"fmt"

	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/hitl"
	"github.com/adaop/o/internal/llm"
)

// ExecuteCall runs the full pipeline for one model tool call and returns the
// tool_result part to append to the conversation:
// validate args -> permission policy -> human-in-the-loop if asked ->
// pre-tool hook -> sandbox+execute -> post-tool hook -> normalize output.
func ExecuteCall(ctx context.Context, reg *Registry, policy *Policy, call llm.ToolCall, ec *ExecContext) core.Part {
	tool, ok := reg.Get(call.Name)
	if !ok {
		return core.Part{Type: "tool_result", ToolUseID: call.ID, Content: fmt.Sprintf("unknown tool %q", call.Name), IsError: true}
	}
	args, err := llm.ParseToolArgs(call.Arguments)
	if err != nil {
		return core.Part{Type: "tool_result", ToolUseID: call.ID, Content: fmt.Sprintf("invalid arguments JSON: %v", err), IsError: true}
	}
	if err := ValidateArgs(tool.Schema(), args); err != nil {
		return core.Part{Type: "tool_result", ToolUseID: call.ID, Content: err.Error(), IsError: true}
	}

	summary := tool.Summarize(args)
	learnedProb, learnedSamples := 0.5, 0
	if ec.Learner != nil {
		learnedProb = ec.Learner.Prob(call.Name)
		learnedSamples = ec.Learner.Samples(call.Name)
	}
	decision := policy.Check(call.Name, summary, tool.Mutating(), learnedProb, learnedSamples)

	if ec.Emit != nil {
		ec.Emit(core.AgentEvent{Type: "tool_call", ID: call.ID, Tool: call.Name, Input: ArgsJSON(args)})
	}

	if decision == Ask {
		allow := false
		if ec.AllowOnce.Allow(call.Name) {
			// the human approved this tool for the whole session
			allow = true
		} else if ec.Decider != nil {
			// The decider owns the request and the verdict, so the UI sees
			// exactly one request and one answer per tool call. The id is
			// the tool-call id: the dialog and the waiting call always
			// refer to the same request.
			if d, ok := ec.Decider.(hitl.IDDecider); ok {
				allow, err = d.AskID(call.ID, call.Name, summary, ArgsJSON(args))
			} else {
				allow, err = ec.Decider.Ask(call.Name, summary)
			}
			if err != nil {
				allow = false
			}
		}
		if !allow {
			content := fmt.Sprintf("permission denied for %s(%s)", call.Name, summary)
			res := ToolResult{Content: content, IsError: true}
			if ec.Emit != nil {
				ec.Emit(core.AgentEvent{Type: "tool_result", ID: call.ID, Tool: call.Name, Content: content, IsError: true})
			}
			return core.Part{Type: "tool_result", ToolUseID: call.ID, Content: res.Content, IsError: true}
		}
	} else if decision == Deny {
		content := fmt.Sprintf("denied by permission policy (mode=%s): %s(%s)", policy.CurrentMode(), call.Name, summary)
		if ec.Emit != nil {
			ec.Emit(core.AgentEvent{Type: "tool_result", ID: call.ID, Tool: call.Name, Content: content, IsError: true})
		}
		return core.Part{Type: "tool_result", ToolUseID: call.ID, Content: content, IsError: true}
	}

	if ec.Hooks != nil {
		if err := ec.Hooks.Run("PreToolUse", map[string]string{
			"O_EVENT": "PreToolUse", "O_TOOL": call.Name, "O_INPUT": string(ArgsJSON(args)),
		}); err != nil {
			content := "blocked by PreToolUse hook: " + err.Error()
			if ec.Emit != nil {
				ec.Emit(core.AgentEvent{Type: "tool_result", ID: call.ID, Tool: call.Name, Content: content, IsError: true})
			}
			return core.Part{Type: "tool_result", ToolUseID: call.ID, Content: content, IsError: true}
		}
	}

	res, err := safeExecute(ctx, tool, args, ec)
	if err != nil {
		res = ToolResult{Content: err.Error(), IsError: true}
	}
	if ec.Hooks != nil {
		_ = ec.Hooks.Run("PostToolUse", map[string]string{
			"O_EVENT": "PostToolUse", "O_TOOL": call.Name, "O_INPUT": string(ArgsJSON(args)), "O_OUTPUT": res.Content,
		})
	}
	if ec.Emit != nil {
		ec.Emit(core.AgentEvent{Type: "tool_result", ID: call.ID, Tool: call.Name, Content: res.Content, IsError: res.IsError})
	}
	return core.Part{Type: "tool_result", ToolUseID: call.ID, Content: res.Content, IsError: res.IsError}
}

// safeExecute runs a tool and converts a panic into an error tool result, so
// one buggy tool can never crash the whole agent process.
func safeExecute(ctx context.Context, tool Tool, args map[string]any, ec *ExecContext) (res ToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = ToolResult{Content: fmt.Sprintf("tool %s panicked: %v", tool.Name(), r), IsError: true}
			err = nil
		}
	}()
	return tool.Execute(ctx, args, ec)
}
