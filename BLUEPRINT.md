# O — Blueprint (as implemented)

O is an autonomous coding agent by Adaop. Stack rule: ~65% Go (engine), ~35%
TS/JS (TUI + tooling). Go stdlib only — no external Go dependencies, so the
engine builds anywhere with `go build ./cmd/o` and distributes as one binary.

Research basis: `docs/RESEARCH.md` (Claude Code / Codex / OpenCode internals).

## Architecture

```
tui/src/main.ts (TS)  --JSONL over stdio-->  cmd/o  -->  internal/agent (loop)
                                                          |        |
                                                          |        v
                                                          |   llm.Provider --SSE--> api.adaop.sbs/v1
                                                          |        |
                                                          |        v
                                                          |   tools.ExecuteCall: validate -> permissions
                                                          |     -> HITL decider -> hooks -> sandbox -> run
                                                          |        |
                                                          v        v
                                              context.Store <-> session JSONL
                                              (tokens, compaction, O.md memory)
```

## Agent loop (internal/agent)

user prompt -> build context (system prompt + O.md + repo snapshot + history)
-> provider.StreamChat(tools) -> collect text/reasoning/tool_use
-> for each tool call: ExecuteCall -> tool_result -> append -> repeat
until stop_reason end_turn or MaxTurns. Every message is appended to the
session JSONL immediately (crash-safe, resumable with `o run --resume <id>`).

## Tool execution model (internal/tools)

Pipeline per call: parse args JSON -> ValidateArgs (schema: required, types,
additionalProperties) -> Policy.Check (rules by glob, then mode; learned
Beta prior can auto-approve) -> if ask: Decider (interactive / auto / JSONL)
-> PreToolUse hook -> tool.Execute (sandbox enforced inside builtin tools)
-> PostToolUse hook -> truncate/offload output -> tool_result part.

Builtin tools: read, write, edit (exact-string replace, unique unless
replace_all), glob (** support), grep (regexp), bash (timeout, sandboxed),
todo, webfetch, plan (search-based), task (subagent), plus MCP tools.

## Sandbox (internal/sandbox)

read-only: no writes, no exec. workspace-write (default): writes only inside
the workspace (traversal-checked), exec allowed. full-access: everything.

## Context (internal/context)

Store of typed messages -> chat messages for the provider. Token estimate is
ceil(chars/4) + per-message overhead. When usage crosses
compactThreshold*maxTokens, older turns are summarized by the provider into
one summary part (compaction), recent turns kept verbatim. System prompt is
a stable prefix: identity, env, git/file snapshot, O.md memory files
(~/.o/O.md, ./O.md, ./O.md.local), todos.

## Search algorithms (internal/search) — real math

- MCTS with UCB1: score = mean + C*sqrt(ln N_parent / n), C=sqrt(2); robust
  child (most visits) is the recommendation. Used by the `plan` tool over
  action sequences.
- MCMC Metropolis-Hastings: accept with min(1, pi(x')q(x|x')/(pi(x)q(x'|x)));
  Gaussian random-walk proposals via Cholesky of the covariance.
- AMCMC (Haario et al. 2001): proposal cov = (2.38^2/d)*(Cov_chain + eps*I),
  optimal scaling giving ~0.234 acceptance; Welford online covariance;
  adaptation frozen after burn-in. Used to sample diverse plans via a
  sigmoid relaxation of action-subset selection.
- Planner heuristic: score = 2*goalKeywordCoverage + 0.2*reads
  - 0.1*writes - 0.05*bash - 0.02*len.

## Human-in-the-loop (internal/hitl)

Every "ask" permission goes to a Decider. InteractiveDecider prompts on the
terminal (y/n/always) and records outcomes; the Learner keeps a Beta(a,b)
posterior per tool (conjugate Bernoulli): approve -> a+1, reject -> b+1,
mean = a/(a+b), var = ab/((a+b)^2(a+b+1)), prior Beta(1,1), persisted to
~/.o/hitl.json. The permission policy auto-approves when samples >= 10 and
posterior mean >= 0.9. Headless AutoDecider denies unless the learned prior
says otherwise. The TS TUI answers through the JSONL channel.

## Sessions, subagents, MCP, hooks

- session: one JSONL file per session in ~/.o/sessions; `o sessions`,
  `--resume <id>`; transcripts are plain JSON lines.
- subagent: `task` tool spawns a bounded child (depth <= 2, read-only tools,
  own session, smaller max turns) and returns its final answer.
- mcp: stdio JSON-RPC client (initialize handshake, tools/list, tools/call);
  remote tools registered as `server__tool`, mutating by default.
- hooks: shell commands per event from config (`PreToolUse` non-zero exit
  blocks the call).

## Config & domains

`config/o.config.json` (global) merged with `.o/config.json` (project) and
`O_CONFIG` env. Domains: website `adaop.sbs`, api `api.adaop.sbs`; the default
provider base URL is `https://api.adaop.sbs/v1` with `ADAOP_API_KEY`.

## Phases

1. setup + Go engine core + search + HITL + TUI v0 — done (see CHANGELOG 0.0.1-0.0.3)
2. full rich TUI (panes, diff viewer, session picker) in TS
3. more providers (local models), richer planner wiring, MCP resources/prompts
4. scale: more tools, benchmarks, docs — growing toward the long-term size goal
   with real code only
