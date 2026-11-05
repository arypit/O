package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/adaop/o/internal/config"
	"github.com/adaop/o/internal/core"
	"github.com/adaop/o/internal/session"
)

// The O TUI is a native full-screen terminal UI built on Bubble Tea.
// Bubble Tea owns the console (raw mode, alternate screen, keyboard and
// mouse reporting), so key and mouse events arrive as real messages.
//
// The model is a translation layer: every backend event becomes a UI
// component — a conversation turn, a workflow object, a permission request,
// an error explanation or a notification. No raw payload is ever rendered.

type viewMode int

const (
	viewStartup viewMode = iota
	viewMain
)

type tabID int

const (
	tabChat tabID = iota
	tabProviders
	tabChangelog
	tabSettings
)

// permission modes used by the model.
const (
	defaultMode = "default"
	fullAutoMode = "full-auto"
)

// permReq is one pending approval request.
type permReq struct {
	id     string
	tool   string
	action actionInfo
	// stepID is the workflow object that waits for this approval, so the
	// "waiting for approval" label can be removed once it is answered.
	stepID int
	// prevSummary is the summary the workflow object had before it waited.
	prevSummary string
}

// agentMsg carries one engine event into the Bubble Tea loop.
type agentMsg struct{ ev core.AgentEvent }

// infoPanel is a titled block of command output.
type infoPanel struct {
	title string
	lines []string
}

// tickMsg drives the spinner, the elapsed timer and the notification expiry.
type tickMsg time.Time

// clickRegion is a mouse-clickable rectangle (1-based rows and columns).
type clickRegion struct {
	y0, y1 int
	x0, x1 int
	kind  string // menu | provider | setting | step | time | stop | notice
	index int
	value int
	id    int
}

// Model is the Bubble Tea model of the O TUI.
type Model struct {
	cwd string
	th  Theme
	st  Styles

	eng    *Engine
	engErr string

	view viewMode
	menu int
	tab  tabID

	// conversation
	turns   []*turn
	turnSeq int
	cur     *turn
	stepSeq int
	scroll  int

	// composer
	input  []rune
	cursor int
	hist   []string

	// execution state
	status    string
	statusErr bool
	running   bool
	spin      int
	started   time.Time
	activity  string

	// permission dialog
	perm      *permReq
	permQueue []*permReq

	// command palette
	palette     bool
	paletteSel  int
	paletteRows []paletteRow

	// notification layer
	notes       []notification
	noteHistory []notification
	noteSeq     int

	// info panel: the output of /help, /sessions and /notifications. It
	// floats above the composer, so command output never mixes into the
	// conversation.
	info *infoPanel

	// settings workspace
	settings settingsState

	// providers and changelog views
	prov     []ProvRow
	provSel  int
	provMode int // 0 list, 1 add-form
	form     []string
	formVals []string
	formIdx  int

	chlog    []string
	chScroll int

	refreshHz int

	// preferences
	animations           bool
	fontScale            int
	density              string
	statusIndicators     bool
	activityTracking     bool
	autoContinue         bool
	executionTransparent bool
	planFirst            bool
	notificationsOn      bool
	noteTTLSeconds       int
	rememberDecisions    bool
	sensitiveProtection  bool
	dataCollection       bool
	telemetry            bool
	clearOnExit          bool
	fileDiscovery        bool
	projectContext       bool
	workspaceIsolation   bool
	providerStatus       string
	rateLimitStatus      string

	width, height int
	regions       []clickRegion
	turnRows      map[int]int
	hoverTurn     int
	quitting      bool
}

var menuItems = []string{"New space", "Login (providers)", "Changelog", "Exit"}
var formFields = []string{"provider id", "base url", "model", "api key", "default? y/n"}

