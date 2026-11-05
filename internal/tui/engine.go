package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/adaop/o/internal/agent"
	"github.com/adaop/o/internal/config"
	octx "github.com/adaop/o/internal/context"
	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/hitl"
	"github.com/adaop/o/internal/hooks"
	"github.com/adaop/o/internal/llm"
	"github.com/adaop/o/internal/sandbox"
	"github.com/adaop/o/internal/session"
	"github.com/adaop/o/internal/subagent"
	"github.com/adaop/o/internal/tools"
	"github.com/adaop/o/internal/tools/builtin"
)

// Engine drives the agent in-process and exposes its events as a channel.
type Engine struct {
	cfg     config.Config
	cwd     string
	home    string
	a       *agent.Agent
	bus     *answerBus
	events  chan core.AgentEvent
	done    chan struct{}
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	pid     string
	model   string
	sessDir string
	learner *hitl.Learner
	// noticeShown keeps the provider notice to one message per session.
	noticeShown bool
}

type answerBus struct {
	mu      sync.Mutex
	n       int
	pending map[string]chan bool
	emit    func(core.AgentEvent)
}

func newAnswerBus(emit func(core.AgentEvent)) *answerBus {
	return &answerBus{pending: map[string]chan bool{}, emit: emit}
}

func (b *answerBus) deliver(id string, allow bool) {
	b.mu.Lock()
	ch := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if ch != nil {
		ch <- allow
	}
}

// denyAll releases every waiting approval request. It is used when the run
// is stopped, so no tool call is left waiting for an answer that will never
// come.
func (b *answerBus) denyAll() {
	b.mu.Lock()
	pending := b.pending
	b.pending = map[string]chan bool{}
	b.mu.Unlock()
	for _, ch := range pending {
		ch <- false
	}
}

type busDecider struct{ b *answerBus }

// AskID registers the request under its tool-call id, emits exactly one
// request event and one verdict event, and waits for the answer. Registering
// before emitting removes the race where the answer arrives first.
func (d busDecider) AskID(id, tool, summary string, args json.RawMessage) (bool, error) {
	b := d.b
	if id == "" {
		b.mu.Lock()
		b.n++
		id = "perm_" + strconv.Itoa(b.n)
		b.mu.Unlock()
	}
	ch := make(chan bool, 1)
	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()

	input := args
	if len(input) == 0 {
		input = json.RawMessage(strconv.Quote(summary))
	}
	b.emit(core.AgentEvent{Type: "permission_request", ID: id, Tool: tool, Input: input})
	allow := <-ch
	b.emit(core.AgentEvent{Type: "permission_request", ID: id, Tool: tool, Allow: &allow})
	return allow, nil
}

func (d busDecider) Ask(tool, summary string) (bool, error) {
	return d.AskID("", tool, summary, nil)
}

