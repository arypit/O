package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adaop/o/internal/core"
)

// onEvent translates one backend event into UI components. Nothing raw is
// ever shown: tool calls become workflow objects, failures become
// explanations and important events become notifications.
func (m *Model) onEvent(ev core.AgentEvent) {
	switch ev.Type {
	case "text":
		if m.cur == nil {
			m.newTurn("")
		}
		m.cur.response += ev.Text
		m.cur.streaming = true
		m.activity = "Writing response"

	case "reasoning":
		if !m.executionTransparent {
			m.activity = "Thinking"
			return
		}
		m.activity = "Thinking"
		s := m.curStep()
		if s == nil || s.kind != stepThought {
			s = m.addStep(stepThought, "Thinking", "Thought")
		}
		s.detail += ev.Text
		s.summary = thoughtSummary(s.duration())

	case "tool_call":
		args := decodeArgs(ev.Input)
		info := describeAction(ev.Tool, args)
		m.activity = activityLabel(ev.Tool)
		if !m.executionTransparent {
			return
		}
		s := m.addStep(stepForTool(ev.Tool), info.Title, info.Summary)
		s.tool = ev.Tool
		s.args = args
		s.detail = info.Detail

	case "tool_result":
		s := m.curStep()
		if ev.IsError {
			fe := classifyError(ev.Content)
			m.activity = "Recovering from an error"
			// One failure, one block: the step that was running becomes
			// the failed block and carries the explanation. No second
			// copy of the same message is added.
			if s == nil {
				s = m.addStep(stepError, fe.Title, fe.Title)
				s.tool = ev.Tool
			}
			s.ended = time.Now()
			s.state = stateFailed
			s.summary = "Failed · " + fe.Title
			s.detail = fe.Detail
			s.sections = []section{{title: "What to do", items: []string{fe.Suggest}}}
			m.notify("error", fe.Title, fe.Detail)
			return
		}
		if s == nil {
			return
		}
		s.ended = time.Now()
		s.state = stateDone
		if s.tool == "bash" {
			// The command output is kept out of the summary line: it is
			// shown as its own block when the step is opened.
			s.summary = "Command finished"
			s.sections = []section{{title: "Command output", items: resultItems(ev.Content)}}
		} else if s.kind == stepSearch {
			// A search block carries a hierarchy: how long it took, what
			// was looked for, what came back and where it came from.
			items := resultItems(ev.Content)
			s.summary = searchSummary(s.tool, len(items))
			s.sections = []section{
				{title: "Duration", items: []string{formatDuration(s.duration())}},
				{title: "Queries", items: searchQueries(s.tool, s.args)},
				{title: "Results", items: items},
				{title: "Sources", items: sourceItems(s, ev.Content)},
			}
		} else {
			s.summary = resultSummary(s.tool, s.args, ev.Content)
		}

	case "permission_request":
		if ev.Allow == nil {
			args := decodeArgs(ev.Input)
			info := describeAction(ev.Tool, args)
			req := &permReq{id: ev.ID, tool: ev.Tool, action: info}
			if s := m.curStep(); s != nil {
				req.stepID = s.id
				req.prevSummary = s.summary
				s.summary = "Waiting for approval · " + s.summary
			}
			if m.perm == nil {
				m.perm = req
			} else {
				// a second request waits behind the first one instead of
				// replacing it: one dialog, one answer, one call.
				m.permQueue = append(m.permQueue, req)
			}
			m.activity = "Waiting for your approval"
			return
		}
		// The verdict closes the dialog. The decision itself was announced
		// once, when the user answered, so it is never repeated here.
		m.dropPermission(ev.ID)
		if m.perm == nil {
			m.nextPermission()
		}

	case "todo":
		if !m.executionTransparent {
			return
		}
		m.activity = "Planning"
		var b strings.Builder
		for i, td := range ev.Todos {
			if i > 0 {
				b.WriteString("  ")
			}
			icon := "○"
			switch td.Status {
			case "completed":
				icon = IconOK
			case "in_progress":
				icon = IconTool
			}
			b.WriteString(icon + " " + td.Content)
		}
		s := m.addStep(stepPlan, "Task list", "Task list updated")
		s.detail = b.String()
		s.ended = time.Now()
		s.state = stateDone

	case "turn_end":
		if ev.Usage != nil && m.cur != nil {
			m.cur.usage = ev.Usage
		}

	case "done":
		m.running = false
		m.activity = ""
		if m.cur != nil {
			m.cur.streaming = false
			m.cur.answerAt = time.Now()
			m.collapseFinished()
		}
		if ev.Usage != nil && m.cur != nil {
			m.cur.usage = ev.Usage
		}
		if ev.StopReason == "paused" {
			m.status = "paused — press enter to continue"
			m.statusErr = false
			return
		}
		m.status = "done"
		m.statusErr = false
		m.notify("success", "Task completed", fmt.Sprintf("%d turns", 1))

	case "error":
		fe := classifyError(ev.Message)
		m.running = false
		m.activity = ""
		if m.cur != nil {
			errStep := m.addStep(stepError, fe.Title, fe.Title)
			errStep.detail = fe.Detail
			errStep.sections = append(errStep.sections, section{title: "What to do", items: []string{fe.Suggest}})
			errStep.ended = time.Now()
			errStep.state = stateFailed
			m.cur.streaming = false
		}
		m.status = fe.Title
		m.statusErr = true
		m.notify("error", fe.Title, fe.Detail)

	case "ready":
		m.running = false
		m.activity = ""
		if m.status == "" {
			m.status = "ready"
		}

	case "sys":
		// the engine banner is not part of the conversation
		if strings.HasPrefix(ev.Message, "O ready:") || strings.HasPrefix(ev.Message, "O chat ready:") {
			return
		}
		m.note(ev.Message)
	}
}

