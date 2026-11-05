package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adaop/o/internal/core"
)

func newTestModel(t *testing.T) *Model {
	t.Helper()
	t.Setenv("O_TUI_STATE", filepath.Join(t.TempDir(), "tui.json"))
	m := &Model{
		cwd:                   t.TempDir(),
		th:                    DarkTheme(),
		view:                  viewMain,
		form:                  formFields,
		tab:                   tabChat,
		width:                 100,
		height:                30,
		turnRows:              map[int]int{},
		hoverTurn:             -1,
		animations:            true,
		fontScale:             100,
		density:               "comfortable",
		statusIndicators:      true,
		activityTracking:      true,
		autoContinue:          true,
		executionTransparent:  true,
		notificationsOn:       true,
		noteTTLSeconds:        10,
		rememberDecisions:     true,
		sensitiveProtection:   true,
		fileDiscovery:         true,
		projectContext:        true,
		workspaceIsolation:    true,
		providerStatus:        "not checked",
		rateLimitStatus:       "no limit reported",
	}
	m.st = stylesFor(m.th)
	return m
}

func key(k tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: k} }

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func typeText(m *Model, s string) {
	for _, r := range s {
		m.onKey(runes(string(r)))
	}
}

// ---- startup ----

func TestStartupMenuNavigation(t *testing.T) {
	m := newTestModel(t)
	m.view = viewStartup
	m.onKey(key(tea.KeyDown))
	if m.menu != 1 {
		t.Fatalf("menu=%d want 1", m.menu)
	}
	m.onKey(key(tea.KeyUp))
	if m.menu != 0 {
		t.Fatalf("menu=%d want 0 (wrap)", m.menu)
	}
	m.menu = 3
	_, cmd := m.onKey(key(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter on Exit must return a quit command")
	}
}

// ---- command palette ----

func TestPaletteOpensOnSlashAndFilters(t *testing.T) {
	m := newTestModel(t)
	typeText(m, "/")
	if !m.palette {
		t.Fatal("typing / must open the command palette")
	}
	if len(m.paletteRows) == 0 {
		t.Fatal("the palette must list the available commands")
	}
	typeText(m, "sett")
	found := false
	for _, r := range m.paletteRows {
		if r.cmd.name == "/settings" {
			found = true
		}
	}
	if !found {
		t.Fatalf("/settings not found in %v", m.paletteRows)
	}
}

func TestEveryCommandHasADescription(t *testing.T) {
	for _, c := range allCommands() {
		if strings.TrimSpace(c.description) == "" {
			t.Fatalf("command %s has no description", c.name)
		}
	}
}

func TestPaletteRunSelectsCommand(t *testing.T) {
	m := newTestModel(t)
	typeText(m, "/settings")
	m.onKey(key(tea.KeyEnter))
	if m.tab != tabSettings {
		t.Fatalf("tab=%d want settings", m.tab)
	}
}

// ---- permissions ----

func TestPermissionRequestOpensDialog(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("run the tests")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "bash", Input: json.RawMessage(`{"command":"go test ./..."}`)})
	m.onEvent(core.AgentEvent{Type: "permission_request", ID: "call_1", Tool: "bash", Input: json.RawMessage(`{"command":"go test ./..."}`)})
	if m.perm == nil {
		t.Fatal("a permission request must open the approval dialog")
	}
	if m.perm.id != "call_1" {
		t.Fatalf("dialog id=%q want call_1", m.perm.id)
	}
	if !strings.Contains(m.perm.action.Level, "shell") {
		t.Fatalf("the dialog must state the access level, got %q", m.perm.action.Level)
	}
	if !strings.Contains(m.perm.action.Why, "agent") {
		t.Fatalf("the dialog must explain why it is needed, got %q", m.perm.action.Why)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Permission required") {
		t.Fatalf("the approval dialog is not visible:\n%s", plain)
	}
	if strings.Contains(plain, `"command"`) {
		t.Fatal("the raw payload must not be shown in the dialog")
	}
}

func TestPermissionDecisionIsANotificationNotAChatLine(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("run the tests")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "bash", Input: json.RawMessage(`{"command":"go test ./..."}`)})
	m.onEvent(core.AgentEvent{Type: "permission_request", ID: "call_1", Tool: "bash", Input: json.RawMessage(`{"command":"go test ./..."}`)})
	m.onKey(runes("n"))
	if m.perm != nil {
		t.Fatal("the dialog must close after the decision")
	}
	if len(m.notes) == 0 {
		t.Fatal("the decision must create a notification")
	}
	if m.notes[0].title != "Permission denied" {
		t.Fatalf("notification=%q want Permission denied", m.notes[0].title)
	}
	plain := stripANSI(m.View())
	if strings.Contains(plain, "permission denied for") {
		t.Fatal("the decision must not be printed into the conversation")
	}
}

