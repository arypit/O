# O — Changelog

Versioning policy: version starts at 0.0.0. Every change bumps the version
and is logged here. Source of truth: `VERSION` and `config/o.config.json` → `version`.

## 0.0.10 — 2026-10-31

**The native TUI now runs on Bubble Tea — keyboard and mouse actually work.**

**Mouse clicks were landing one cell off.** Bubble Tea reports mouse
coordinates zero-based while the click regions were built one-based, so a
click missed its target (or hit the wrong one) and the provider rows were off
by one line. Both are fixed, and clicks now work on the startup menu, the
provider rows and the settings values.

**New settings panel (`/settings`)** — a proper panel with aligned columns:

- **Theme**: dark / light.
- **Refresh rate**: 60, 120, 144 or 440 Hz. The rate is read from the TUI
  state file, otherwise from `"tui": {"refreshHz": 120}` in any O config
  file, otherwise 60. It drives the renderer (`tea.WithFPS`) and the spinner
  tick, and it can be changed live with `↑↓` (row), `←→` (value), `enter`
  (cycle) or by clicking a value.

**Math rendering** — chat text containing LaTeX/KaTeX is rendered as Unicode
in the terminal: `$...$` and `\(...\)` inline, `$$...$$` and `\[...\]` as
display math on its own centred line. Fractions, powers, subscripts, roots,
Greek letters, operators, `\mathbb`/`\mathcal`, delimiters and the common
functions are supported.

**Clean header** — the header row is now completely empty: no brand, no
session id, no provider URL, no tab strip. Every view is reached through the
slash commands: `/chat`, `/providers` (or `/login`), `/changelog`,
`/settings`, plus `/new`, `/clear`, `/theme`, `/sessions`, `/version`,
`/help` and `/exit`.


The previous hand-written console layer mixed two incompatible Windows console
modes: it enabled `ENABLE_VIRTUAL_TERMINAL_INPUT` *and* read input records with
`ReadConsoleInputW`. With VT input enabled the console host queues escape
sequences for `ReadFile`, so the key records came back without a Unicode
character and ordinary letters were dropped; mouse reporting was never
enabled, and the byte-mode fallback parser dropped every byte after the first
one in a chunk and did not understand SGR mouse reports. That is why keys and
the mouse felt broken.

The TUI is now a Bubble Tea program (`github.com/charmbracelet/bubbletea` +
`lipgloss`), still compiled into `o.exe` and still a native full-screen
terminal UI (no Node, no subprocess):

- **Keyboard** — Bubble Tea owns the console in raw mode with VT input, so
  arrows, enter, tab, esc, ctrl+c, ctrl+l, ctrl+u, backspace/delete,
  home/end and page up/down all arrive as real messages.
- **Mouse** — mouse tracking is enabled (`tea.WithMouseCellMotion`), so the
  terminal reports clicks and wheel events. Wheel scrolls the transcript,
  changelog and provider list; clicks are hit-tested against the regions
  rendered in the current frame: the startup menu, the tab bar and the
  provider rows.
- **Full-screen canvas** — the frame is rendered as one styled block that is
  exactly the terminal size, and the theme background is re-applied after
  every style reset, so there are no black gaps at any window size.
- **Clean UI** — the header row is empty (no names, no session id, no
  provider URL), the transcript no longer shows the engine banner
  (`O ready: session=...`), and the theme is only changed through
  `/theme dark|light` without printing anything.
- **Framed composer** — the input sits between two rules with a `>` prompt,
  and the footer shows `mode · provider · model` on the left with the
  command hints on the right.
- **Diagnostics** — `O_TUI_DEBUG=1` logs every key and mouse event to
  `~/.o/tui-debug.log`.

Tests cover the model: menu navigation, composer editing, slash commands,
theme switching and persistence, wheel scrolling, tab clicks, provider clicks
and menu clicks.

## 0.0.9 — 2026-10-31

**The TUI is now native and compiled into `o.exe`** — no Node, no `tsx`, no
TypeScript runtime required to use O. `internal/tui` is a full-screen terminal
UI written in Go (stdlib only):

- **Windows console input** (`internal/tui/term_windows.go`): raw-mode via
  `kernel32` `GetConsoleMode`/`SetConsoleMode`, key events decoded from
  `ReadConsoleInputW`, mouse wheel events (`MOUSE_EVENT`) reported as scroll
  deltas. POSIX equivalent uses `termios` (`term_unix.go`).
