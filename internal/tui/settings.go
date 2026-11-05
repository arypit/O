package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// settingKind describes how a setting is changed.
type settingKind int

const (
	kindToggle settingKind = iota
	kindChoice
	kindAction
	kindInfo
)

// settingDef is one entry of the settings workspace.
type settingDef struct {
	key      string
	title    string
	category string
	kind     settingKind
	options  []string
	desc     string
	value    func(m *Model) string
	set      func(m *Model, value string) error
	action   func(m *Model) tea.Cmd
}

// settingsState is the state of the settings workspace.
type settingsState struct {
	query    string
	sel      int
	scroll   int
	prompt   string
	promptFor string
}

// visibleSettings returns the rows shown in the settings workspace:
// categories in alphabetical order, settings alphabetical inside each
// category, filtered by the search query.
func (m *Model) visibleSettings() []settingDef {
	q := strings.ToLower(strings.TrimSpace(m.settings.query))
	all := allSettings()
	byCat := map[string][]settingDef{}
	for _, s := range all {
		if q != "" && !settingMatches(s, q, m) {
			continue
		}
		byCat[s.category] = append(byCat[s.category], s)
	}
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	var out []settingDef
	for _, c := range cats {
		rows := byCat[c]
		sort.Slice(rows, func(i, j int) bool { return rows[i].title < rows[j].title })
		out = append(out, rows...)
	}
	return out
}

// settingMatches reports whether a setting matches the search query.
func settingMatches(s settingDef, q string, m *Model) bool {
	if strings.Contains(strings.ToLower(s.title), q) ||
		strings.Contains(strings.ToLower(s.category), q) ||
		strings.Contains(strings.ToLower(s.desc), q) {
		return true
	}
	if s.value != nil && strings.Contains(strings.ToLower(s.value(m)), q) {
		return true
	}
	return false
}

// settingValue returns the current value of a setting.
func (m *Model) settingValue(s settingDef) string {
	if s.value == nil {
		return ""
	}
	return s.value(m)
}

// setSetting applies a value to a setting.
func (m *Model) setSetting(s settingDef, value string) {
	if s.set == nil {
		return
	}
	if err := s.set(m, value); err != nil {
		m.fail(err.Error())
		return
	}
	m.status = s.title + ": " + value
	m.statusErr = false
}

