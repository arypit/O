# O — Research: How Existing Coding Agents Work

Research basis for the O harness blueprint. Sources: public docs and repos of
Anthropic's Claude Code, OpenAI's Codex (codex-rs), the open-source OpenCode
(anomalyco/opencode), the Model Context Protocol spec, and 2026 literature on
coding-agent architecture (Raschka, "Components of a Coding Agent"; arXiv
coding-agent taxonomy papers).

---

## 1. Claude Code (Anthropic — TypeScript, proprietary)

**Agent loop.** A query engine streams events from the LLM API. The model
emits `tool_use` blocks; the harness executes them locally, appends
`tool_result` blocks, and sends everything back. The loop repeats until the
model returns `stop_reason: end_turn` with no tool calls. This is the classic
ReAct-style loop: *reason → act → observe → repeat*.

**Tool set.** Bash, Read, Write, Edit (exact-string replacement, fails if the
old string is not unique), Glob, Grep (ripgrep), WebFetch, WebSearch, TodoWrite,
Task (spawn subagents), NotebookEdit, LSP tools, and MCP-provided tools.

**Context management.**
- System prompt assembled from: identity, environment (OS, cwd, git status,
  date), tool definitions, and memory files.
- `CLAUDE.md` memory files at multiple levels (user `~/.claude/CLAUDE.md`,
  project `CLAUDE.md`, local `CLAUDE.md.local`) are injected into context —
  this is how users steer the agent per-project.
- `@file` mentions pull files into context on demand.
- Auto-compaction: when the context window fills, older turns are summarized
  by the model and replaced with a compact summary.
- Prompt caching: the system prompt and tool definitions are cached (cache
  breakpoints) so repeated turns are cheap.

**Permissions.** Every tool call can be allow / deny / ask. Permission modes:
default, acceptEdits, plan (read-only), bypassPermissions. Rules can be
persisted in settings (`allowedTools`, `disallowedTools`) with patterns like
`Bash(git *)`.

**Sessions & extensibility.** Transcripts persist and can be resumed
(`--resume`), checkpoints allow reverting, hooks run shell commands on events
(PreToolUse, PostToolUse, Stop, …), slash commands are user-defined prompts,
plan mode forces a read-only planning phase first, and headless mode
(`-p "..."`, `--output-format json`) makes it scriptable. MCP servers extend
the tool set over a standard protocol.

## 2. Codex CLI (OpenAI — Rust, open source `codex-rs`)

