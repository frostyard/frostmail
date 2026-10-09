# 0008 — Claude plans and verifies; a local model executes task cards

- **Status:** Accepted; the executor and card sizes superseded by
  [0021](0021-run-task-cards-with-codex.md)
- **Date:** 2026-10-07

## Context

Frontier-model time is spent where judgment matters: architecture, sync
correctness, security and review. A local mixture-of-experts model
(qwen3.8-flash-next: 125B parameters, 6B active, served through an
OpenAI-compatible endpoint and driven by opencode) is fast and free per token
but loses track in long or ambiguous tasks.

## Decision

- Claude writes ADRs, designs, the RPC schema and SQL schema, interfaces,
  and the contract tests for each task, and implements the hard parts: sync,
  threading, the HTML sanitizer, OAuth, the outbox, the Tauri bridge and the
  generators.
- The executor model implements task cards (`docs/tasks/`): small (size S is
  at most 150 non-test lines in at most 3 files), with a fixed file list, an
  exact contract and given tests checked against a reference solution before
  the card is published.
- `tools/taskrun` (`make task T=NNNN`) runs a card on its own branch,
  retries with the failure output, and rejects the result if files outside
  the card changed, a given test was modified, or the acceptance command or
  `make check` fails.
- Claude reviews merged task branches at each milestone boundary.

## Consequences

- Most code volume moves to the local model; contracts and tests keep its
  output honest.
- Writing a good card costs planner time. The M0 calibration cards measure
  whether larger (M) cards are worth allowing.
- The executor never touches schema, generated code, dependencies or
  migrations.

## Alternatives considered

- **Let the local model take whole features:** its 6B active parameters lose
  coherence across many files.
- **Claude writes everything:** slower and more expensive for routine code.

## References

- Shapes: [design/agent-workflow.md](../design/agent-workflow.md), [tasks/EXECUTOR.md](../tasks/EXECUTOR.md)
