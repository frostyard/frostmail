---
id: "0160"
title: mailctl's invitees and --notify
milestone: M5.5
size: S
touch:
  - cmd/mailctl/cal_edit.go
given:
  - cmd/mailctl/cal_invite_test.go
acceptance: go test ./cmd/mailctl/ -run TestCal
---
# T-0160: mailctl's invitees and --notify

## Goal

`mailctl cal add` can invite people, and `cal add` and `cal rm` can say
yes to telling them (ADR-0035): maild refuses a write that would tell
invitees with `notifyRequired` unless it carries `notify: true`.
mailctl turns that refusal into a plain message that names `--notify`.

## Read first

- `docs/adr/0035-invite-through-the-providers-scheduling.md`.
- `schema/rpc/calendar.yaml`: `Invitee`, `EventFields.attendees`,
  `InviteNotice`, `notify` on `create` and `delete`;
  `schema/rpc/errors.yaml`: `notifyRequired`.
- `cmd/mailctl/cal_edit.go`: `calFieldOptions`, `newCalAddCmd`,
  `newCalRmCmd`.
- `api/api.go`: `api.Error` and its `Data`.
- The given test, and `cmd/mailctl/pim_test.go` (`pimFixture`).
- `docs/tasks/EXECUTOR.md`

## Contract

- **`cal add`** gains `--invite ADDRESS` and `--invite-optional ADDRESS`,
  each repeatable (string arrays, so an address is never split at
  commas). When either is given, the fields carry `attendees`: the
  `--invite` addresses with role `required`, then the
  `--invite-optional` ones with role `optional`, as typed (maild
  lowercases and checks them).
- **`--notify`** (a bool) on `cal add` and `cal rm` sends `notify:
  true`; without it, `notify` is left out (never sent as false).
- **The refusal:** when maild answers `notifyRequired`, the command
  fails with `the change sends <kind> to <count> invitees: run it again
  with --notify to send them`, `<kind>` and `<count>` from the error's
  `InviteNotice` data. Other errors are returned as they are.
- Nothing else changes: the output on success, `--json`, the other
  flags.

## Tests (given, do not edit)

`cmd/mailctl/cal_invite_test.go`; the existing mailctl tests must still
pass.

## Gotchas

- `api.Error.Data` is JSON; decode it with `encoding/json/v2` into an
  `api.InviteNotice`.
- Use `errors.As` for the `*api.Error`: the client may wrap it.

## Out of scope

Changing invitees of an existing event from mailctl (`cal edit` does
not exist yet); every file not under `touch`.

## Done when

`make accept T=0160` and `make check` pass, and only the files under
`touch` changed.