// ---- plain language actions ----

func TestToolCallBecomesPlainLanguage(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("find the bug")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "grep", Input: json.RawMessage(`{"pattern":"TODO","path":"internal"}`)})
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Searching for") {
		t.Fatalf("the action must be described in plain language:\n%s", plain)
	}
	if strings.Contains(plain, `"pattern"`) || strings.Contains(plain, "{") {
		t.Fatalf("the raw payload must not be rendered:\n%s", plain)
	}
}

func TestActionDescriptions(t *testing.T) {
	info := describeAction("read", map[string]any{"file_path": "internal/tui/model.go"})
	if !strings.Contains(info.Summary, "Reading") {
		t.Fatalf("summary=%q", info.Summary)
	}
	if !strings.Contains(info.Level, "read") {
		t.Fatalf("level=%q", info.Level)
	}
	bash := describeAction("bash", map[string]any{"command": "git status"})
	if !strings.Contains(bash.Summary, "git status") {
		t.Fatalf("summary=%q", bash.Summary)
	}
}

// ---- error classification ----

func TestErrorsAreClassified(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"provider adaop: HTTP 401 (auth): bad key", "API key"},
		{"provider adaop: HTTP 429 (rate_limit): slow down", "rate limiting"},
		{"provider adaop: HTTP 500 (server): boom", "having trouble"},
		{"dial tcp 127.0.0.1:1: connection refused", "reach"},
		{"invalid character 'x' looking for beginning of value", "could not read"},
	}
	for _, c := range cases {
		fe := classifyError(c.raw)
		if !strings.Contains(strings.ToLower(fe.Title), strings.ToLower(c.want)) {
			t.Fatalf("classify(%q).Title=%q want ~%q", c.raw, fe.Title, c.want)
		}
		if fe.Suggest == "" {
			t.Fatalf("classify(%q) has no recovery suggestion", c.raw)
		}
	}
}

// ---- notifications ----

func TestRoutineActivityCreatesNoNotification(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("read a file")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "read", Input: json.RawMessage(`{"file_path":"a.go"}`)})
	m.onEvent(core.AgentEvent{Type: "text", Text: "done"})
	m.onEvent(core.AgentEvent{Type: "done", StopReason: "end_turn"})
	if len(m.notes) == 0 {
		t.Fatal("a finished task creates one completion notification")
	}
	for _, n := range m.notes {
		if n.title == "Permission granted" || n.title == "Full access ON" {
			t.Fatalf("routine activity must not create %q", n.title)
		}
	}
}

func TestFullAccessNotification(t *testing.T) {
	m := newTestModel(t)
	m.notify("warn", "Full access ON", "all tool actions auto-run")
	if len(m.notes) != 1 {
		t.Fatalf("notes=%d want 1", len(m.notes))
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Full access ON") {
		t.Fatalf("the notification area must show the notification:\n%s", plain)
	}
}

// ---- activity bar ----

func TestActivityFollowsTheRunningTool(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("look around")
	m.running = true
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "read", Input: json.RawMessage(`{"file_path":"a.go"}`)})
	if m.activity != "Reading files" {
		t.Fatalf("activity=%q want Reading files", m.activity)
	}
	bar, stop := m.activityBar(100)
	if !strings.Contains(stripANSI(bar), "Reading files") {
		t.Fatalf("activity bar=%q", stripANSI(bar))
	}
	if stop == nil {
		t.Fatal("a stop control must be available while the agent works")
	}
}

func TestElapsedTimerRunsWhileWorking(t *testing.T) {
	m := newTestModel(t)
	m.running = true
	m.started = time.Now().Add(-3200 * time.Millisecond)
	bar, _ := m.activityBar(100)
	if !strings.Contains(stripANSI(bar), "3.2s") {
		t.Fatalf("the elapsed timer is missing: %q", stripANSI(bar))
	}
}

// ---- workflow objects ----