- **Flicker-free rendering** (`internal/tui/screen.go`): one composed frame per
  tick, single write, frames skipped when unchanged, cursor hidden, alternate
  screen buffer restored on exit.
- **Centered, resize-aware startup screen**: the greeting block is centered
  horizontally *and* vertically on every frame; terminal resize is picked up
  per-frame through `Size()`.
- **Themes**: dark (default) and light, using the brand palette (warm white
  `#F4F3EF`, ink `#171B20`, signal lime `#C8EE59`, soft grey `#D5D5CE`).
  The choice is persisted to `~/.o/tui.json` (override with `O_TUI_STATE`).
- **Text-presentation icons only** (`internal/tui/icons.go`): `◉ ■ □ • · < > ›
  ▸ ✔ ✖ ▲ ● ─ ϟ` plus a braille spinner. No emoji. `ϟ` marks full-access /
  full-auto permission mode.
- **Tabs**: Chat, Providers, Changelog; slash commands (`/help`, `/theme`,
  `/login`, `/clear`, `/model`, `/exit`) and a composer with history.

**Bug fix — ANSI-aware text width and truncation** (`internal/tui/text.go`):
`visWidth`/`truncTo` previously only understood `ESC [ ... m` (SGR) sequences,
so any other escape sequence (cursor moves, OSC titles, other CSI commands)
was counted as visible text, which shifted centered columns and broke
truncation. Both now use a shared `ansiEnd` scanner that understands CSI,
OSC and two-byte escapes, and decode runes by byte length so multi-byte UTF-8
is measured correctly.

**Scripts now launch the native TUI**: `scripts/tui.ps1` and `scripts/tui.sh`
run `o tui` instead of `node tsx`. `scripts/build.ps1` / `scripts/build.sh`
build only the Go binary by default; pass `-WithNodeTUI` / `--with-node` to
also build the older TypeScript TUI.

**Housekeeping**: removed the temporary TUI debug harness (`debug_test.go`).

## 0.0.8 — 2026-10-31

**Fix: keyboard and mouse input completely broken in TUI** — this was the
primary regression. Three root-cause bugs fixed:

1. **`screen.ts` — raw mode silently swallowed on Windows Terminal**: `setRawMode`
   was called inside `if (inp.isTTY)` with no try/catch. On Windows Terminal
   and some builds, `isTTY` is `true` but `setRawMode` throws, leaving stdin in
   cooked (line-buffered) mode — keystrokes were only delivered after pressing
   Enter. Fixed: wrapped `setRawMode` in try/catch so the `data` listener still
   fires even if raw mode fails, and noted the degraded state clearly.

2. **`screen.ts` — mouse reporting never enabled**: No mouse escape sequences
   were ever written to the terminal. Added `\x1b[?1000h\x1b[?1002h\x1b[?1006h`
   (X10 + button-event tracking + SGR extended coords) on `start()` and the
   matching disable sequence `\x1b[?1006l\x1b[?1002l\x1b[?1000l` on `stop()`.

3. **`keys.ts` — mouse events never parsed**: Even if the terminal emitted
   mouse events, the parser had no decoder for them. Added full parsing of:
   - `CSI M` — X10 classic 3-byte mouse (basic compatibility)
   - `CSI < btn ; x ; y M/m` — SGR extended mouse (modern terminals: Windows
     Terminal, iTerm2, xterm, Kitty). Decodes press, release, motion, and
     scroll-wheel events. Modifier bits (shift, alt, ctrl) preserved. Also
     added SS3 sequences (ESC O <final>) for F1-F4 and function key table.

**New mouse interactions wired into `app.ts`**:
- **Startup screen**: click any menu item to select and activate it.
- **Tab bar**: click `Chat`, `Providers`, or `Changelog` label to switch tab.
- **Scroll wheel**: scrolls transcript (Chat), changelog, or provider list,
  3 lines per notch (everywhere, routed through a central `onScrollWheel`).
- **Providers tab**: click a provider row to highlight/select it.

**Keyboard improvements** (incidental fixes while auditing):
- Fixed operator precedence bug in `onStartupKey`: `k.name === "enter" || k.name === "text" && ...`
  was parsed as `k.name === "enter" || (k.name === "text" && ...)` due to JS
  precedence — parentheses made explicit.