// NewEngine builds the full agent stack for cwd, exactly like `o chat`,
// with provider fallback across all configured providers that have keys.
func NewEngine(cwd string) (*Engine, error) {
	home, _ := os.UserHomeDir()
	cfg, err := config.Load(cwd)
	if err != nil {
		return nil, err
	}
	sb, err := sandbox.New(cwd, sandbox.WorkspaceWrite)
	if err != nil {
		return nil, err
	}
	rules := map[string][]tools.Rule{}
	for tool, raw := range cfg.Permissions.Rules {
		rules[tool] = tools.ParseRuleList(raw)
	}
	policy := tools.NewPolicy(tools.Mode(cfg.Permissions.Mode), rules)

	reg := tools.NewRegistry()
	builtin.RegisterDefaults(reg)
	builtin.RegisterMutating(reg)
	builtin.RegisterPlan(reg, builtin.PlanCandidates(reg))

	// providers: default first, then the rest; only ones with a resolvable key
	order := []string{cfg.DefaultProvider}
	for _, id := range config.ListProviders(cfg) {
		if id != cfg.DefaultProvider {
			order = append(order, id)
		}
	}
	var provs []llm.Provider
	for _, id := range order {
		pc, ok := cfg.Providers[id]
		if !ok {
			continue
		}
		key := pc.APIKey
		if pc.APIKeyEnv != "" {
			key = os.Getenv(pc.APIKeyEnv)
		}
		if key == "" {
			continue
		}
		provs = append(provs, llm.NewOpenAI(id, pc.BaseURL, key))
	}
	if len(provs) == 0 {
		return nil, fmt.Errorf("no provider with an API key — add one with `o login` (or set the env key)")
	}
	var provider llm.Provider
	pid := cfg.DefaultProvider
	if len(provs) == 1 {
		provider = provs[0]
	} else {
		provider = llm.NewFallback(provs...)
		pid = provider.ID()
	}
	modelName := "o-1"
	if pc, ok := cfg.Providers[cfg.DefaultProvider]; ok && pc.Model != "" {
		modelName = pc.Model
	}

	sessDir := cfg.Paths.SessionsDir
	sess, err := session.New(sessDir)
	if err != nil {
		return nil, err
	}
	store := octx.NewStore()
	learner := hitl.NewLearner(filepath.Join(session.ExpandDir("~/.o"), "hitl.json"))
	hks := hooks.New(cfg.Hooks)

	e := &Engine{
		cfg: cfg, cwd: cwd, home: home,
		events: make(chan core.AgentEvent, 4096), done: make(chan struct{}),
		pid: pid, model: modelName, sessDir: sessDir, learner: learner,
	}
	emit := func(ev core.AgentEvent) {
		select {
		case e.events <- ev:
		case <-e.done:
		}
	}
	e.bus = newAnswerBus(emit)
	a := &agent.Agent{
		Cfg: &cfg, Provider: provider, Model: modelName,
		Reg: reg, Policy: policy, SB: sb, Store: store, Sess: sess,
		Decider: busDecider{e.bus}, Learner: learner, Hooks: hks,
		Emit: emit, CWD: cwd, Home: home, AllowOnce: tools.NewAllowSet(),
	}
	if ac, ok := cfg.Agents["primary"]; ok && ac.MaxTurns > 0 {
		a.MaxTurns = ac.MaxTurns
	}
	reg.Add(subagent.TaskTool{Factory: func(name, prompt string) (*agent.Agent, error) {
		childReg := tools.NewRegistry()
		builtin.RegisterDefaults(childReg)
		childSess, err := session.New(sessDir)
		if err != nil {
			return nil, err
		}
		childCfg := cfg
		return &agent.Agent{
			Cfg: &childCfg, Provider: provider, Model: modelName,
			Reg: childReg, Policy: policy, SB: sb, Store: octx.NewStore(), Sess: childSess,
			Decider: hitl.NewAutoDecider(learner), Learner: learner, Hooks: hks,
			Emit: emit, CWD: cwd, Home: home, Depth: a.Depth + 1, AllowOnce: tools.NewAllowSet(),
		}, nil
	}})
	e.a = a
	emit(core.AgentEvent{Type: "sys", Message: fmt.Sprintf("O ready: session=%s provider=%s model=%s mode=%s sandbox=workspace-write", sess.ID, pid, modelName, cfg.Permissions.Mode)})
	return e, nil
}

func (e *Engine) Events() <-chan core.AgentEvent { return e.events }

func (e *Engine) Info() (sessionID, provider, model string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	id := ""
	if e.a != nil && e.a.Sess != nil {
		id = e.a.Sess.ID
	}
	return id, e.pid, e.model
}

func (e *Engine) Mode() string { return e.cfg.Permissions.Mode }

