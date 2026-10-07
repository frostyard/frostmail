---
id: "0005"
title: Store raw messages by content hash
milestone: M1
size: S
touch:
  - internal/blob/blob.go
given:
  - internal/blob/blob_test.go
acceptance: go test ./internal/blob -race -count=1
---
# T-0005: Store raw messages by content hash

## Goal

Fetched messages are kept as raw RFC 5322 files named by their SHA-256, so
the same content is stored once and a file never changes after it is
written. Implement the four stubbed methods of `blob.Store`.

## Read first

- `internal/blob/blob.go`: `Store`, `ErrInvalidID`, and the stubs with doc
  comments stating each method's contract.
- `docs/tasks/EXECUTOR.md`

## Contract

- A valid ID is exactly 64 characters, each `0-9` or `a-f`. Every method
  checks it first and returns `ErrInvalidID` otherwise. Write one unexported
  helper, `path(id string) (string, error)`, returning
  `filepath.Join(s.dir, id[:2], id)`.
- `Put`: `os.MkdirAll(s.dir, 0o700)`, then `os.CreateTemp(s.dir, ".put-*")`.
  Copy from r with `io.Copy(io.MultiWriter(tmp, h), r)` where
  `h := sha256.New()`. Then check `ctx.Err()`, `Chmod(0o600)`, `Sync`,
  `Close`; create the shard directory `<dir>/<id[:2]>` with 0700; if the
  final path already exists, keep it and drop the temporary file; otherwise
  `os.Rename` into place. On every error path remove the temporary file.
  Wrap errors with `blob:` context and `%w`, so the reader's error stays
  visible.
- `Open`: `os.Open` of the path; the `*os.File` is the `io.ReadCloser`.
  Wrap errors with `%w` so `fs.ErrNotExist` matches.
- `Has`: `os.Stat`; `fs.ErrNotExist` means false with a nil error.
- `Remove`: `os.Remove`; `fs.ErrNotExist` is not an error.
- Delete `errNotImplemented`.

## Tests (given, do not edit)

`internal/blob/blob_test.go`: round trip with layout and modes, idempotent
Put of 1 MiB, Has and Remove, seven invalid IDs, and failed Puts (a failing
reader, a canceled context) leaving no files.

## Gotchas

- The ID is `hex.EncodeToString(h.Sum(nil))`, lowercase.
- Check `ctx.Err()` after copying and before renaming; the canceled-context
  test expects `context.Canceled` and an empty directory.

## Out of scope

Compression, garbage collection, and every file except `internal/blob/blob.go`.

## Done when

`make accept T=0005` and `make check` pass, and only `internal/blob/blob.go`
changed.
