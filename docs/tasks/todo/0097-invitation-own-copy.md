---
id: "0097"
title: An invitation is no conflict with its own event
milestone: M4.5
size: S
touch:
  - internal/engine/invitation.go
given:
  - internal/engine/invitation_own_copy_test.go
acceptance: go test ./internal/engine -count=1
---
# T-0097: An invitation is no conflict with its own event

## Goal

The user invited their iCloud account from their throwaway Gmail
account, both in Frostmail. The invitation card listed the organizer's
copy of the same event, in the throwaway's calendar, as a conflict with
itself. The same event in any of the user's calendars (by UID) is never
a conflict or an adjacent event.

## Read first

- `internal/engine/invitation.go`: `Invitation` and
  `invitationOccurrences` (`own` holds the invitation's stored events in
  the receiving account and is skipped).
- `internal/store/events.go`: `EventsByUID` (per account),
  `DB.ListAccounts`.
- The given test; `docs/tasks/EXECUTOR.md`.

## Contract

`invitationOccurrences` also skips the occurrences of every stored event
with the invitation's UID (`out.Event.UID`) in any account. The other
rules (declined, cancelled, all-day and transparent events) are
unchanged.

## Tests (given, do not edit)

`internal/engine/invitation_own_copy_test.go`. The other invitation tests
must keep passing.

## Out of scope

The store, the schema, the app, and every file not under `touch`.

## Done when

`make accept T=0097` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