// NewModel builds the TUI state for cwd.
func NewModel(cwd string) *Model {
	m := &Model{
		cwd:       cwd,
		th:        resolveTheme(cwd),
		view:      viewStartup,
		form:      formFields,
		tab:       tabChat,
		turnRows:  map[int]int{},
		hoverTurn: -1,
	}
	m.st = stylesFor(m.th)
	m.refreshHz = resolveRefreshHz(cwd)
	m.loadPreferences()
	eng, err := NewEngine(cwd)
	if err != nil {
		m.engErr = err.Error()
	} else {
		m.eng = eng
		m.applyPreferencesToEngine()
	}
	m.prov = LoadProvRows(cwd)
	m.chlog = loadChangelog(cwd)
	return m
}

// loadPreferences reads the persisted interface preferences.
func (m *Model) loadPreferences() {
	st := LoadState()
	m.animations = boolVal(st.Animations, true)
	m.fontScale = st.FontScale
	if m.fontScale == 0 {
		m.fontScale = 100
	}
	m.density = st.Density
	if m.density == "" {
		m.density = "comfortable"
	}
	m.statusIndicators = boolVal(st.StatusIndicators, true)
	m.activityTracking = boolVal(st.ActivityTracking, true)
	m.autoContinue = boolVal(st.AutoContinue, true)
	m.executionTransparent = boolVal(st.ExecutionTransparent, true)
	m.planFirst = boolVal(st.PlanFirst, false)
	m.notificationsOn = boolVal(st.NotificationsOn, true)
	m.noteTTLSeconds = st.NoteTTLSeconds
	if m.noteTTLSeconds == 0 {
		m.noteTTLSeconds = 10
	}
	m.rememberDecisions = boolVal(st.RememberDecisions, true)
	m.sensitiveProtection = boolVal(st.SensitiveProtection, true)
	m.dataCollection = boolVal(st.DataCollection, false)
	m.telemetry = boolVal(st.Telemetry, false)
	m.clearOnExit = boolVal(st.ClearOnExit, false)
	m.fileDiscovery = boolVal(st.FileDiscovery, true)
	m.projectContext = boolVal(st.ProjectContext, true)
	m.workspaceIsolation = boolVal(st.WorkspaceIsolation, true)
	m.providerStatus = "not checked"
	m.rateLimitStatus = "no limit reported"
}

// applyPreferencesToEngine pushes the preferences into the running agent.
func (m *Model) applyPreferencesToEngine() {
	if m.eng == nil {
		return
	}
	m.eng.SetAutoContinue(m.autoContinue)
	m.eng.SetPlanFirst(m.planFirst)
	m.eng.SetProjectContext(m.projectContext)
	m.eng.SetSensitiveProtection(m.sensitiveProtection)
	m.eng.SetFileDiscovery(m.fileDiscovery)
}

// saveState persists the interface preferences.
func (m *Model) saveState() error {
	st := LoadState()
	st.Theme = m.th.Name
	st.RefreshHz = m.refreshHz
	st.Animations = boolPtr(m.animations)
	st.FontScale = m.fontScale
	st.Density = m.density
	st.StatusIndicators = boolPtr(m.statusIndicators)
	st.ActivityTracking = boolPtr(m.activityTracking)
	st.AutoContinue = boolPtr(m.autoContinue)
	st.ExecutionTransparent = boolPtr(m.executionTransparent)
	st.PlanFirst = boolPtr(m.planFirst)
	st.NotificationsOn = boolPtr(m.notificationsOn)
	st.NoteTTLSeconds = m.noteTTLSeconds
	st.RememberDecisions = boolPtr(m.rememberDecisions)
	st.SensitiveProtection = boolPtr(m.sensitiveProtection)
	st.DataCollection = boolPtr(m.dataCollection)
	st.Telemetry = boolPtr(m.telemetry)
	st.ClearOnExit = boolPtr(m.clearOnExit)
	st.FileDiscovery = boolPtr(m.fileDiscovery)
	st.ProjectContext = boolPtr(m.projectContext)
	st.WorkspaceIsolation = boolPtr(m.workspaceIsolation)
	return SaveState(st)
}

