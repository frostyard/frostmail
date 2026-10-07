# Design: agent workflow

Decided in [ADR-0008](../adr/0008-local-executor-workflow.md).

## Roles

- **Planner (Claude):** milestone plans, ADRs, designs, the RPC and SQL
  schemas, interfaces, hard subsystems, task cards with contract tests, and
  milestone reviews.
- **Executor (local model through opencode):** one task card at a time.
- **Human:** approves plans, merges task branches, runs anything with
  credentials.

## A card's life

1. The planner writes `docs/tasks/todo/NNNN-slug.md` from
   [TEMPLATE.md](../tasks/TEMPLATE.md), with given files under
   `NNNN-slug/_given/<repo path>` (the underscore keeps `go ./...` out), and
   checks the given tests against a reference solution that is then deleted.
   Stubs the card replaces may live on main if main stays green.
2. `make task T=NNNN` (`tools/taskrun run`) requires a clean tree, creates
   branch `task/NNNN`, copies the given files into place, moves the card to
   `doing/` and commits that as `chore(tasks): start T-NNNN …`.
3. It runs `opencode run --model $(EXECUTOR_MODEL)` with the card. opencode
   loads `AGENTS.md` and `docs/tasks/EXECUTOR.md` (`opencode.json`
   `instructions`) and may only run `make`, `go` and read-only commands.
4. After each attempt it runs the acceptance command, then `taskrun verify`;
   on either failure it continues the session with the error and the last 80
   lines of output, up to `-attempts` (3).
5. `taskrun verify` (and `make task-verify T=NNNN`) fails if any changed file
   is outside `touch`, a given file differs from `_given`, acceptance fails,
   `make check` fails, or, for a card that touches `app/`, `make ui-check`
   fails.
6. `make task-finish T=NNNN` re-verifies, moves the card to `done/` and
   commits the work. The human reviews and merges the branch; the planner
   reviews all of a milestone's merged cards.

## Batching cards

Cards run one at a time in the executor worktree (`.worktrees/exec`), each
branched from `main` as it is when the card starts. Two cards branched from
the same `main` cannot see each other's code, so:

- Cards that touch the same file must run one after another, with the first
  merged before the second starts (T-0016 and T-0017 both edit `read.go`).
- Cards in the same Go package can collide even in different files: T-0012
  and T-0013 each added a package-level `bit()` helper, and only the merge
  failed to compile. Run same-package cards sequentially with merges in
  between, or name shared helpers in the card's contract.
- A card whose given tests call another card's code waits for that card's
  merge (T-0016 needs T-0011's `SearchQuery`).
- Run `make verify` before committing a card and its stubs. A stub that
  always fails can trip staticcheck (SA4023) in its callers, and a red
  `main` fails every card's final `make check`.

## Executor model

`EXECUTOR_MODEL` in the Makefile names an opencode `provider/model`; the
provider (endpoint and model ID) lives in the user's global opencode config,
not in this repository. Override per run:
`make task T=0001 EXECUTOR_MODEL=provider/model`.

## Sizing

Size S: at most 150 non-test lines in at most 3 files. Size M (at most 400)
was allowed once the M0 calibration cards (T-0001 CLI, T-0002 SQL, T-0003
pure logic) passed with at most one retry each; all three passed on the first
attempt in 1–2 minutes ([plan 0001](../plans/0001-m0-foundations.md)). Keep a
card to one concern even when it is size M, and keep sync, threading and
security code with the planner regardless of size.
