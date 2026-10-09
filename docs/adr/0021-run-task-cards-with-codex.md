# 0021 — Run task cards with Codex

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

[ADR-0008](0008-local-executor-workflow.md) split the work: Claude plans,
writes contracts and given tests, builds the hard parts and reviews; a local
model (Qwen through opencode, 6B active parameters) implements small task
cards. The cards had to be small (size S: 150 lines in 3 files) because the
model was. For M4.5 the user wants the cards run by Codex (the OpenAI Codex
CLI, 0.155) on their Codex subscription with its default model (a GPT model
at high reasoning), which can take on far larger units of work.

Codex reads `AGENTS.md`, this repository's instruction file, by itself.
`codex exec` runs non-interactively; its `workspace-write` sandbox lets
commands write the repository and `/tmp`, with the network off unless
configured on. Checked
2026-10-08: in that sandbox `go test` works once Go's build cache is
writable; the shell is a login shell without mise's tools, so the pinned Go
and golangci-lint come through `mise exec`; and the nsl machine, where the
app's checks run, cannot be reached even with its state directories
writable and the network on. Without the network, the sandbox also
refuses loopback and Unix sockets, so `make check`'s server tests fail in
it (checked with T-0060, the first card Codex ran).

## Decision

- **Codex is the executor.** `make task T=NNNN` runs `tools/taskrun` with
  `-executor codex` (the Makefile's `EXECUTOR`): `codex exec` in the
  `workspace-write` sandbox, with Go's build cache and golangci-lint's and
  mise's caches writable, the network on, approvals off, and the model the
  user's Codex default unless `EXECUTOR_MODEL` names one. A retry resumes the same session
  (`codex exec resume --last`, run from the repository) with the failure.
  opencode stays available (`EXECUTOR=opencode`).
- **The network is on.** Without it the sandbox refuses every socket,
  loopback included, so tests that start an in-process server (httptest,
  rpctest, davtest) fail inside it; the user chose to give Codex the
  network for every card rather than per card.
- **Codex runs the Go gates itself** through `mise exec -- make …`. For a
  card that touches `app/`, taskrun runs `make ui-fmt`, the acceptance
  command and `make ui-check` outside the sandbox after each attempt and
  sends Codex any failure.
- **Larger cards.** Cards may be S, M (400 lines in 6 files) or L (1,000
  lines: one package or one feature, such as a module of the app or a sync
  loop with its store queries). A card still has a fixed file list, a
  contract and given tests that taskrun protects byte for byte.
- **What stays with Claude:** plans, ADRs, designs and specs; the RPC and SQL
  schemas and migrations; the contracts and given tests of every card; the
  security boundaries (credentials, OAuth, the HTML sanitizer, what is
  sent); fork patches; and a review of every card before it merges, not only
  at the milestone's end.

## Consequences

- A milestone's implementation moves faster and in larger pieces; more of
  the engine can go on cards, since the contracts, not the model's size,
  bound what a card may do.
- App cards get slower feedback: formatting and Vitest failures come back
  from taskrun between attempts instead of from Codex's own runs.
- The work spends the user's Codex quota, not a local GPU.
- Larger cards need sharper contracts: given tests cover behavior at the
  package boundary, and the per-card review is where design drift is caught.

## Alternatives considered

- **Keep the local model:** cards stay tiny and the planner keeps most of
  the engine, which is what made M1–M4 slow to hand out.
- **Codex without a sandbox (`danger-full-access`):** would reach nsl, but
  gives the executor the whole machine; the app's checks run fine from
  taskrun instead.

## References

- Shapes: [design/agent-workflow.md](../design/agent-workflow.md),
  [tasks/EXECUTOR.md](../tasks/EXECUTOR.md), [tasks/TEMPLATE.md](../tasks/TEMPLATE.md)
- Builds on: [ADR-0008](0008-local-executor-workflow.md)