// resolveTheme picks the theme: the saved choice first, then the "tui.theme"
// setting of any O config file, then dark.
func resolveTheme(cwd string) Theme {
	st := LoadState()
	if st.Theme != "" {
		if st.Theme == "light" {
			return LightTheme()
		}
		return DarkTheme()
	}
	if cfg, err := config.Load(cwd); err == nil && cfg.TUI.Theme == "light" {
		return LightTheme()
	}
	return DarkTheme()
}

// resolveRefreshHz picks the refresh rate: the saved choice first, then the
// "tui.refreshHz" setting of any O config file, then 60.
func resolveRefreshHz(cwd string) int {
	if hz := LoadRefreshHz(); hz > 0 {
		return hz
	}
	if cfg, err := config.Load(cwd); err == nil && cfg.TUI.RefreshHz > 0 {
		return cfg.TUI.RefreshHz
	}
	return 60
}

// SetTheme switches the palette and persists the choice.
func (m *Model) SetTheme(name string) {
	if name == "light" {
		m.th = LightTheme()
	} else {
		m.th = DarkTheme()
	}
	m.st = stylesFor(m.th)
	_ = SaveTheme(name)
}

// Init starts the engine-event pump and the spinner ticker.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.waitEvent(), m.tickCmd())
}

func (m *Model) waitEvent() tea.Cmd {
	return func() tea.Msg {
		if m.eng == nil {
			return nil
		}
		ev, ok := <-m.eng.Events()
		if !ok {
			return nil
		}
		return agentMsg{ev: ev}
	}
}

// tickCmd schedules the next spinner frame at the selected refresh rate.
func (m *Model) tickCmd() tea.Cmd {
	hz := m.refreshHz
	if hz <= 0 {
		hz = 60
	}
	d := time.Second / time.Duration(hz)
	if d < time.Millisecond {
		d = time.Millisecond
	}
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update handles every incoming message: keys, mouse, window size, engine
// events and the spinner tick.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.expireNotifications()
		if m.running {
			m.spin++
			return m, m.tickCmd()
		}
		return m, nil

	case agentMsg:
		m.onEvent(msg.ev)
		return m, m.waitEvent()

	case tea.KeyMsg:
		return m.onKey(msg)

	case tea.MouseMsg:
		return m.onMouse(msg)
	}
	return m, nil
}

// ---- keyboard ----

func (m *Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	debugf("key type=%v string=%q runes=%q", msg.Type, msg.String(), string(msg.Runes))
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.view == viewStartup {
			return m, tea.Quit
		}
		if m.perm != nil {
			m.answerPermission(false, false)
			return m, nil
		}
		if m.running && m.eng != nil {
			m.eng.Abort()
			m.status = "stopped"
			m.statusErr = false
			m.running = false
			m.activity = ""
			m.closeRunningStep()
			m.notify("warn", "Run stopped", "the agent was interrupted")
		}
		return m, nil
	case tea.KeyEsc:
		if m.view == viewStartup {
			return m, tea.Quit
		}
		if m.perm != nil {
			m.answerPermission(false, false)
			return m, nil
		}
		if m.palette {
			m.palette = false
			m.paletteRows = nil
			return m, nil
		}
		if m.info != nil {
			m.info = nil
			return m, nil
		}
		if m.tab == tabSettings {
			m.tab = tabChat
			return m, nil
		}
		if m.scroll > 0 {
			m.scroll = 0
		} else if m.tab != tabChat {
			m.tab = tabChat
		}
		return m, nil
	}

	if m.view == viewStartup {
		return m, m.startupKey(msg)
	}
	// the permission dialog owns the keyboard while it is open
	if m.perm != nil {
		return m.permissionKey(msg)
	}
	if m.tab == tabSettings {
		return m.settingsKey(msg)
	}
	if m.palette {
		if cmd, handled := m.paletteKey(msg); handled {
			return m, cmd
		}
	}
	m.composerKey(msg)
	return m, nil
}

