---
id: "0114"
title: Add mailctl rules
milestone: M5
size: M
touch:
  - cmd/mailctl/rules.go
  - cmd/mailctl/root.go
given:
  - cmd/mailctl/rules_test.go
acceptance: go test ./cmd/mailctl -run 'TestRules' -count=1
---
# T-0114: Add mailctl rules

## Goal

`mailctl rules` lists, turns on and off, reorders, removes and imports
maild's rules, and applies them to messages, so rules can be checked and
kept from a shell; with `--json rules ls` and `rules import`, a rules file
is a backup.

## Read first

- `cmd/mailctl/tasks.go`: a command group, `clix.OutputJSON`, tabwriter
  tables.
- `cmd/mailctl/ops.go`: `parseIDs`, `messageWord`.
- `cmd/mailctl/people.go`: `pimCell`.
- `cmd/mailctl/root.go`: where commands are added.
- `schema/rpc/rule.yaml` and `api/zz_generated.go`: `rule.*`, `Rule`,
  `RuleApplied`.
- The given test; `docs/tasks/EXECUTOR.md`.

## Contract

`newRulesCmd(opts)` in `rules.go`, added in `root.go` after `newTasksCmd`:
`rules` with these subcommands. Every argument is checked before maild is
dialed; a bad one is an error.

- **`ls`** (no arguments): `rule.list`. With `--json`, the rules as JSON.
  Otherwise "no rules" when there are none, else a table from
  `tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)`: header `#\tID\tON\tNAME\tPROBLEM`,
  then per rule its position plus one, its ID, `x` when enabled or `-`,
  its name, and its problem or `-` (`pimCell`).
- **`on RULE_ID...`**, **`off RULE_ID...`**: `rule.update {id, enabled}`
  for each, in order. Prints `updated N rule` or `updated N rules`; with
  `--json`, the updated rules.
- **`rm RULE_ID...`**: `rule.delete` for each. Prints `removed N rule(s)`
  the same way.
- **`mv RULE_ID POSITION`**: POSITION counts from 1, as `ls`'s `#` column;
  less than 1 or not a number is an error. `rule.move {id, position:
  POSITION - 1}`. Prints `moved rule ID to POSITION`.
- **`apply MESSAGE_ID...`**: `rule.apply {ids}`. Prints `M of N messages
  matched a rule` (`message` for N of 1, by `messageWord`); with `--json`,
  the `RuleApplied`.
- **`import FILE`** (`-` reads stdin): a JSON array of rules as
  `--json rules ls` prints them. An empty array is an error. Each is
  created in order with `rule.create {name, conditions, actions,
  enabled}` (ID, position and problem ignored). Prints `added N rule(s)`.
  When maild refuses one, the error names it (`rule K "<name>"`) and says
  how many were added before it.

## Tests (given, do not edit)

`rules_test.go`: `TestRulesCommands` (against `rpctest`) and
`TestRulesApplyCommand` (against the synced in-memory server of
`ops_test.go`'s `opsFixture`).

## Gotchas

- Use `encoding/json/v2` for reading the import file, as the repository
  does; `Rule`'s optional fields are pointers with `omitzero`.
- An `enabled: false` rule must import off: pass `Enabled` explicitly.

## Out of scope

maild, the app, and every file not under `touch`.

## Done when

`make accept T=0114` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