func TestWorkflowObjectsCollapseWhenTheTaskFinishes(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("work")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "read", Input: json.RawMessage(`{"file_path":"a.go"}`)})
	if len(m.cur.steps) != 1 {
		t.Fatalf("steps=%d want 1", len(m.cur.steps))
	}
	m.toggleStep(m.cur.steps[0].id)
	m.onEvent(core.AgentEvent{Type: "tool_result", Tool: "read", Content: "package main"})
	m.onEvent(core.AgentEvent{Type: "done", StopReason: "end_turn"})
	if !m.cur.steps[0].open {
		t.Fatal("a block the user opened must stay open")
	}

	m2 := newTestModel(t)
	m2.newTurn("work")
	m2.onEvent(core.AgentEvent{Type: "tool_call", Tool: "read", Input: json.RawMessage(`{"file_path":"a.go"}`)})
	m2.onEvent(core.AgentEvent{Type: "tool_result", Tool: "read", Content: "package main"})
	m2.onEvent(core.AgentEvent{Type: "done", StopReason: "end_turn"})
	if m2.cur.steps[0].open {
		t.Fatal("a block the user never opened must collapse when the task finishes")
	}
}

func TestThoughtBlockShowsItsRuntime(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("think")
	m.onEvent(core.AgentEvent{Type: "reasoning", Text: "considering the options"})
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Thought for") {
		t.Fatalf("the thought block must show its runtime:\n%s", plain)
	}
}

// ---- conversation ----

func TestTurnHasTimestampAndAnswer(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("hello there")
	m.onEvent(core.AgentEvent{Type: "text", Text: "Hi, I can help with that."})
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "hello there") {
		t.Fatalf("the request is missing:\n%s", plain)
	}
	if !strings.Contains(plain, "Hi, I can help with that.") {
		t.Fatalf("the answer is missing:\n%s", plain)
	}
	if !strings.Contains(plain, IconBot) {
		t.Fatalf("the answer must be a native chat message:\n%s", plain)
	}
	ts := compactTime(m.turns[0].at)
	if !strings.Contains(plain, ts) {
		t.Fatalf("the timestamp %q is missing:\n%s", ts, plain)
	}
}

func TestTimestampTooltipShowsTheFullDate(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("hello")
	m.View()
	var region *clickRegion
	for i := range m.regions {
		if m.regions[i].kind == "time" {
			region = &m.regions[i]
		}
	}
	if region == nil {
		t.Fatal("the timestamp must be hoverable")
	}
	m.onMouse(tea.MouseMsg{Action: tea.MouseActionMotion, X: region.x0 - 1, Y: region.y0 - 1})
	if m.hoverTurn != m.turns[0].id {
		t.Fatalf("hoverTurn=%d want %d", m.hoverTurn, m.turns[0].id)
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, fullTime(m.turns[0].at)) {
		t.Fatalf("the tooltip must show the full date:\n%s", plain)
	}
}

// ---- settings ----

func TestSettingsAreAlphabetical(t *testing.T) {
	m := newTestModel(t)
	rows := m.visibleSettings()
	if len(rows) == 0 {
		t.Fatal("the settings registry is empty")
	}
	lastCat := ""
	seen := map[string]string{}
	for _, r := range rows {
		if r.category != lastCat {
			if lastCat != "" && r.category < lastCat {
				t.Fatalf("category %q comes after %q", r.category, lastCat)
			}
			lastCat = r.category
		}
		if prev, ok := seen[r.category]; ok && r.title < prev {
			t.Fatalf("setting %q comes after %q in %q", r.title, prev, r.category)
		}
		seen[r.category] = r.title
	}
}

func TestSettingsSearchFilters(t *testing.T) {
	m := newTestModel(t)
	m.settings.query = "theme"
	rows := m.visibleSettings()
	if len(rows) != 1 || rows[0].title != "Theme" {
		t.Fatalf("rows=%v", rows)
	}
}

func TestSettingsWorkspaceRendersTheDescription(t *testing.T) {
	m := newTestModel(t)
	m.tab = tabSettings
	m.settings.query = "theme"
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Settings") {
		t.Fatalf("the settings workspace is missing:\n%s", plain)
	}
	if !strings.Contains(plain, "Dark") || !strings.Contains(plain, "Light") {
		t.Fatalf("the theme must only offer Dark and Light:\n%s", plain)
	}
	if !strings.Contains(plain, "well-lit") {
		t.Fatalf("the description of the selected setting is missing:\n%s", plain)
	}
}

// ---- frame ----