func (m *Model) startupKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up":
		m.menu = (m.menu + len(menuItems) - 1) % len(menuItems)
	case "down":
		m.menu = (m.menu + 1) % len(menuItems)
	case "enter":
		return m.activateMenu(m.menu)
	}
	return nil
}

func (m *Model) activateMenu(i int) tea.Cmd {
	switch i {
	case 0:
		m.view = viewMain
		m.tab = tabChat
	case 1:
		m.view = viewMain
		m.tab = tabProviders
		m.prov = LoadProvRows(m.cwd)
	case 2:
		m.view = viewMain
		m.tab = tabChangelog
		m.chlog = loadChangelog(m.cwd)
	case 3:
		m.quitting = true
		return tea.Quit
	}
	return nil
}

// composerKey handles the chat input.
func (m *Model) composerKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyEnter:
		m.submit()
		return
	case tea.KeyBackspace:
		if m.cursor > 0 && len(m.input) > 0 {
			m.input = append(m.input[:m.cursor-1], m.input[m.cursor:]...)
			m.cursor--
			m.syncPalette()
		}
		return
	case tea.KeyDelete:
		if m.cursor < len(m.input) {
			m.input = append(m.input[:m.cursor], m.input[m.cursor+1:]...)
			m.syncPalette()
		}
		return
	case tea.KeyLeft:
		if m.cursor > 0 {
			m.cursor--
		}
		return
	case tea.KeyRight:
		if m.cursor < len(m.input) {
			m.cursor++
		}
		return
	case tea.KeyHome:
		m.cursor = 0
		return
	case tea.KeyEnd:
		m.cursor = len(m.input)
		return
	case tea.KeyUp:
		m.scrollBy(1)
		return
	case tea.KeyDown:
		m.scrollBy(-1)
		return
	case tea.KeyPgUp:
		m.scrollBy(10)
		return
	case tea.KeyPgDown:
		m.scrollBy(-10)
		return
	case tea.KeyCtrlL:
		m.turns = nil
		m.cur = nil
		m.scroll = 0
		return
	case tea.KeyCtrlU:
		m.input = nil
		m.cursor = 0
		m.syncPalette()
		return
	case tea.KeyCtrlO:
		// open or close the workflow object of the current turn
		if m.cur != nil && len(m.cur.steps) > 0 {
			m.toggleStep(m.cur.steps[len(m.cur.steps)-1].id)
		}
		return
	case tea.KeyRunes:
		m.info = nil
		for _, r := range msg.Runes {
			if r >= 0x20 {
				m.input = append(m.input, 0)
				copy(m.input[m.cursor+1:], m.input[m.cursor:])
				m.input[m.cursor] = r
				m.cursor++
			}
		}
		m.syncPalette()
	}
}

// paletteKey handles the command palette: ↑↓ choose, enter run, esc close.
func (m *Model) paletteKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyUp:
		if len(m.paletteRows) > 0 {
			m.paletteSel = (m.paletteSel - 1 + len(m.paletteRows)) % len(m.paletteRows)
		}
		return nil, true
	case tea.KeyDown:
		if len(m.paletteRows) > 0 {
			m.paletteSel = (m.paletteSel + 1) % len(m.paletteRows)
		}
		return nil, true
	case tea.KeyEnter:
		m.runPaletteSelection()
		return nil, true
	}
	return nil, false
}

// syncPalette opens the palette as soon as the input starts with a slash and
// keeps the result list in sync with the typed query.
func (m *Model) syncPalette() {
	s := string(m.input)
	if strings.HasPrefix(s, "/") && !strings.Contains(s, " ") {
		cmds := filterCommands(s)
		m.paletteRows = m.paletteRows[:0]
		for _, c := range cmds {
			m.paletteRows = append(m.paletteRows, paletteRow{cmd: c})
		}
		m.palette = true
		if m.paletteSel >= len(m.paletteRows) {
			m.paletteSel = 0
		}
		return
	}
	m.palette = false
	m.paletteRows = nil
	m.paletteSel = 0
}