// SetMode switches the permission mode of the running agent.
func (e *Engine) SetMode(mode string) error {
	if !config.ValidMode(mode) {
		return fmt.Errorf("invalid permission mode %q", mode)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.a.Policy.SetMode(tools.Mode(mode))
	e.cfg.Permissions.Mode = mode
	return nil
}

// AllowAlways approves a tool for the rest of the session.
func (e *Engine) AllowAlways(tool string) { e.a.AllowOnce.Set(tool) }

// ClearAlways forgets every approval of this session.
func (e *Engine) ClearAlways() { e.a.AllowOnce.Clear() }

// SetAutoContinue lets the agent continue through multi-step workflows.
func (e *Engine) SetAutoContinue(on bool) { e.a.PauseBetweenSteps = !on }

// SetPlanFirst asks the agent to plan before it acts.
func (e *Engine) SetPlanFirst(on bool) { e.a.PlanFirst = on }

// SetProjectContext includes or excludes the project context in the prompt.
func (e *Engine) SetProjectContext(on bool) { e.a.NoProjectContext = !on }

// SetSensitiveProtection forces an approval for destructive commands.
func (e *Engine) SetSensitiveProtection(on bool) { e.a.Policy.SetSensitiveProtection(on) }

// SetFileDiscovery offers the search tools to the agent.
func (e *Engine) SetFileDiscovery(on bool) {
	e.a.Reg.SetEnabled("glob", on)
	e.a.Reg.SetEnabled("grep", on)
}

// Workspace returns the project folder the agent works in.
func (e *Engine) Workspace() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.a.CWD
}

// AccessLevel returns the sandbox scope of the session.
func (e *Engine) AccessLevel() string { return "workspace-write" }

// SetWorkspace moves the agent to another project folder.
func (e *Engine) SetWorkspace(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("cannot use %s: %w", path, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a folder", path)
	}
	sb, err := sandbox.New(abs, sandbox.WorkspaceWrite)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.a.SB = sb
	e.a.CWD = abs
	e.mu.Unlock()
	return nil
}

// ModelOptions lists the models the configured providers serve.
func (e *Engine) ModelOptions() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, id := range config.ListProviders(e.cfg) {
		pc := e.cfg.Providers[id]
		if pc.Model == "" || seen[pc.Model] {
			continue
		}
		seen[pc.Model] = true
		out = append(out, pc.Model)
	}
	return out
}

// SetModel switches the agent to another configured model.
func (e *Engine) SetModel(model string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, id := range config.ListProviders(e.cfg) {
		pc := e.cfg.Providers[id]
		if pc.Model != model {
			continue
		}
		key := pc.APIKey
		if pc.APIKeyEnv != "" {
			key = os.Getenv(pc.APIKeyEnv)
		}
		if key == "" {
			return fmt.Errorf("provider %q has no API key", id)
		}
		e.a.Provider = llm.NewOpenAI(id, pc.BaseURL, key)
		e.a.Model = model
		e.pid = id
		e.model = model
		return nil
	}
	return fmt.Errorf("no configured provider serves the model %q", model)
}

// Prompt starts one agent run; returns false if one is already running.
func (e *Engine) Prompt(text string) bool {
	return e.start(func(ctx context.Context) (string, error) { return e.a.Run(ctx, text) })
}

// Continue resumes a paused run without adding a new user message.
func (e *Engine) Continue() bool {
	return e.start(func(ctx context.Context) (string, error) { return e.a.Resume(ctx) })
}

// start runs one agent turn in the background.
func (e *Engine) start(run func(context.Context) (string, error)) bool {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return false
	}
	e.running = true
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.mu.Unlock()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				e.safeEmit(core.AgentEvent{Type: "error", Message: fmt.Sprintf("internal panic: %v", r)})
			}
			e.mu.Lock()
			e.running = false
			e.mu.Unlock()
			if e.learner != nil {
				_ = e.learner.Save()
			}
			e.safeEmit(core.AgentEvent{Type: "ready"})
		}()
		_, err := run(ctx)
		if err != nil && ctx.Err() == nil {
			e.safeEmit(core.AgentEvent{Type: "error", Message: err.Error()})
		}
		e.reportProviderNotice()
	}()
	return true
}

