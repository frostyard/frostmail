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
4. After each attempt it runs the acceptance command; on failure it continues
   the session with the last 80 lines of output, up to `-attempts` (3).
5. `taskrun verify` (and `make task-verify T=NNNN`) fails if any changed file
   is outside `touch`, a given file differs from `_given`, acceptance fails, or
   `make check` fails.
6. `make task-finish T=NNNN` re-verifies, moves the card to `done/` and
   commits the work. The human reviews and merges the branch; the planner
   reviews all of a milestone's merged cards.

## Executor model

`EXECUTOR_MODEL` in the Makefile names an opencode `provider/model`; the
provider (endpoint and model ID) lives in the user's global opencode config,
not in this repository. Override per run:
`make task T=0001 EXECUTOR_MODEL=provider/model`.

## Sizing

Size S: at most 150 non-test lines in at most 3 files. Size M (at most 400)
is allowed only after the M0 calibration cards (T-0001 CLI, T-0002 SQL,
T-0003 pure logic) pass with at most one retry each.