// runPaletteSelection runs the command the user selected in the palette.
func (m *Model) runPaletteSelection() {
	text := strings.TrimSpace(string(m.input))
	parts := strings.Fields(text)
	name, arg := "", ""
	if len(parts) > 0 {
		name = parts[0]
		arg = strings.TrimSpace(strings.TrimPrefix(text, name))
	}
	cmd, ok := lookupCommand(name)
	if !ok {
		if len(m.paletteRows) > 0 {
			cmd = m.paletteRows[m.paletteSel].cmd
			arg = ""
		} else {
			m.fail("unknown command " + name)
			m.clearInput()
			return
		}
	}
	m.clearInput()
	m.palette = false
	m.paletteRows = nil
	m.runCommand(cmd, arg)
}

// runCommand executes one command from the registry.
func (m *Model) runCommand(cmd command, arg string) {
	m.info = nil
	if cmd.run == nil {
		return
	}
	_ = cmd.run(m, arg)
}

// clearInput empties the composer.
func (m *Model) clearInput() {
	m.input = nil
	m.cursor = 0
	m.palette = false
	m.paletteRows = nil
}

// ---- providers ----

func (m *Model) formKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyEnter:
		m.formIdx++
		if m.formIdx >= len(m.form) {
			m.submitForm()
		}
	case tea.KeyBackspace:
		if len(m.formVals) > m.formIdx && len(m.formVals[m.formIdx]) > 0 {
			r := []rune(m.formVals[m.formIdx])
			m.formVals[m.formIdx] = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		m.formVals[m.formIdx] += string(msg.Runes)
	}
}

func (m *Model) submitForm() {
	m.provMode = 0
	m.formIdx = 0
	v := m.formVals
	for len(v) < len(m.form) {
		v = append(v, "")
	}
	id, base, model, key, def := v[0], v[1], v[2], v[3], strings.ToLower(v[4])
	if id == "" || base == "" {
		m.status = "id and base url are required"
		m.statusErr = true
		m.formVals = make([]string, len(m.form))
		return
	}
	path, err := config.AddProvider(id, config.ProviderConfig{BaseURL: base, Model: model, APIKey: key}, def == "y" || def == "yes")
	if err != nil {
		m.fail(err.Error())
	} else {
		m.status = fmt.Sprintf("provider %q saved", id)
		m.statusErr = false
		m.notify("success", "Provider saved", id+" → "+path)
	}
	m.formVals = make([]string, len(m.form))
	m.prov = LoadProvRows(m.cwd)
}

func (m *Model) providerKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "t":
		if m.provSel < len(m.prov) {
			m.status = "testing " + m.prov[m.provSel].ID + "…"
			m.statusErr = false
			res := TestProvider(m.cwd, m.prov[m.provSel].ID)
			m.status = m.prov[m.provSel].ID + ": " + res
			m.statusErr = !strings.HasPrefix(res, "HTTP 200")
		}
		return true
	case "a":
		m.provMode = 1
		m.formVals = make([]string, len(m.form))
		m.formIdx = 0
		return true
	case "r":
		if m.provSel < len(m.prov) {
			id := m.prov[m.provSel].ID
			if _, err := config.RemoveProvider(id); err != nil {
				m.fail(err.Error())
			} else {
				m.status = "removed " + id
				m.notify("info", "Provider removed", id)
			}
			m.prov = LoadProvRows(m.cwd)
			if m.provSel > len(m.prov)-1 && m.provSel > 0 {
				m.provSel--
			}
		}
		return true
	case "d":
		if m.provSel < len(m.prov) {
			id := m.prov[m.provSel].ID
			if _, err := config.SetDefaultProvider(id); err != nil {
				m.fail(err.Error())
			} else {
				m.status = "default provider = " + id
				m.notify("info", "Default provider changed", id)
			}
			m.prov = LoadProvRows(m.cwd)
		}
		return true
	}
	return false
}

