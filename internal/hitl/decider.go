// Deciders answer "ask" permission requests.
//
// InteractiveDecider asks the human on the terminal (y/n/always) and records
// the decision in the Learner. AutoDecider is used in headless mode: it
// approves only when the learned acceptance probability is high, otherwise
// it denies (safe default).
package hitl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Decider answers whether a tool call may run.
type Decider interface {
	// Ask returns true to allow. summary describes the call, e.g. "git status".
	Ask(tool, summary string) (bool, error)
}

// IDDecider is implemented by deciders that can correlate the human's
// answer with one specific request. The id is the tool-call id, so the
// approval dialog and the waiting tool call always refer to the same
// request: exactly one request and one verdict per call.
type IDDecider interface {
	// AskID returns true to allow. args carries the raw tool arguments so
	// the UI can describe the request in plain language.
	AskID(id, tool, summary string, args json.RawMessage) (bool, error)
}

// InteractiveDecider prompts a human. "always" remembers the tool for the session.
type InteractiveDecider struct {
	In      *bufio.Reader
	Out     io.Writer
	Learner *Learner
	once    sync.Mutex
	always  map[string]bool
}

func NewInteractiveDecider(in io.Reader, out io.Writer, learner *Learner) *InteractiveDecider {
	return &InteractiveDecider{In: bufio.NewReader(in), Out: out, Learner: learner, always: map[string]bool{}}
}

func (d *InteractiveDecider) Ask(tool, summary string) (bool, error) {
	d.once.Lock()
	if d.always[tool] {
		d.once.Unlock()
		return true, nil
	}
	d.once.Unlock()
	label := tool
	if summary != "" {
		label = tool + "(" + summary + ")"
	}
	for {
		fmt.Fprintf(d.Out, "\nO wants to run: %s\nAllow? [y]es / [n]o / [a]lways: ", label)
		line, err := d.In.ReadString('\n')
		if err != nil && line == "" {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "":
			d.record(tool, summary, true)
			return true, nil
		case "a", "always":
			d.once.Lock()
			d.always[tool] = true
			d.once.Unlock()
			d.record(tool, summary, true)
			return true, nil
		case "n", "no":
			d.record(tool, summary, false)
			return false, nil
		}
	}
}

func (d *InteractiveDecider) record(tool, summary string, ok bool) {
	if d.Learner == nil {
		return
	}
	d.Learner.Record(tool, ok)
	d.Learner.Record(tool+"|"+summary, ok)
	_ = d.Learner.Save()
}

// AutoDecider never prompts; it uses the learner and otherwise denies.
type AutoDecider struct {
	Learner       *Learner
	MinSamples    int
	MinProb       float64
}

func NewAutoDecider(learner *Learner) *AutoDecider {
	return &AutoDecider{Learner: learner, MinSamples: 10, MinProb: 0.9}
}

func (d *AutoDecider) Ask(tool, summary string) (bool, error) {
	if d.Learner == nil {
		return false, nil
	}
	if d.Learner.Samples(tool) >= d.MinSamples && d.Learner.Prob(tool) >= d.MinProb {
		return true, nil
	}
	return false, nil
}