- Startup `activateStartupItem` extracted to shared helper (used by both
  keyboard Enter and mouse click paths).

`tsc` clean. All existing `go test ./...` tests still green.

## 0.0.7 - 2026-10-30
- Local-only git, enforced: the O repository has no remotes (`git remote -v`
  is empty, nothing can be pushed anywhere), `push.default` is set to
  `nothing` in the local git config, and a `pre-push` hook in
  `.git/hooks/pre-push` exits 1 on every push attempt. Nothing in O is ever
  pushed to GitHub or any other host; all history stays on this machine and
  under your control.
- Verified: a temporary local-path remote was added only to prove the
  `pre-push` hook blocks the push (exit 1 before any transfer), then the
  remote was removed again; `git remote -v` is empty and the tree is clean.

## 0.0.6 — 2026-10-30
- Real full-screen TUI (`tui/src/`, launched with `o tui` or `o-tui`, no extra
  npm dependencies — raw ANSI + raw-mode stdin): startup screen with
  New space / Login / Changelog / Exit, then a tabbed main screen
  [Chat | Providers | Changelog] with a bottom composer. No box-drawing
  borders anywhere — layout is lines, spacing and the brand palette
  (#F4F3EF / #171B20 / #C8EE59 / #D5D5CE); `/theme dark|light` persists to
  ~/.o/tui.json.
- Chat tab drives the real engine (`o chat` JSONL): streaming text/reasoning,
  tool calls/results, todos, usage, inline y/n/always permission prompts,
  abort with ctrl+c, slash commands in the composer (/new /clear /login
  /changelog /sessions /resume /theme /version /exit /help), history,
  transcript scrolling.
- Providers tab: lists merged config with masked keys, add form (id, base
  URL, model, masked API key, default) with the same validation as the engine,
  remove/set-default, and a real connection test (GET {baseUrl}/models).
- Login bug fixes from real usage: provider id is now validated
  (letters/digits/._- only) and the base URL is normalized — a trailing
  `/chat/completions` is stripped in both Go (`config.NormalizeBaseURL`) and
  the TUI, so `.../v1/chat/completions` no longer produces HTTP 404.
  New `o provider test [id]` does a real GET /models check.
- New `o tui [args]` subcommand launches the TUI (locates tui/dist/main.js).
- TUI robustness: engine events re-render immediately (async events were not
  drawn before), piped multi-char key chunks are dispatched per character,
  EOF on stdin finishes the running turn before quitting.
- Verified scripted end to end: startup navigation, New space -> real prompt ->
  live SSE response rendered, /help, Providers tab list + real HTTP 200
  connection test, add-provider form saving a normalized URL, Changelog tab,
  clean exit 0 everywhere. `go vet` + `go test ./...` green, `tsc` clean.

## 0.0.5 — 2026-10-30
- Crash-proofing: a panic anywhere now exits as a clean `error: ...` message
  instead of a stack trace. `main()` recovers, the `chat` prompt goroutine and
  the JSONL event emitter recover, `Agent.Run` recovers (with nil-guards for
  Store/Sess), `tools.ExecuteCall` converts a panicking tool into an error
  tool_result (new regression test), and the SSE reader goroutine recovers and
  stops sending when the consumer cancels.
- TUI hardening: spawn failures (`child.on("error")`) and writes to a dead
  engine stdin (EPIPE) are handled; top-level `uncaughtException` /
  `unhandledRejection` handlers print a clean error and exit 1.
- New `scripts/install.ps1` / `scripts/install.sh`: one-command install
  (build engine + TUI; `-AddToPath` / `--add-to-path` put `o` on your PATH).
- Verified no crashes on: garbage stdin to `o chat`, unknown provider, missing
  API key, malformed `$O_CONFIG` JSON, invalid permission mode, `o login` on
  EOF, TUI with a missing engine binary. `go vet` + `go test ./...` green,
  `tsc` clean.

## 0.0.4 — 2026-10-30
- Custom providers via `/login`: the TUI handles slash commands locally —
  `/login` opens an interactive menu to add/update any custom provider
  (id, base URL, API key, model), remove one, or set the default;
  `/login list`, `/login add -id X -base-url URL -model M [-api-key K] [-default]`,
  `/login remove <id>`, `/providers` and `/help` also work.
- Engine: new `o login` (interactive provider manager) and
  `o provider list|add|remove` subcommands; `o chat` multi-turn JSONL protocol
  (stdin prompt/answer/abort, stdout AgentEvent stream) for the TUI.
- `internal/config`: providers persist to the user config (`~/.o/config.json`
  or `$O_USER_CONFIG`) with owner-only (0600) permissions via
  AddProvider / RemoveProvider / SetDefaultProvider / ListProviders.
  Precedence: defaults < shipped config/o.config.json < user config <
  project .o/config.json < $O_CONFIG.
- Real providers only, no mock data: the mock provider is gone from the shipped
  CLI; `o run` / `o chat` resolve the API key from config (or env var) and fail
  with a clear error when it is missing. Tests use a real local httptest
  HTTP/SSE server, never canned responses.
- Verified end to end: added a custom provider through `o-tui /login add ...`,
  then ran `o run --provider <id>` against a real local SSE HTTP server and
  received the streamed text + done events.

## 0.0.3 — 2026-10-30
- TS TUI v0 (`tui/`): spawns the Go engine with `run --output json`, renders
  the JSONL event stream (text, tool calls/results, todos, usage, done) and
  answers `permission_request` events with y/n/always prompts over stdin.
- scripts: build/dev/start/test/tui in `.ps1` and `.sh`.
- Docs updated: README, BLUEPRINT (as-implemented), ROADMAP. Website work dropped.

## 0.0.2 — 2026-10-30
- Search package (`internal/search`): MCTS with UCB1 (C=sqrt(2), robust child),
  Metropolis-Hastings MCMC with Cholesky-based Gaussian proposals,
  adaptive MCMC (Haario: proposal cov = (2.38^2/d)(Cov+eps*I), Welford online
  covariance, ~0.234 target acceptance), planner (`plan` tool) using MCTS over
  action sequences and AMCMC over a sigmoid action-subset relaxation.
- Human-in-the-loop (`internal/hitl`): Beta-Bernoulli acceptance learner
  (conjugate posterior, persisted to ~/.o/hitl.json) wired into the permission
  policy (auto-approve at >=10 samples and mean >= 0.9); interactive, auto and
  JSONL deciders.
- All algorithms unit-tested with fixed seeds (UCB1 values, MH sampling of
  N(0,1), AMCMC acceptance/covariance, Beta posterior mean/variance).

## 0.0.1 — 2026-10-30
- Restructured to Go engine (~65% Go / 35% TS per project rule), stdlib only.
- `internal/config`: config load/merge/validate (O_CONFIG env, config/o.config.json
  walking up, .o/config.json project override, defaults).
- `internal/llm`: Provider interface, registry, OpenAI-compatible SSE streaming
  client (default https://api.adaop.sbs/v1, ADAOP_API_KEY), mock provider.
- `internal/sandbox`: read-only / workspace-write / full-access, path confinement.
- `internal/tools`: registry, JSON-schema arg validation, permission policy
  (glob rules, modes default/auto-edit/plan/full-auto), exec pipeline
  (validate -> permission -> HITL -> hooks -> sandbox -> run -> normalize).
- builtin tools: read, write, edit (exact replace), glob (**), grep, bash
  (timeout), todo, webfetch, plan.
- `internal/context`: message store, chat conversion, token estimate
  (ceil(chars/4)), compaction via provider summary, O.md memory chain, repo
  snapshot + system prompt builder.
- `internal/session`: JSONL persistence, list/resume. `internal/agent`: the
  agent loop with max-turn guard. `internal/subagent`: task tool (depth<=2).
  `internal/mcp`: stdio JSON-RPC client. `internal/hooks`: event shell hooks.
- `cmd/o`: run (flags: -p, --provider/--model/--mode/--sandbox/--output/--headless/
  --resume/--session/--max-turns), config, version, sessions.
- Unit tests across all packages: `go test ./...` green; `go vet` clean;
  end-to-end smoke runs verified (pretty + JSONL + TUI).

## 0.0.0 — 2026-10-30
- Initial scaffold: folder `O/`, research (`docs/RESEARCH.md`), blueprint, config with Adaop domains.
