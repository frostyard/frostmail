---
id: "0004"
title: Store secrets in a private JSON file
milestone: M1
size: S
touch:
  - internal/secrets/secrets.go
given:
  - internal/secrets/file_test.go
acceptance: go test ./internal/secrets -race -count=1
---
# T-0004: Store secrets in a private JSON file

## Goal

maild needs account passwords to log in. Until Secret Service arrives in M4,
`secrets.File` keeps them in one JSON file that only the user can read.
Implement its three stubbed methods.

## Read first

- `internal/secrets/secrets.go`: the `Store` interface, `File`, and the
  stubs with doc comments stating each method's contract.
- `docs/tasks/EXECUTOR.md`

## Contract

- Add `mu sync.Mutex` to `File` and hold it for the whole of each method, so
  concurrent calls never lose a write. Delete `errNotImplemented`.
- The file holds one JSON object of string values: `{"key":"value"}`. Use
  `encoding/json/v2` (`json.Marshal`, `json.Unmarshal`).
- Reading: a missing file (`errors.Is(err, fs.ErrNotExist)`) means no
  secrets. Before reading, `os.Stat` the file; if `mode.Perm()&0o077 != 0`,
  return an error that includes the path and the mode. Invalid JSON is an
  error (never `ErrNotFound`).
- `Get` returns `ErrNotFound` for a missing key (wrap is fine; the tests use
  `errors.Is`).
- Writing (`Set`, `Delete`): read the current map (same rules), change it,
  then write atomically: `os.CreateTemp(dir, ".secrets-*")`, write, `Chmod`
  0600, `Sync`, `Close`, then `os.Rename` over the file. On any error remove
  the temporary file. `Delete` of a missing key or file returns nil and need
  not write.
- Error strings start with `secrets:`.

## Tests (given, do not edit)

`internal/secrets/file_test.go`: round trip across instances, missing file
and key, mode 0600 with no temp files left, refusal of a 0644 file, a corrupt
file, delete, and 20 concurrent sets (run with `-race`).

## Gotchas

- `os.CreateTemp` creates files with mode 0600 already, but `Chmod` anyway;
  a umask cannot widen it.
- Write the temporary file in the same directory as the target, or
  `os.Rename` can fail across filesystems.

## Out of scope

Secret Service, encryption, and every file except `internal/secrets/secrets.go`.

## Done when

`make accept T=0004` and `make check` pass, and only
`internal/secrets/secrets.go` changed.