func (e *Engine) safeEmit(ev core.AgentEvent) {
	select {
	case e.events <- ev:
	case <-e.done:
	}
}

// reportProviderNotice tells the user when the provider had to be asked
// without tool definitions, so the missing tools are explained instead of
// looking like a broken agent.
func (e *Engine) reportProviderNotice() {
	e.mu.Lock()
	p := e.a.Provider
	e.mu.Unlock()
	w, ok := p.(interface{ Warning() string })
	if !ok {
		return
	}
	msg := w.Warning()
	if msg == "" || e.noticeShown {
		return
	}
	e.noticeShown = true
	e.safeEmit(core.AgentEvent{Type: "sys", Message: msg})
}

func (e *Engine) Abort() {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Unlock()
	// release every waiting approval so the stopped run can unwind
	e.bus.denyAll()
}

func (e *Engine) Answer(permID string, allow bool) { e.bus.deliver(permID, allow) }

// NewSession starts a fresh conversation in the same engine.
func (e *Engine) NewSession() (string, error) {
	sess, err := session.New(e.sessDir)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	e.a.Sess = sess
	e.a.Store = octx.NewStore()
	e.mu.Unlock()
	return sess.ID, nil
}

func (e *Engine) Close() {
	e.Abort()
	select {
	case <-e.done:
	default:
		close(e.done)
	}
}

// ---- providers tab helpers ----

type ProvRow struct {
	ID, Base, Model, Key string
	Default              bool
}

func maskKey(k string) string {
	if k == "" {
		return "(not set)"
	}
	if len(k) <= 4 {
		return "****"
	}
	return k[:2] + "..." + k[len(k)-2:]
}

func LoadProvRows(cwd string) []ProvRow {
	cfg, err := config.Load(cwd)
	if err != nil {
		return nil
	}
	var rows []ProvRow
	for _, id := range config.ListProviders(cfg) {
		pc := cfg.Providers[id]
		key := pc.APIKey
		if pc.APIKeyEnv != "" {
			if v := os.Getenv(pc.APIKeyEnv); v != "" {
				key = "env " + pc.APIKeyEnv
			}
		}
		rows = append(rows, ProvRow{ID: id, Base: pc.BaseURL, Model: pc.Model, Key: maskKey(key), Default: id == cfg.DefaultProvider})
	}
	return rows
}

// TestProvider does a REAL GET {baseUrl}/models connectivity check.
func TestProvider(cwd, id string) string {
	cfg, err := config.Load(cwd)
	if err != nil {
		return "error: " + err.Error()
	}
	pc, ok := cfg.Providers[id]
	if !ok {
		return "unknown provider " + id
	}
	key := pc.APIKey
	if pc.APIKeyEnv != "" {
		key = os.Getenv(pc.APIKeyEnv)
	}
	if key == "" {
		return "no API key"
	}
	req, err := http.NewRequest(http.MethodGet, pc.BaseURL+"/models", nil)
	if err != nil {
		return "error: " + err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+key)
	cl := &http.Client{Timeout: 12 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return "unreachable: " + err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	n := 0
	if resp.StatusCode == http.StatusOK {
		var m struct {
			Data []any `json:"data"`
		}
		if json.Unmarshal(body, &m) == nil {
			n = len(m.Data)
		}
	}
	if resp.StatusCode == http.StatusOK {
		return fmt.Sprintf("HTTP 200 — %d models available", n)
	}
	return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, oneLine(string(body), 120))
}

func oneLine(s string, max int) string {
	out := ""
	for _, r := range s {
		if r == '\n' || r == '\r' {
			out += " "
		} else {
			out += string(r)
		}
	}
	if n := len([]rune(out)); n > max {
		out = string([]rune(out)[:max]) + "…"
	}
	return out
}
