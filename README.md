# O

**O** is an autonomous coding agent built by **Adaop** (adaop.sbs / api.adaop.sbs).

Stack rule: **~65% Go, ~35% TypeScript/JavaScript**. The Go engine is the core
(agent loop, tool execution, sandboxing, permissions, context, sessions, MCP,
search algorithms, human-in-the-loop). The TS layer is the terminal UI client
plus shell tooling.

## Layout

```
O/
├── go.mod                  # module github.com/adaop/o (stdlib only)
├── cmd/o/                  # CLI entry: run | chat | login | provider | config | version | sessions | tools
├── internal/
│   ├── core/               # wire types: Message, Part, ToolCall, AgentEvent
│   ├── config/             # config load/merge/validate (config/o.config.json + .o/config.json)
│   ├── llm/                # Provider iface + OpenAI-compatible SSE streaming client (real HTTP only)
│   ├── tools/              # registry, JSON-schema validation, permission policy, exec pipeline
│   │   └── builtin/        # read write edit glob grep bash todo webfetch plan
│   ├── sandbox/            # read-only | workspace-write | full-access confinement
│   ├── context/            # messages, token estimate, compaction, O.md memory, system prompt
│   ├── session/            # JSONL session persistence, list/resume
│   ├── agent/              # the agent loop (stream -> tool calls -> results -> end_turn)
│   ├── subagent/           # task tool: bounded child agents
│   ├── search/             # MCTS (UCB1), Metropolis-Hastings MCMC, adaptive MCMC, planner
│   ├── hitl/               # human-in-the-loop: Beta-Bernoulli learner + deciders
│   ├── mcp/                # MCP stdio client (JSON-RPC), tools merged as server__tool
│   └── hooks/              # shell hooks on events (PreToolUse blocks on non-zero exit)
├── pkg/o/                  # public Go facade for embedding
├── tui/                    # TypeScript full-screen TUI (startup screen, tabs, composer; drives the engine via JSONL stdio)
├── config/o.config.json    # domains adaop.sbs / api.adaop.sbs, providers, agents, permissions
├── scripts/                # install/build/dev/start/test/tui in .ps1 and .sh
├── docs/                   # RESEARCH.md (how Claude Code/Codex/OpenCode work), ROADMAP.md
├── BLUEPRINT.md  CHANGELOG.md  VERSION
```

## Quick start

```powershell
.\scripts\install.ps1               # one-command install: build engine + TUI (-AddToPath puts o.exe on your PATH)
.\scripts\test.ps1                  # go vet + go test ./...
.\scripts\start.ps1 version
.\scripts\start.ps1 login                            # add a custom provider (id, base URL, API key, model)
.\scripts\start.ps1 run -p "refactor X" --provider adaop
.\scripts\tui.ps1 "what does this repo do?" --provider adaop   # TS TUI
.\scripts\tui.ps1 /login                             # provider manager from the TUI
```

```bash
./scripts/install.sh                # one-command install (--add-to-path symlinks bin/o into ~/.local/bin)
./scripts/test.sh
./scripts/start.sh version
./scripts/start.sh login
./scripts/start.sh run -p "..." --provider adaop
./scripts/tui.sh "..." --provider adaop
./scripts/tui.sh /login
```

## The TUI

`o tui` (or `./scripts/tui.ps1`) opens the full-screen UI — no extra npm
dependencies, just raw ANSI:

- **Startup screen**: New space / Login / Changelog / Exit (↑/↓ + Enter).
- **Main screen**: tabs `[Chat | Providers | Changelog]` (Tab to switch, no
  borders), a status line, a scrollable transcript and a bottom composer.
- **Chat tab**: drives the real engine (`o chat` JSONL) — streaming text,
  tool calls, inline y/n/always permission prompts, ctrl+c aborts. Slash
  commands in the composer: `/new /clear /login /changelog /sessions
  /resume <id> /theme dark|light /version /exit /help`.
- **Providers tab**: list providers (keys masked), `a` add (id, base URL,
  model, masked API key, default), `x` remove, `d` set default, `t` real
  connection test (GET {baseUrl}/models).
- **Changelog tab**: scrollable CHANGELOG.md.

`o-tui /login ...` still runs standalone provider commands without the full
screen. Theme persists to `~/.o/tui.json`.

## How it runs

`o run -p "prompt" --output json` emits a JSONL event stream on stdout
(text, reasoning, tool_call, tool_result, permission_request, todo, turn_end,
done). The TUI drives the engine through `o chat` on the same stream and
answers permission_request events by writing `{"id","allow"}` to the engine
stdin — that is the human-in-the-loop channel. In `--headless` mode an auto
decider uses the learned Beta-Bernoulli acceptance rate per tool (safe
default: deny).

Permission modes: `default` (ask on mutating), `auto-edit`, `plan` (read-only),
`full-auto`. Sandbox modes: `read-only`, `workspace-write` (default),
`full-access`.

## Providers and /login

Providers are OpenAI-compatible endpoints (base URL + model + API key). The
built-in `adaop` provider uses `https://api.adaop.sbs/v1` and the
`ADAOP_API_KEY` env var. Add any custom provider with:

```
o login                                                            # interactive menu: add / remove / set default
o provider add -id myllm -base-url https://llm.example.com/v1 -model my-model -api-key sk-... -default
o-tui /login                                                       # same menu from the TUI composer
o-tui /login add -id myllm -base-url https://... -model ... -api-key ...   # non-interactive
o provider list                                                    # keys always shown masked
```

Keys are stored in the user config (`~/.o/config.json`, or `$O_USER_CONFIG`),
written with owner-only (0600) permissions. There is no mock provider: runs
always make real HTTP calls and fail with a clear error when a key is missing.

## Versioning

Version starts at 0.0.0 and is bumped on every change; see `CHANGELOG.md`.
Source of truth: `VERSION` and `config/o.config.json` -> `version`.