**Crate layout.** `codex-cli` (TUI), `codex-core` (agent loop + conversation
manager), `codex-exec` (headless `codex exec`), `codex-protocol` (wire types),
`codex-apply-patch` (parses the model's patch format for edits).

**Agent loop.** Turn-based: each user message starts a turn; the model streams
response items (reasoning, messages, function calls); the executor runs each
call, appends results, and the turn ends when the model stops calling tools.
Conversations are recorded as JSONL "rollouts" on disk, so any session can be
resumed and forked.

**Edits.** Instead of exact-string replace, Codex uses an `apply_patch`
tool — the model emits a structured patch (add/update/delete file with
hunks), which the harness applies and reports per-hunk success/failure.

**Sandboxing & approvals.** Three layers:
- Sandbox: Seatbelt on macOS, Landlock on Linux, restricted tokens on Windows.
  Modes: `read-only`, `workspace-write`, `danger-full-access`.
- Approval policies: `suggest` (ask for everything), `auto-edit`
  (auto-approve edits, ask for shell), `full-auto` (auto-approve within
  sandbox).
- Config in `config.toml`: model, reasoning effort, approval policy, sandbox
  mode, MCP servers, notify hooks, profiles.

**Memory.** `AGENTS.md` files (repo root, nested dirs) injected like
CLAUDE.md. Context overflow triggers compaction (summary) automatically.

## 3. OpenCode (open-source — TypeScript/Bun, `anomalyco/opencode`)

**Architecture.** Monorepo: `packages/opencode` (core: session, agent, tool
registry, provider layer), a Solid.js TUI, a desktop app, and an HTTP server
mode (`opencode serve`) that exposes sessions as an API — the same core can
be driven from TUI, web, or IDE.

**Providers.** One provider abstraction over 100+ LLM APIs via the Vercel AI
SDK — any model, local or cloud, swappable per agent via `opencode.json`.

**Agents.** Config-declared agents: a primary agent plus subagents, each with
its own model, system prompt, temperature, and tool allow/deny list. The
`task` tool delegates to a subagent with a bounded context.

**Messages.** Every message is a list of typed "parts": text, reasoning,
tool call + result, step-start/step-finish markers, file attachments,
snapshots. Sessions persist to local storage (SQLite), can be shared as URLs,
and compact automatically (older parts summarized) when the window fills.

**Tools & permissions.** bash, read, write, edit, glob, grep, patch, list,
todowrite/todoread, webfetch, task, question, plus MCP tools. Permissions are
per-tool, per-pattern: `ask` / `allow` / `deny` — configurable globally and
overridden in `AGENTS.md`/`.opencode` rules.

**Extras.** Built-in LSP integration, formatters, git integration, custom
commands, and a plugin system.

## 4. Cross-cutting patterns (2026 literature)

Raschka's "Components of a Coding Agent" decomposes every production harness
into six parts — these are what make agents outperform raw models:

1. **Live repo context** — file tree, git status, memory files injected fresh each turn.
2. **Prompt shape & cache stability** — stable prefix (system + tools) so prompt caching hits.
3. **Validated tools** — strict schemas, argument validation, and useful error feedback fed back to the model.
4. **Context reduction** — compaction/summarization, trimming huge tool outputs, offloading to files.
5. **Structured session memory** — persisted sessions, todos, checkpoints, resume.
6. **Bounded subagents** — delegation with isolated, smaller contexts.

Plus the MCP (Model Context Protocol): a JSON-RPC standard over stdio/HTTP
that lets external "MCP servers" expose tools/resources/prompts to any MCP
client — the universal extension mechanism all three tools adopted.

---

## 5. Master feature list for O (what we will build)

Derived from the above:

| # | Feature | Inspired by |
|---|---------|-------------|
| 1 | Agent loop: stream → tool_use → execute → tool_result → repeat until end_turn | All three |
| 2 | Tool registry with JSON-schema tool definitions sent to the model | All three |
| 3 | Builtin tools: read, write, edit (exact replace + patch), bash/shell, glob, grep, webfetch, todo | Claude Code / OpenCode |
| 4 | Tool execution model: validate args → check permissions → run (sandboxed) → return result/error to model | All three |
| 5 | Permission system: allow / deny / ask per tool + per command pattern; permission modes (default, auto-edit, plan, full-auto) | Claude Code / Codex |
| 6 | Sandbox for shell + file writes (workspace-write default; read-only plan mode) | Codex |
| 7 | Context management: message list, token counting, auto-compaction (summarize old turns), tool-output trimming, offload-to-file | All three |
| 8 | Memory files: `O.md` at user/project/local levels + repo file tree + git status injected per turn | CLAUDE.md / AGENTS.md |
| 9 | System prompt assembly with stable prefix for prompt caching | Raschka |
| 10 | Provider abstraction: pluggable LLM backends (API + local), model registry, per-agent model selection | OpenCode |
| 11 | Sessions: persist transcripts (JSONL), resume, fork, share | Codex / OpenCode |
| 12 | Subagents: spawn bounded child agents (own context, tool subset, model) via a `task` tool | All three |
| 13 | Todo tracking tool (todowrite/todoread) for multi-step plans | Claude Code / OpenCode |
| 14 | Plan mode: read-only planning phase before edits | Claude Code / Codex |
| 15 | MCP client: connect to MCP servers over stdio/HTTP, merge their tools | MCP |
| 16 | Hooks: run shell commands on events (pre/post tool, stop, session start) | Claude Code |
| 17 | Slash commands / custom commands from markdown files | Claude Code / OpenCode |
| 18 | Headless mode: `o -p "prompt"` with JSON/stream output for scripting | Claude Code / Codex exec |
| 19 | TUI: streaming chat, tool-call rendering, permission prompts, diff viewer, todo panel, session picker | All three (built LAST) |
| 20 | HTTP server mode exposing sessions as an API (`api.adaop.sbs` backend) | OpenCode serve |
| 21 | Config: `config/o.config.json` + per-project `.o/config.json` + env vars; domains & branding (adaop.sbs / api.adaop.sbs) | OpenCode / Codex |
| 22 | Checkpoints / revert of file changes | Claude Code |
| 23 | Token usage & cost tracking per turn/session | All three |
| 24 | Website for O company: `adaop.sbs`; API service: `api.adaop.sbs` | — |

## 6. Build order (agreed with the user)

1. **Setup** (this phase): folder `O/`, research, blueprint, config with
   `adaop.sbs` / `api.adaop.sbs`, package scaffolding, shell scripts.
2. **Core first**: agent loop, tool execution model, permissions/sandbox,
   context management + compaction, memory files, provider abstraction,
   sessions, subagents, MCP, hooks, headless mode.
3. **TUI last**: full terminal UI design + polish.
4. **Web surfaces**: `web/` static site served as `adaop.sbs`, `api/`
   server served as `api.adaop.sbs`.
