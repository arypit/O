package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/adaop/o/internal/core"
)

// Workflow object kinds. Every backend operation becomes one of these and
// is rendered as a structured component, never as a raw log line.
const (
	stepThought = "thought" // reasoning / thinking
	stepSearch  = "search"  // searching files, code or the web
	stepAction  = "action"  // any other tool action
	stepCode    = "code"    // writing or changing code
	stepTest    = "test"    // running tests or checks
	stepPlan    = "plan"    // task list / planning
	stepError   = "error"   // a failure, rendered as an explanation
)

// step states.
const (
	stateRunning = "running"
	stateDone    = "done"
	stateFailed  = "failed"
)

// metaRow is one labelled detail inside an expanded workflow object.
type metaRow struct {
	label string
	value string
}

// section is one group of details inside an expanded workflow object, for
// example the queries and the results of a search.
type section struct {
	title string
	items []string
}

// step is one workflow object in the chain that produced a result.
// It tracks its runtime, its state and whether the user opened it.
type step struct {
	id      int
	kind    string
	title   string
	summary string // compact line shown while collapsed
	detail  string // body shown while expanded
	sections []section
	tool    string
	args    map[string]any
	meta    []metaRow
	started time.Time
	ended   time.Time
	state   string
	manual  bool // the user opened it: never collapsed automatically
	open    bool // current expansion state
}

// duration returns how long the step took.
func (s *step) duration() time.Duration {
	if s.started.IsZero() {
		return 0
	}
	if s.ended.IsZero() {
		return time.Since(s.started)
	}
	return s.ended.Sub(s.started)
}

// turn is one user request together with everything the agent did in
// response: the workflow chain and the final answer.
type turn struct {
	id        int
	at        time.Time
	request   string
	steps     []*step
	response  string
	answerAt  time.Time
	streaming bool
	usage     *core.Usage
}

// newTurn starts a new conversation block.
func (m *Model) newTurn(request string) *turn {
	m.turnSeq++
	t := &turn{id: m.turnSeq, at: time.Now(), request: request, streaming: true}
	m.turns = append(m.turns, t)
	m.cur = t
	return t
}

// curStep returns the step that is currently running, if any.
func (m *Model) curStep() *step {
	if m.cur == nil {
		return nil
	}
	for i := len(m.cur.steps) - 1; i >= 0; i-- {
		if m.cur.steps[i].state == stateRunning {
			return m.cur.steps[i]
		}
	}
	return nil
}

// addStep appends a workflow object to the active turn.
func (m *Model) addStep(kind, title, summary string) *step {
	if m.cur == nil {
		m.newTurn("")
	}
	m.stepSeq++
	s := &step{
		id:      m.stepSeq,
		kind:    kind,
		title:   title,
		summary: summary,
		started: time.Now(),
		state:   stateRunning,
	}
	m.cur.steps = append(m.cur.steps, s)
	return s
}

// closeStep finishes the running step and attaches its detail.
func (m *Model) closeStep(detail string, failed bool) {
	s := m.curStep()
	if s == nil {
		return
	}
	s.ended = time.Now()
	if failed {
		s.state = stateFailed
	} else {
		s.state = stateDone
	}
	if detail != "" {
		s.detail = detail
	}
}

// stepForTool returns the workflow kind for a tool call.
func stepForTool(tool string) string {
	switch tool {
	case "grep", "glob", "webfetch":
		return stepSearch
	case "edit", "write":
		return stepCode
	case "bash":
		return stepTest
	case "todo", "plan":
		return stepPlan
	default:
		return stepAction
	}
}

// toggleStep opens or closes a workflow object. A block the user opened is
// never collapsed automatically.
func (m *Model) toggleStep(id int) {
	for _, t := range m.turns {
		for _, s := range t.steps {
			if s.id != id {
				continue
			}
			s.open = !s.open
			s.manual = s.open
			return
		}
	}
}

// collapseFinished collapses every workflow object of the finished turn that
// the user never opened, so the final answer stays the focus.
func (m *Model) collapseFinished() {
	if m.cur == nil {
		return
	}
	for _, s := range m.cur.steps {
		if s.manual {
			continue
		}
		s.open = false
	}
}

// thoughtSummary is the compact label of a thinking block.
func thoughtSummary(d time.Duration) string {
	return fmt.Sprintf("Thought for %s", formatDuration(d))
}

// searchSummary is the compact label of a search block: how much was
// searched, in the words of the user ("Searched 16 websites").
func searchSummary(tool string, n int) string {
	switch tool {
	case "webfetch":
		if n <= 0 {
			return "Searched the web"
		}
		return fmt.Sprintf("Searched %d websites", n)
	case "grep":
		if n <= 0 {
			return "Searched the code"
		}
		return fmt.Sprintf("Searched %d matches", n)
	case "glob":
		if n <= 0 {
			return "Looked for files"
		}
		return fmt.Sprintf("Found %d files", n)
	default:
		if n <= 0 {
			return "Search finished"
		}
		return fmt.Sprintf("Searched %d results", n)
	}
}

// formatDuration renders a duration the way an activity timer does.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %02ds", mins, secs)
}

// compactTime is the short timestamp of a message ("3:44 PM").
func compactTime(t time.Time) string {
	return t.Local().Format("3:04 PM")
}

// fullTime is the detailed timestamp of a message.
func fullTime(t time.Time) string {
	return t.Local().Format("Monday, January 2, 2006 at 3:04:05 PM")
}

// answerText returns the final answer of a turn.
func (t *turn) answerText() string {
	return strings.TrimRight(t.response, "\n")
}
