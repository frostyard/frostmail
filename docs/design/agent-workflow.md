# Design: agent workflow

Decided in [ADR-0008](../adr/0008-local-executor-workflow.md); the
executor is Codex since [ADR-0021](../adr/0021-run-task-cards-with-codex.md).

## Roles

- **Planner (Claude):** milestone plans, ADRs, designs, specs, the RPC and
  SQL schemas and migrations, interfaces, security boundaries (credentials,
  OAuth, the sanitizer, what is sent), fork patches, task cards with
  contract tests, and a review of every card before it merges.
- **Executor (Codex, the user's model; opencode with a local model on
  request):** one task card at a time.
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
3. It runs the executor with the card (`EXECUTOR`, `codex` by default):
   `codex exec` in the `workspace-write` sandbox, with Go's build cache and
   golangci-lint's and mise's caches writable and the network on. Codex
   loads `AGENTS.md` itself and runs the Go gates through
   `mise exec -- make …`; it cannot reach the nsl machine, so for a card
   that touches `app/` taskrun runs `make ui-fmt` before checking.
   (`EXECUTOR=opencode` runs `opencode run --model $(EXECUTOR_MODEL)`,
   which loads `AGENTS.md` and `docs/tasks/EXECUTOR.md` through
   `opencode.json` and may only run `make`, `go` and read-only commands.)
4. After each attempt it runs the acceptance command, then `taskrun verify`;
   on either failure it continues the session (`codex exec resume --last`,
   or `opencode run --continue`) with the error and the last 80 lines of
   output, up to `-attempts` (3).
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

`EXECUTOR` in the Makefile picks the executor: `codex` (default) or
`opencode`. Codex uses the user's default model from `~/.codex/config.toml`
unless `EXECUTOR_MODEL` names one; opencode needs `EXECUTOR_MODEL` as a
`provider/model`, whose endpoint lives in the user's opencode config. Per
run: `make task T=0001 EXECUTOR=opencode
EXECUTOR_MODEL=selfie/halogen-qwen3.8-flash-next`.

## Sizing

- **S:** at most 150 non-test lines in at most 3 files.
- **M:** at most 400 lines in at most 6 files.
- **L:** at most 1,000 lines: one package or one feature (a module of the
  app, a sync loop with its store queries), with Codex only.

The local model ran S and, after the M0 calibration cards, M
([plan 0001](../plans/0001-m0-foundations.md)). A card of any size keeps one
concern, a fixed file list and given tests at its boundary; schemas,
migrations and security boundaries stay with the planner regardless of size.