func (m *Model) scrollBy(d int) {
	switch m.tab {
	case tabProviders:
		if d > 0 && m.provSel > 0 {
			m.provSel--
		}
		if d < 0 && m.provSel < len(m.prov)-1 {
			m.provSel++
		}
	case tabChangelog:
		m.chScroll += d
		if m.chScroll < 0 {
			m.chScroll = 0
		}
	default:
		m.scroll += d
		if m.scroll < 0 {
			m.scroll = 0
		}
	}
}

// ---- settings workspace ----

// settingsKey handles the settings workspace: the search field has the
// focus, ↑↓ chooses a setting, ←→ changes its value, enter runs an action.
func (m *Model) settingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.visibleSettings()
	switch msg.Type {
	case tea.KeyUp:
		if len(rows) > 0 {
			m.settings.sel = (m.settings.sel - 1 + len(rows)) % len(rows)
		}
		return m, nil
	case tea.KeyDown:
		if len(rows) > 0 {
			m.settings.sel = (m.settings.sel + 1) % len(rows)
		}
		return m, nil
	case tea.KeyLeft:
		if len(rows) > 0 && m.settings.sel < len(rows) {
			m.cycleSetting(rows[m.settings.sel], -1)
		}
		return m, nil
	case tea.KeyRight, tea.KeyEnter:
		if len(rows) == 0 || m.settings.sel >= len(rows) {
			return m, nil
		}
		s := rows[m.settings.sel]
		switch s.kind {
		case kindAction:
			if s.action != nil {
				return m, s.action(m)
			}
		case kindInfo:
			// nothing to change
		default:
			m.cycleSetting(s, 1)
		}
		return m, nil
	case tea.KeyBackspace:
		if len(m.settings.query) > 0 {
			r := []rune(m.settings.query)
			m.settings.query = string(r[:len(r)-1])
			m.settings.sel = 0
		}
		return m, nil
	case tea.KeyRunes:
		m.settings.query += string(msg.Runes)
		m.settings.sel = 0
		return m, nil
	}
	return m, nil
}

// ---- mouse ----

