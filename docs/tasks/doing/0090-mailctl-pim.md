---
id: "0090"
title: People, calendar and tasks in mailctl
milestone: M4.5
size: M
touch:
  - cmd/mailctl/people.go
  - cmd/mailctl/cal.go
  - cmd/mailctl/tasks.go
  - cmd/mailctl/root.go
given:
  - cmd/mailctl/pim_test.go
acceptance: go test ./cmd/mailctl -count=1
---
# T-0090: People, calendar and tasks in mailctl

## Goal

`mailctl people`, `mailctl cal` and `mailctl tasks`: the PIM data from the
terminal, for scripts and for checking a sync without the app.

## Read first

- `cmd/mailctl/root.go` (the command list, `dial`), `list.go` (tables
  with `tabwriter`, `clix.OutputJSON` for `--json`, "no messages"),
  `show.go`; one `newXxxCmd(opts)` per file.
- `schema/rpc/people.yaml`, `calendar.yaml` (`range`), `tasks.yaml`,
  `account.yaml` (`collections`).
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

Every command dials maild with `opts.dial`, prints `clix.OutputJSON` of
the API's answer when `--json` is given (and nothing else), and returns
the API's errors. Tables use `tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)`;
an empty value prints as `-` so every column has a cell.

- **`people ls [TERMS...] [--book ID]`** (`people.go`): `people.list`
  with the terms joined by spaces as `query` and `--book` as
  `collectionId`. Columns `ID NAME EMAIL ORGANIZATION`; "no people" when
  empty.
- **`people show ID`**: `people.get`; prints the display name, then
  `Organization: X` when set, then for each contact in order its emails
  as `Email: <value> (<label>)` and phones as `Phone: <value> (<label>)`
  (` (<label>)` left out when the label is empty), and `Note: X` when
  the contact has one. A bad ID is an error.
- **`cal ls`** (`cal.go`): `account.collections` of kind calendar.
  Columns `ID ACCOUNT NAME SHOWN READ-ONLY`, the last two `yes`/`no`
  (`SHOWN` is `enabled`).
- **`cal agenda [--from YYYY-MM-DD] [--days N] [--zone IANA]`**:
  `calendar.range` from `--from` (default: today in the zone) for
  `--days` days (default 7, at least 1) in `--zone` (default: the local
  zone; it must load with `time.LoadLocation`). Prints, in the answer's
  order, a header `YYYY-MM-DD Mon` before the first occurrence of each
  day (an all-day occurrence's `startDate`, a timed one's start day in
  the zone), then each occurrence as two spaces, `HH:MM–HH:MM` (en dash,
  in the zone) or `all day` padded to the same 11 columns, two spaces,
  the summary, ` (<location>)` when set and ` [cancelled]` when
  cancelled. "no events" when empty. A bad date, day count or zone is
  an error.
- **`tasks ls [--list ID] [--all] [--due-before YYYY-MM-DD]`**
  (`tasks.go`): `tasks.list` (`--all` is `completed: true`). Columns
  `ID DONE DUE LIST TITLE`: `x` or `-`, the due date or `-`, the list's
  name (from `account.collections` of kind tasklist), and the title,
  prefixed `↳ ` for a subtask. "no tasks" when empty.
- **`tasks add TITLE... [--list ID] [--due DATE] [--notes TEXT]
  [--parent ID]`**: `tasks.create` with the words joined by spaces;
  prints `task <ID>`. No title is an error.
- **`tasks done ID [--undo]`**: `tasks.update` with `completed` true (or
  false with `--undo`); prints nothing.
- **`tasks rm ID`**: `tasks.delete`; prints nothing.
- `root.go` adds the three commands after `verify`.

## Tests (given, do not edit)

`cmd/mailctl/pim_test.go` seeds a server's store with an address book, a
shown and a hidden calendar, and two Google task lists, and runs the
commands. Cells are compared after splitting lines at runs of two or
more spaces.

## Gotchas

- `–` is one rune but three bytes: pad `all day` by columns (to 11), not
  with `%-11s` on the time string.
- Parse IDs with `strconv.ParseInt` and report a bad one as an error.
- Keep functions under 60 lines.

## Out of scope

Editing people or events, and every file not under `touch`.

## Done when

`make accept T=0090` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