// note adds a system notice to the conversation.
func (m *Model) note(text string) {
	if m.cur == nil {
		m.newTurn("")
	}
	s := m.addStep(stepAction, "Notice", text)
	s.detail = text
	s.ended = time.Now()
	s.state = stateDone
}

// closeRunningStep finishes the workflow object that was still running.
func (m *Model) closeRunningStep() {
	if s := m.curStep(); s != nil {
		s.ended = time.Now()
		s.state = stateFailed
		s.summary = "Interrupted"
	}
}

// answerPermission answers the pending approval request. The decision is
// shown as a notification, never as a line in the conversation, and it is
// announced exactly once.
func (m *Model) answerPermission(allow, always bool) {
	if m.perm == nil {
		return
	}
	p := m.perm
	m.perm = nil
	m.restoreStepSummary(p)
	if always {
		if m.eng != nil && m.rememberDecisions {
			m.eng.AllowAlways(p.tool)
		}
	}
	if m.eng != nil {
		m.eng.Answer(p.id, allow)
	}
	if allow {
		m.notify("success", "Permission granted", p.action.Title)
	} else {
		m.notify("warn", "Permission denied", p.action.Title)
	}
	m.nextPermission()
}

// nextPermission shows the next waiting approval request, if there is one.
func (m *Model) nextPermission() {
	if m.perm != nil {
		return
	}
	if len(m.permQueue) == 0 {
		return
	}
	m.perm = m.permQueue[0]
	m.permQueue = m.permQueue[1:]
	m.activity = "Waiting for your approval"
}

// dropPermission removes a request from the queue, for example when the
// engine already answered it.
func (m *Model) dropPermission(id string) {
	if m.perm != nil && m.perm.id == id {
		m.perm = nil
	}
	kept := m.permQueue[:0]
	for _, r := range m.permQueue {
		if r.id != id {
			kept = append(kept, r)
		}
	}
	m.permQueue = kept
}

// restoreStepSummary removes the "waiting for approval" label from the
// workflow object that was blocked.
func (m *Model) restoreStepSummary(p *permReq) {
	if p == nil || p.stepID == 0 {
		return
	}
	for _, t := range m.turns {
		for _, s := range t.steps {
			if s.id == p.stepID {
				s.summary = p.prevSummary
				return
			}
		}
	}
}

// permissionKey handles the keys of the approval dialog.
func (m *Model) permissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.answerPermission(true, false)
	case "a", "A":
		m.answerPermission(true, true)
	case "n", "N":
		m.answerPermission(false, false)
	}
	return m, nil
}

// searchQueries describes what a search step looked for.
func searchQueries(tool string, args map[string]any) []string {
	switch tool {
	case "grep":
		pat := argString(args, "pattern")
		where := argString(args, "path")
		if where == "" {
			where = "the workspace"
		}
		return []string{fmt.Sprintf("%q in %s", pat, where)}
	case "glob":
		pat := argString(args, "pattern")
		where := argString(args, "path")
		if where == "" {
			where = "the workspace"
		}
		return []string{fmt.Sprintf("files matching %s in %s", pat, where)}
	case "webfetch":
		return []string{argString(args, "url")}
	default:
		return nil
	}
}

// resultItems splits a tool result into readable lines.
func resultItems(content string) []string {
	var out []string
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out = append(out, clipText(l, 100))
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// sourceItems lists where the information of a search step came from.
func sourceItems(s *step, content string) []string {
	var out []string
	if s.tool == "webfetch" && s.detail != "" {
		out = append(out, s.detail)
	}
	for _, u := range extractURLs(content) {
		if !containsString(out, u) {
			out = append(out, u)
		}
		if len(out) >= 8 {
			break
		}
	}
	return out
}

// extractURLs finds the web addresses inside a text.
func extractURLs(text string) []string {
	var out []string
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if i+4 <= len(runes) && string(runes[i:i+4]) == "http" {
			j := i
			for j < len(runes) && !isURLStop(runes[j]) {
				j++
			}
			u := strings.TrimRight(string(runes[i:j]), ".,;:)]")
			if len(u) > 10 && !containsString(out, u) {
				out = append(out, u)
			}
			i = j
		}
	}
	return out
}

// isURLStop reports whether a rune ends a web address.
func isURLStop(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '"', '\'', '<', '>', '`', '|':
		return true
	}
	return false
}

// containsString reports whether s contains v.
func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