// cycleSetting moves a setting to its next value.
func (m *Model) cycleSetting(s settingDef, dir int) {
	if s.kind == kindToggle {
		cur := m.settingValue(s)
		next := "off"
		if cur == "on" {
			next = "on"
		}
		m.setSetting(s, next)
		return
	}
	if len(s.options) == 0 {
		return
	}
	cur := m.settingValue(s)
	idx := 0
	for i, o := range s.options {
		if o == cur {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(s.options)) % len(s.options)
	m.setSetting(s, s.options[idx])
}

// allSettings is the complete settings registry.
func allSettings() []settingDef {
	return []settingDef{
		// ---- Appearance ----
		{
			key: "appearance.animations", title: "Animations", category: "Appearance", kind: kindToggle,
			desc: "Controls whether transitions, state changes, notifications, command menus and loading indicators use animated behaviour or are drawn immediately. Turning animations off replaces the moving activity spinner with a static marker and removes the blinking cursor effect; the interface stays responsive either way.",
			value: func(m *Model) string { return onOff(m.animations) },
			set: func(m *Model, v string) error {
				m.animations = v == "on"
				return m.saveState()
			},
		},
		{
			key: "appearance.fontsize", title: "Font Size", category: "Appearance", kind: kindChoice,
			options: []string{"Terminal default", "Small (90%)", "Large (110%)"},
			desc: "Scales the interface content inside the terminal window. The terminal font itself is owned by your terminal application, so O changes the width and spacing of everything it draws instead: Small fits more text on one line, Large gives every element more room. The layout keeps its proportions at every scale.",
			value: func(m *Model) string {
				switch m.fontScale {
				case 90:
					return "Small (90%)"
				case 110:
					return "Large (110%)"
				default:
					return "Terminal default"
				}
			},
			set: func(m *Model, v string) error {
				switch v {
				case "Small (90%)":
					m.fontScale = 90
				case "Large (110%)":
					m.fontScale = 110
				default:
					m.fontScale = 100
				}
				return m.saveState()
			},
		},
		{
			key: "appearance.density", title: "Interface Density", category: "Appearance", kind: kindChoice,
			options: []string{"Compact", "Comfortable", "Spacious"},
			desc: "Controls how compact or spacious interface elements appear. It changes the spacing between components, list entries, commands, activity objects, messages and configuration items. Compact fits the most content on screen, Spacious gives every block room to breathe.",
			value: func(m *Model) string {
				switch m.density {
				case "compact":
					return "Compact"
				case "spacious":
					return "Spacious"
				default:
					return "Comfortable"
				}
			},
			set: func(m *Model, v string) error {
				switch v {
				case "Compact":
					m.density = "compact"
				case "Spacious":
					m.density = "spacious"
				default:
					m.density = "comfortable"
				}
				return m.saveState()
			},
		},
		{
			key: "appearance.status", title: "Status Indicators", category: "Appearance", kind: kindToggle,
			desc: "Controls the visibility of activity states, execution progress indicators, background operation markers, provider status indicators and execution badges. With the indicators off, the interface shows only the conversation and the input area; the agent keeps working in the background.",
			value: func(m *Model) string { return onOff(m.statusIndicators) },
			set: func(m *Model, v string) error {
				m.statusIndicators = v == "on"
				return m.saveState()
			},
		},
		{
			key: "appearance.theme", title: "Theme", category: "Appearance", kind: kindChoice,
			options: []string{"Dark", "Light"},
			desc: "Dark applies darker interface colours optimised for low-light environments and long development sessions. Light applies brighter interface colours optimised for well-lit environments and improved content readability. The choice is remembered for the next start.",
			value: func(m *Model) string {
				if m.th.Name == "light" {
					return "Light"
				}
				return "Dark"
			},
			set: func(m *Model, v string) error {
				if v == "Light" {
					m.SetTheme("light")
				} else {
					m.SetTheme("dark")
				}
				m.notify("info", "Theme: "+v, "")
				return nil
			},
		},

		// ---- Agent ----
		{
			key: "agent.activity", title: "Activity Tracking", category: "Agent", kind: kindToggle,
			desc: "Controls live execution state reporting. When enabled, the agent continuously shows what it is actively doing — Thinking, Searching, Reading Files, Coding, Debugging, Testing, Reviewing, Analysing, Formatting, Refactoring, Validating, Planning, Investigating, Fixing, Opening Files, Creating Files, Writing Code or Summarising. The displayed activity always reflects the operation that is really running.",
			value: func(m *Model) string { return onOff(m.activityTracking) },
			set: func(m *Model, v string) error {
				m.activityTracking = v == "on"
				return m.saveState()
			},
		},
		{
			key: "agent.autocontinue", title: "Automatic Task Continuation", category: "Agent", kind: kindToggle,
			desc: "Controls whether the agent may continue through multi-step workflows on its own. When it is on, the agent keeps working through related steps until the task is finished. When it is off, the agent stops after each step and waits for you to continue it from the input line.",
			value: func(m *Model) string { return onOff(m.autoContinue) },
			set: func(m *Model, v string) error {
				m.autoContinue = v == "on"
				if m.eng != nil {
					m.eng.SetAutoContinue(m.autoContinue)
				}
				return m.saveState()
			},
		},
		{
			key: "agent.transparency", title: "Execution Transparency", category: "Agent", kind: kindToggle,
			desc: "Controls the visibility of action summaries, reasoning summaries, execution progress information, tool usage summaries and operation stages. With transparency on you can inspect every stage the agent went through; with it off only the final answer and errors are shown.",
			value: func(m *Model) string { return onOff(m.executionTransparent) },
			set: func(m *Model, v string) error {
				m.executionTransparent = v == "on"
				return m.saveState()
			},
		},
		{
			key: "agent.planmode", title: "Plan Mode", category: "Agent", kind: kindToggle,
			desc: "Controls whether the agent first constructs an execution plan before beginning work. When it is on, the agent starts every task with a written plan and works through it step by step; when it is off, the agent goes straight to the shortest path.",
			value: func(m *Model) string { return onOff(m.planFirst) },
			set: func(m *Model, v string) error {
				m.planFirst = v == "on"
				if m.eng != nil {
					m.eng.SetPlanFirst(m.planFirst)
				}
				return m.saveState()
			},
		},

		// ---- Models ----
		{
			key: "models.model", title: "Model", category: "Models", kind: kindChoice,
			desc: "Determines the active intelligence provider used for all interactions throughout the application. The change applies to conversation generation, reasoning, tool usage, planning, analysis, coding, execution guidance, summaries and all agent decision-making. Newly created sessions immediately use the currently configured model, and existing conversations stay compatible.",
			value: func(m *Model) string { return m.modelName() },
			set: func(m *Model, v string) error {
				if m.eng == nil {
					return fmt.Errorf("no provider is configured")
				}
				if err := m.eng.SetModel(v); err != nil {
					return err
				}
				m.notify("info", "Model: "+v, "")
				return nil
			},
		},

		// ---- Notifications ----
		{
			key: "notifications.enabled", title: "Notifications", category: "Notifications", kind: kindToggle,
			desc: "Controls the notification layer. Notifications appear only for important events — full access enabled or disabled, permission granted or denied, workspace changed, provider connected or disconnected, critical execution failure, task completed, session restored, configuration updated and security-sensitive actions approved. Routine thinking, file reading and command execution never create a notification.",
			value: func(m *Model) string { return onOff(m.notificationsOn) },
			set: func(m *Model, v string) error {
				m.notificationsOn = v == "on"
				if !m.notificationsOn {
					m.notes = nil
				}
				return m.saveState()
			},
		},
		{
			key: "notifications.duration", title: "Notification Duration", category: "Notifications", kind: kindChoice,
			options: []string{"5 seconds", "10 seconds", "20 seconds", "Until dismissed"},
			desc: "How long a notification stays in the notification area before it disappears. Notifications are always kept in the notification history, which you can open with /notifications, so nothing is lost when a notification expires.",
			value: func(m *Model) string {
				switch m.noteTTLSeconds {
				case 5:
					return "5 seconds"
				case 20:
					return "20 seconds"
				case 0:
					return "Until dismissed"
				default:
					return "10 seconds"
				}
			},
			set: func(m *Model, v string) error {
				switch v {
				case "5 seconds":
					m.noteTTLSeconds = 5
				case "20 seconds":
					m.noteTTLSeconds = 20
				case "Until dismissed":
					m.noteTTLSeconds = 0
				default:
					m.noteTTLSeconds = 10
				}
				return m.saveState()
			},
		},

		// ---- Permissions ----
		{
			key: "permissions.mode", title: "Permission Mode", category: "Permissions", kind: kindChoice,
			options: []string{"Default", "Auto-edit", "Plan", "Full auto"},
			desc: "Determines the approval strategy for all tool execution. Default asks before every action that changes something. Auto-edit allows file edits and asks for shell commands. Plan allows read-only work and refuses changes. Full auto lets every action run without asking. Approval requests always appear in the dedicated permission interface, never in the conversation.",
			value: func(m *Model) string {
				switch m.permissionMode() {
				case "auto-edit":
					return "Auto-edit"
				case "plan":
					return "Plan"
				case "full-auto":
					return "Full auto"
				default:
					return "Default"
				}
			},
			set: func(m *Model, v string) error {
				mode := "default"
				switch v {
				case "Auto-edit":
					mode = "auto-edit"
				case "Plan":
					mode = "plan"
				case "Full auto":
					mode = "full-auto"
				}
				m.setMode(mode)
				if mode == "full-auto" {
					m.notify("warn", "Full access ON", "all tool actions auto-run")
				} else {
					m.notify("info", "Permission mode: "+mode, "")
				}
				return nil
			},
		},
		{
			key: "permissions.remember", title: "Remember Decisions", category: "Permissions", kind: kindToggle,
			desc: "Controls whether previously approved actions remain approved throughout the current session. With it on, choosing “always allow” keeps that tool approved until O closes; with it off, every action asks again and the stored approvals are cleared.",
			value: func(m *Model) string { return onOff(m.rememberDecisions) },
			set: func(m *Model, v string) error {
				m.rememberDecisions = v == "on"
				if !m.rememberDecisions && m.eng != nil {
					m.eng.ClearAlways()
				}
				return m.saveState()
			},
		},
		{
			key: "permissions.sensitive", title: "Sensitive Operations Protection", category: "Permissions", kind: kindToggle,
			desc: "Controls the additional confirmation required for actions that affect files, system settings, network access, configuration changes or potentially destructive operations. With it on, destructive commands always ask for approval, even while full access is active.",
			value: func(m *Model) string { return onOff(m.sensitiveProtection) },
			set: func(m *Model, v string) error {
				m.sensitiveProtection = v == "on"
				if m.eng != nil {
					m.eng.SetSensitiveProtection(m.sensitiveProtection)
				}
				return m.saveState()
			},
		},
		{
			key: "permissions.scope", title: "Session Access Level", category: "Permissions", kind: kindInfo,
			desc: "Shows the access scope that is active for this session. The workspace sandbox lets the agent read and write inside the project folder and run commands there; everything outside that folder stays out of reach. Change the folder itself with Workspace → Default Workspace.",
			value: func(m *Model) string { return m.accessLevel() },
		},

		// ---- Privacy ----
		{
			key: "privacy.collection", title: "Data Collection", category: "Privacy", kind: kindToggle,
			desc: "Controls whether anonymous usage information may be collected for application improvement. O does not collect usage data and sends nothing about your code anywhere except to the provider you configured. This switch records your preference so the behaviour is explicit.",
			value: func(m *Model) string { return onOff(m.dataCollection) },
			set: func(m *Model, v string) error {
				m.dataCollection = v == "on"
				return m.saveState()
			},
		},
		{
			key: "privacy.storage", title: "Session Storage", category: "Privacy", kind: kindChoice,
			options: []string{"Keep history", "Clear on exit"},
			desc: "Controls the retention of conversation history and local metadata. Conversations are stored as plain JSONL files in your local sessions folder so they can be resumed. “Clear on exit” removes the session file when O closes; the history stays on your machine in either case.",
			value: func(m *Model) string {
				if m.clearOnExit {
					return "Clear on exit"
				}
				return "Keep history"
			},
			set: func(m *Model, v string) error {
				m.clearOnExit = v == "Clear on exit"
				return m.saveState()
			},
		},
		{
			key: "privacy.telemetry", title: "Telemetry", category: "Privacy", kind: kindToggle,
			desc: "Controls operational diagnostics and performance reporting. O never sends telemetry: diagnostics stay in a local log file on your machine and are only written when you enable the debug flag yourself. This switch records your preference and keeps every measurement local.",
			value: func(m *Model) string { return onOff(m.telemetry) },
			set: func(m *Model, v string) error {
				m.telemetry = v == "on"
				return m.saveState()
			},
		},

		// ---- Provider ----
		{
			key: "provider.api", title: "API Configuration", category: "Provider", kind: kindAction,
			desc: "Manages authentication information and provider connection details: the provider name, its endpoint, the model it serves and the API key it uses. Keys are masked on screen and stored in your local O configuration file.",
			value: func(m *Model) string { return m.providerLabel() },
			action: func(m *Model) tea.Cmd {
				m.tab = tabProviders
				m.prov = LoadProvRows(m.cwd)
				return nil
			},
		},
		{
			key: "provider.connection", title: "Connection Status", category: "Provider", kind: kindAction,
			desc: "Displays the current availability and health of the active provider. The check is a real request to the provider's models endpoint, so the status reflects the connection rather than a cached guess.",
			value: func(m *Model) string { return m.providerStatus },
			action: func(m *Model) tea.Cmd {
				return m.testProvider()
			},
		},
		{
			key: "provider.ratelimit", title: "Rate Limit Status", category: "Provider", kind: kindInfo,
			desc: "Displays provider availability constraints and current usage state. Most providers only report a limit when it is reached, so this shows the last known state and is updated whenever the provider answers with a rate-limit response.",
			value: func(m *Model) string { return m.rateLimitStatus },
		},
		{
			key: "provider.validation", title: "Provider Validation", category: "Provider", kind: kindAction,
			desc: "Allows immediate verification of authentication and service availability. The validation performs a real request against the configured provider and reports the result in plain language, never as a raw API response.",
			value: func(m *Model) string { return "Run a check" },
			action: func(m *Model) tea.Cmd {
				return m.testProvider()
			},
		},

		// ---- Workspace ----
		{
			key: "workspace.default", title: "Default Workspace", category: "Workspace", kind: kindChoice,
			desc: "Determines where new sessions begin. The workspace is the project folder the agent reads and writes in, and the root of its sandbox. Changing it moves the agent to another folder; sessions stay available in the session list.",
			value: func(m *Model) string { return m.workspacePath() },
			set: func(m *Model, v string) error {
				return m.changeWorkspace(v)
			},
		},
		{
			key: "workspace.discovery", title: "File Discovery", category: "Workspace", kind: kindToggle,
			desc: "Controls how files are indexed and explored. With discovery on, the agent can search the workspace and match file names before it reads anything; with it off those tools are removed from the agent and only direct file access stays available.",
			value: func(m *Model) string { return onOff(m.fileDiscovery) },
			set: func(m *Model, v string) error {
				m.fileDiscovery = v == "on"
				if m.eng != nil {
					m.eng.SetFileDiscovery(m.fileDiscovery)
				}
				return m.saveState()
			},
		},
		{
			key: "workspace.context", title: "Project Context Management", category: "Workspace", kind: kindToggle,
			desc: "Controls how project knowledge is collected and reused within a session. With it on, the agent receives the project structure, the git state and your memory files at the start of a task; with it off it starts from the request alone.",
			value: func(m *Model) string { return onOff(m.projectContext) },
			set: func(m *Model, v string) error {
				m.projectContext = v == "on"
				if m.eng != nil {
					m.eng.SetProjectContext(m.projectContext)
				}
				return m.saveState()
			},
		},
		{
			key: "workspace.isolation", title: "Workspace Isolation", category: "Workspace", kind: kindToggle,
			desc: "Controls whether projects remain separated from one another. With isolation on, the session list only shows conversations that belong to the current workspace; with it off every local session is listed.",
			value: func(m *Model) string { return onOff(m.workspaceIsolation) },
			set: func(m *Model, v string) error {
				m.workspaceIsolation = v == "on"
				return m.saveState()
			},
		},
	}
}

// onOff renders a boolean as a setting value.
func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// modelName returns the active model.
func (m *Model) modelName() string {
	if m.eng == nil {
		return "not configured"
	}
	_, _, model := m.eng.Info()
	if model == "" {
		return "not configured"
	}
	return model
}

// providerLabel returns the active provider.
func (m *Model) providerLabel() string {
	if m.eng == nil {
		return "not configured"
	}
	_, pid, _ := m.eng.Info()
	return pid
}

// accessLevel returns the sandbox scope of the session.
func (m *Model) accessLevel() string {
	if m.eng == nil {
		return "unavailable"
	}
	return m.eng.AccessLevel()
}

// workspacePath returns the current workspace folder.
func (m *Model) workspacePath() string {
	if m.eng != nil {
		return m.eng.Workspace()
	}
	return m.cwd
}