func TestViewRendersFullFrame(t *testing.T) {
	m := newTestModel(t)
	m.view = viewStartup
	out := m.View()
	plain := stripANSI(out)
	for _, want := range []string{"Adaop", "autonomous coding agent", "New space", "Exit"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("startup view missing %q\n%s", want, plain)
		}
	}
	lines := strings.Split(plain, "\n")
	if len(lines) != 30 {
		t.Fatalf("frame lines=%d want 30", len(lines))
	}
	for i, l := range lines {
		if w := visWidth(l); w != 100 {
			t.Fatalf("startup line %d width=%d want 100", i, w)
		}
	}
	m.view = viewMain
	frame := m.View()
	plain = stripANSI(frame)
	for _, want := range []string{">", "/ commands"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("main view missing %q\n%s", want, plain)
		}
	}
	// the header is empty: no names, no tabs, no metadata
	if strings.Contains(plain, "Adaop") || strings.Contains(plain, "Providers") {
		t.Fatalf("header must be empty\n%s", plain)
	}
	lines = strings.Split(plain, "\n")
	if len(lines) != 30 {
		t.Fatalf("main frame lines=%d want 30", len(lines))
	}
	for i, l := range lines {
		if w := visWidth(l); w != 100 {
			t.Fatalf("main line %d width=%d want 100: %q", i, w, l)
		}
	}
	// the canvas must stay painted: no style reset without the background
	bg := m.bgSeq()
	if strings.Contains(strings.ReplaceAll(frame, "\x1b[0m"+bg, ""), "\x1b[0m") {
		t.Fatal("frame resets the style without re-applying the background (black gaps)")
	}
}

// stripANSI removes escape sequences so the frame can be inspected as text.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		switch s[i] {
		case '[':
			i++
			for i < len(s) {
				c := s[i]
				i++
				if c >= '@' && c <= '~' {
					break
				}
			}
		case ']':
			i++
			for i < len(s) {
				c := s[i]
				i++
				if c == 0x07 {
					break
				}
				if c == 0x1b && i < len(s) && s[i] == '\\' {
					i++
					break
				}
			}
		default:
			i++
		}
	}
	return b.String()
}

// ---- one failure, one block ----

func TestFailedToolShowsOneBlockOnly(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("read the file")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "read", Input: json.RawMessage(`{"file_path":"missing.go"}`)})
	m.onEvent(core.AgentEvent{Type: "tool_result", Tool: "read", Content: "open missing.go: The system cannot find the file specified.", IsError: true})
	if len(m.cur.steps) != 1 {
		t.Fatalf("steps=%d want 1: the failure must not be shown twice", len(m.cur.steps))
	}
	s := m.cur.steps[0]
	if s.state != stateFailed {
		t.Fatalf("state=%q want failed", s.state)
	}
	if !strings.Contains(s.summary, "not found") {
		t.Fatalf("summary=%q", s.summary)
	}
	plain := stripANSI(m.View())
	if n := strings.Count(plain, "The system cannot find the file"); n > 0 {
		t.Fatalf("the raw backend message is shown %d times:\n%s", n, plain)
	}
}

func TestCommandOutputStaysOutOfTheSummary(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("check the repository")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "bash", Input: json.RawMessage(`{"command":"git status"}`)})
	m.onEvent(core.AgentEvent{Type: "tool_result", Tool: "bash", Content: "On branch master\nChanges not staged for commit:"})
	s := m.cur.steps[0]
	if strings.Contains(s.summary, "branch") || strings.Contains(s.summary, "Changes not staged") {
		t.Fatalf("the raw command output leaked into the summary: %q", s.summary)
	}
	if s.summary != "Command finished" {
		t.Fatalf("summary=%q want Command finished", s.summary)
	}
	// the output is still there, in its own block
	found := false
	for _, sec := range s.sections {
		if sec.title == "Command output" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the command output block is missing: %+v", s.sections)
	}
}

// ---- notifications ----

func TestNotificationIsOneLineAndHidesItself(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("a question that must stay readable")
	m.onEvent(core.AgentEvent{Type: "text", Text: "the answer that must stay readable"})
	m.notify("success", "Task completed", "1 turn")
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "a question that must stay readable") {
		t.Fatalf("the conversation lost its text:\n%s", plain)
	}
	if !strings.Contains(plain, "Task completed") {
		t.Fatalf("the notification is missing:\n%s", plain)
	}
	// a notification is one line, not a panel
	count := 0
	for _, l := range strings.Split(plain, "\n") {
		if strings.Contains(l, "Task completed") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("the notification uses %d lines, want 1:\n%s", count, plain)
	}
	// and it disappears on its own after the configured time
	m.noteTTLSeconds = 5
	m.notes[0].at = time.Now().Add(-10 * time.Second)
	m.expireNotifications()
	if len(m.notes) != 0 {
		t.Fatalf("the notification did not expire: %+v", m.notes)
	}
}

