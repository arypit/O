# O — Roadmap

Long-term goal: a very large, industry-class codebase. We only write real,
tested logic — no padding. Version bumps + CHANGELOG on every change.

## Done (see CHANGELOG.md)
- 0.0.0 scaffold + research
- 0.0.1 Go engine core: config, llm providers (OpenAI-compatible SSE + mock),
  sandbox, tools (registry/validation/permissions/exec pipeline + 9 builtin),
  context (messages/tokens/compaction/memory/system prompt), session JSONL,
  agent loop, MCP stdio client, hooks, subagent task tool, headless CLI, tests
- 0.0.2 search + HITL: MCTS/UCB1, Metropolis-Hastings, adaptive MCMC (Haario),
  planner (plan tool), Beta-Bernoulli learner, interactive/auto/JSONL deciders
- 0.0.3 TS TUI v0 (JSONL stdio client with human permission prompts),
  build/dev/test/tui scripts, docs

## Next
- [ ] Rich TUI in TS: chat pane, tool-call cards, diff viewer, todo panel,
  session picker, slash commands, theme via CSS variables
- [ ] More providers: local (ollama) + anthropic-compatible endpoints
- [ ] Planner wiring: use MCTS/AMCMC to rank candidate tool calls in plan mode
- [ ] MCP resources/prompts, HTTP transport
- [ ] Checkpoints (git-based snapshot/revert of edits)
- [ ] Benchmarks: SWE-bench-style harness running `o run --headless` on tasks
- [ ] Headless JSON output schema stable for api.adaop.sbs serving later
- [ ] Grow package count organically with real features (tools, providers,
  planners, verifiers) — toward 650+ dirs / multi-million-line scale over months
