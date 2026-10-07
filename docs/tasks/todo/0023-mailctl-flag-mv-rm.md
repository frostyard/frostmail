---
id: "0023"
title: Add mailctl flag, mv and rm
milestone: M1
size: S
touch:
  - cmd/mailctl/ops.go
  - cmd/mailctl/root.go
given:
  - cmd/mailctl/ops_test.go
acceptance: go test ./cmd/mailctl -count=1
---
# T-0023: Add mailctl flag, mv and rm

## Goal

Change mail from the command line through maild's offline actions:
`message.setFlags`, `message.move` and `message.delete` apply locally at
once and reach the server when it can be reached.

## Read first

- `cmd/mailctl/show.go`, `cmd/mailctl/list.go`: the command pattern and
  existing helpers in the package.
- `docs/specs/rpc-api.md`: `message.setFlags` (`FlagChanges`),
  `message.move`, `message.delete`.
- The given test `cmd/mailctl/ops_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `cmd/mailctl/ops.go` with `newFlagCmd(opts)`, `newMvCmd(opts)` and
`newRmCmd(opts)`, registered in `root.go` after `newSyncCmd(opts)`. Message
and mailbox IDs are parsed with `strconv.ParseInt`; a bad one is an error.
Output uses `messages`, or `message` when the count is 1.

**`flag ID...`** (`cobra.MinimumNArgs(1)`), flags `--seen`, `--unseen`,
`--flag`, `--unflag` (bools) and `--color N` (int, 0–7, only when the flag
was given: use `cmd.Flags().Changed("color")`). Build `api.FlagChanges`:
`--seen`/`--unseen` set `Seen` to true/false; `--flag`/`--unflag` set
`Flagged`; `--color` sets `FlagColor`. Errors, before calling maild: no
change requested; `--seen` with `--unseen`; `--flag` with `--unflag`; a color
outside 0–7. Call `message.setFlags` with all IDs and print
`updated <n> messages`.

**`mv MAILBOX_ID ID...`** (`cobra.MinimumNArgs(2)`): `message.move`; print
`moved <n> messages`.

**`rm ID...`** (`cobra.MinimumNArgs(1)`): `message.delete`; print
`deleted <n> messages`.

## Tests (given, do not edit)

`cmd/mailctl/ops_test.go`: TestFlagCommand, TestMvAndRmCommands. They run a
real sync engine against an in-memory IMAP server with MOVE and UIDPLUS and
check both the local state and the server.

## Gotchas

- Six invalid `flag` invocations must fail; check them all before dialing.
- Red is color 1: `--flag` alone leaves the color to maild, which sets 1.

## Out of scope

Every file except the two listed.

## Done when

`make accept T=0023` and `make check` pass, and only the two listed files
changed.