// TestPersistedTheme: the theme choice is written to the state file.
func TestPersistedTheme(t *testing.T) {
	m := newTestModel(t)
	m.SetTheme("light")
	b, err := os.ReadFile(themePath())
	if err != nil || !strings.Contains(string(b), "light") {
		t.Fatalf("theme not persisted: %v %s", err, b)
	}
}

// ---- one decision, one message ----

func TestPermissionDecisionIsAnnouncedOnlyOnce(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("run the tests")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "bash", Input: json.RawMessage(`{"command":"go test ./..."}`)})
	m.onEvent(core.AgentEvent{Type: "permission_request", ID: "call_1", Tool: "bash", Input: json.RawMessage(`{"command":"go test ./..."}`)})
	m.onKey(runes("y"))
	before := len(m.notes)
	// the engine reports the verdict of the same request
	allow := true
	m.onEvent(core.AgentEvent{Type: "permission_request", ID: "call_1", Tool: "bash", Allow: &allow})
	if len(m.notes) != before {
		t.Fatalf("the decision was announced twice: %d notifications", len(m.notes))
	}
	granted := 0
	for _, n := range m.notes {
		if n.title == "Permission granted" {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("permission granted notifications=%d want 1", granted)
	}
}

func TestSecondPermissionRequestWaitsItsTurn(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("run two commands")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "bash", Input: json.RawMessage(`{"command":"one"}`)})
	m.onEvent(core.AgentEvent{Type: "permission_request", ID: "call_1", Tool: "bash", Input: json.RawMessage(`{"command":"one"}`)})
	m.onEvent(core.AgentEvent{Type: "permission_request", ID: "call_2", Tool: "bash", Input: json.RawMessage(`{"command":"two"}`)})
	if m.perm == nil || m.perm.id != "call_1" {
		t.Fatalf("the first request must stay open, got %+v", m.perm)
	}
	if len(m.permQueue) != 1 || m.permQueue[0].id != "call_2" {
		t.Fatalf("the second request must wait in the queue: %+v", m.permQueue)
	}
	m.onKey(runes("y"))
	if m.perm == nil || m.perm.id != "call_2" {
		t.Fatalf("the second request must open after the first was answered: %+v", m.perm)
	}
}

func TestStoppedRunReleasesWaitingApprovals(t *testing.T) {
	b := newAnswerBus(func(core.AgentEvent) {})
	ch := make(chan bool, 1)
	b.pending["call_1"] = ch
	b.denyAll()
	select {
	case allow := <-ch:
		if allow {
			t.Fatal("a stopped run must not approve the waiting request")
		}
	default:
		t.Fatal("the waiting request was never released")
	}
}

// ---- search blocks ----

func TestSearchBlockShowsCountsAndHierarchy(t *testing.T) {
	m := newTestModel(t)
	m.newTurn("find the bug")
	m.onEvent(core.AgentEvent{Type: "tool_call", Tool: "grep", Input: json.RawMessage(`{"pattern":"TODO","path":"internal"}`)})
	m.onEvent(core.AgentEvent{Type: "tool_result", Tool: "grep", Content: "a.go:1 TODO one\na.go:2 TODO two"})
	s := m.cur.steps[0]
	if !strings.Contains(s.summary, "Searched 2") {
		t.Fatalf("summary=%q want a count of the results", s.summary)
	}
	m.toggleStep(s.id)
	plain := stripANSI(m.View())
	for _, want := range []string{"Duration", "Queries", "Results", "Sources"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("the search block is missing the %q section:\n%s", want, plain)
		}
	}
}

// ---- rich answers ----

func TestAnswerRendersFilePreviewAndSources(t *testing.T) {
	m := newTestModel(t)
	answer := "Here it is.\n\n```file " + m.cwd + "/sample.txt\n```\n\n```sources\nhttps://example.com/docs\n```\n"
	if err := os.WriteFile(m.cwd+"/sample.txt", []byte("first line\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plain := stripANSI(strings.Join(m.renderAnswer(answer, 80), "\n"))
	if !strings.Contains(plain, "first line") {
		t.Fatalf("the file preview is missing:\n%s", plain)
	}
	if !strings.Contains(plain, "https://example.com/docs") {
		t.Fatalf("the source panel is missing:\n%s", plain)
	}
}

func TestCommandOutputStaysOutOfTheConversation(t *testing.T) {
	m := newTestModel(t)
	m.showHelp()
	if m.info == nil {
		t.Fatal("the help output must open a panel")
	}
	if m.cur != nil && len(m.cur.steps) != 0 {
		t.Fatalf("the help output must not enter the conversation: %d steps", len(m.cur.steps))
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "esc close") {
		t.Fatalf("the panel is not visible:\n%s", plain)
	}
}