// onMouse handles wheel scrolling, hover and click activation. Bubble Tea
// gives cell coordinates, so clicks are hit-tested against the regions
// rendered in the last frame.
func (m *Model) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	debugf("mouse action=%v button=%v x=%d y=%d", msg.Action, msg.Button, msg.X, msg.Y)
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.scrollBy(1)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.scrollBy(-1)
		return m, nil
	}
	// Bubble Tea reports zero-based cells; the regions are one-based.
	mx, my := msg.X+1, msg.Y+1
	if msg.Action == tea.MouseActionMotion {
		m.hoverTurn = -1
		for _, r := range m.regions {
			if r.kind == "time" && my >= r.y0 && my <= r.y1 && mx >= r.x0 && mx <= r.x1 {
				m.hoverTurn = r.id
			}
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft && msg.Button != tea.MouseButtonRight {
		return m, nil
	}
	for _, r := range m.regions {
		if my < r.y0 || my > r.y1 || mx < r.x0 || mx > r.x1 {
			continue
		}
		switch r.kind {
		case "menu":
			m.menu = r.index
			return m, m.activateMenu(r.index)
		case "provider":
			m.provSel = r.index
			m.status = "selected " + m.prov[r.index].ID
		case "setting":
			rows := m.visibleSettings()
			if r.index < len(rows) {
				s := rows[r.index]
				switch s.kind {
				case kindAction:
					if s.action != nil {
						return m, s.action(m)
					}
				default:
					m.setSetting(s, s.options[r.value])
				}
			}
		case "step":
			m.toggleStep(r.id)
		case "stop":
			if m.running && m.eng != nil {
				m.eng.Abort()
				m.status = "stopped"
				m.statusErr = false
				m.running = false
				m.activity = ""
				m.closeRunningStep()
				m.notify("warn", "Run stopped", "the agent was interrupted")
			}
		case "notice":
			m.dismissNotification(r.id)
		}
		return m, nil
	}
	return m, nil
}

// ---- submitting ----

func (m *Model) submit() {
	text := strings.TrimSpace(string(m.input))
	m.clearInput()
	if text == "" {
		return
	}
	if strings.HasPrefix(text, "/") {
		parts := strings.Fields(text)
		name := parts[0]
		arg := strings.TrimSpace(strings.TrimPrefix(text, name))
		if cmd, ok := lookupCommand(name); ok {
			m.runCommand(cmd, arg)
			return
		}
		m.fail("unknown command " + name + " — type / to see the list")
		return
	}
	if m.eng == nil {
		m.fail(m.engErr)
		return
	}
	if !m.eng.Prompt(text) {
		m.fail("a run is already in progress (ctrl+c to stop it)")
		return
	}
	m.hist = append(m.hist, text)
	m.newTurn(text)
	m.running = true
	m.started = time.Now()
	m.activity = "Acknowledging task"
	m.scroll = 0
	m.status = ""
	m.statusErr = false
}

// ---- helpers ----

// fail shows an error explanation in the status area.
func (m *Model) fail(msg string) {
	fe := classifyError(msg)
	m.status = fe.Title
	m.statusErr = true
}

// permissionMode returns the permission mode of the engine.
func (m *Model) permissionMode() string {
	if m.eng == nil {
		return defaultMode
	}
	return m.eng.Mode()
}

// sessionDir returns the folder with the saved conversations.
func (m *Model) sessionDir() string {
	if m.eng != nil {
		return m.eng.sessDir
	}
	cfg, err := config.Load(m.cwd)
	if err != nil {
		return "~/.o/sessions"
	}
	return cfg.Paths.SessionsDir
}

// rememberSession notes which workspace a session belongs to.
func (m *Model) rememberSession(id string) {
	if id == "" {
		return
	}
	st := LoadState()
	if st.Sessions == nil {
		st.Sessions = map[string]string{}
	}
	st.Sessions[id] = m.workspacePath()
	_ = SaveState(st)
}

// testProvider performs a real connectivity check against the active
// provider and reports it in plain language.
func (m *Model) testProvider() tea.Cmd {
	if m.eng == nil {
		m.fail(m.engErr)
		return nil
	}
	id := m.providerLabel()
	res := TestProvider(m.cwd, id)
	switch {
	case strings.HasPrefix(res, "HTTP 200"):
		m.providerStatus = "connected"
		m.status = id + ": connected"
		m.statusErr = false
		m.notify("success", "Provider connected", id)
	case strings.Contains(res, "HTTP 429"):
		m.rateLimitStatus = "limit reached — wait before the next request"
		m.providerStatus = "rate limited"
		m.status = id + ": rate limited"
		m.statusErr = true
		m.notify("warn", "Provider rate limited", id)
	case strings.Contains(res, "HTTP 401"), strings.Contains(res, "HTTP 403"):
		m.providerStatus = "authentication failed"
		m.status = id + ": the API key was rejected"
		m.statusErr = true
		m.notify("error", "Provider authentication failed", id)
	default:
		m.providerStatus = "unreachable"
		m.status = id + ": unreachable"
		m.statusErr = true
		m.notify("error", "Provider unreachable", id)
	}
	return nil
}

// changeWorkspace moves the agent to another project folder.
func (m *Model) changeWorkspace(path string) error {
	if m.eng == nil {
		return fmt.Errorf("no agent is running")
	}
	if err := m.eng.SetWorkspace(path); err != nil {
		return err
	}
	m.cwd = m.eng.Workspace()
	m.notify("info", "Workspace changed", m.cwd)
	return nil
}

// Clear removes the stored conversations of the current workspace.
func (m *Model) clearStoredSessions() error {
	dir := session.ExpandDir(m.sessionDir())
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if err := os.Remove(dir + string(os.PathSeparator) + e.Name()); err == nil {
			removed++
		}
	}
	m.notify("info", "Stored conversations cleared", fmt.Sprintf("%d removed", removed))
	return nil
}
