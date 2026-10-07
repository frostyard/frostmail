---
id: "0001"
title: Add the mailctl hello command
milestone: M0
size: S
touch:
  - cmd/mailctl/hello.go
  - cmd/mailctl/root.go
given:
  - cmd/mailctl/hello_test.go
acceptance: go test ./cmd/mailctl -count=1
---
# T-0001: Add the mailctl hello command

## Goal

`mailctl hello` connects to maild, completes `rpc.hello` and prints the
server's name and protocol version. It is the smallest end-to-end check that
maild is running, and the pattern later mailctl commands copy.

## Read first

- `cmd/mailctl/root.go`: `rootOptions.dial` connects and returns the client
  and the `*api.Hello` result. Commands are registered in `root.AddCommand(...)`.
- `cmd/mailctl/main.go`: mailctl runs through `clix.App`, which adds `--json`.
- The `Hello` type in `api/zz_generated.go` (fields `Protocol`, `Server`).
- `docs/tasks/EXECUTOR.md`

## Contract

- Create `cmd/mailctl/hello.go` with
  `func newHelloCmd(opts *rootOptions) *cobra.Command`.
- `Use: "hello"`, a one-line `Short`, `Args: cobra.NoArgs`, and a `RunE`.
- `RunE` calls `opts.dial(cmd.Context())`. It returns a dial error unchanged
  (its message already contains "connect to maild"). After a successful dial
  it closes the client before returning (`defer c.Close()`).
- With `--json`, output is the `*api.Hello` and nothing else. Use exactly:
  `if written, err := clix.OutputJSON(h); err != nil || written { return err }`
- Otherwise write exactly `<server> (protocol <n>)` and a newline to
  `cmd.OutOrStdout()`, for example `maild v0.1.0 (protocol 1)`.
- In `root.go`, replace the placeholder comment inside `root.AddCommand(...)`
  with `newHelloCmd(opts),`. Change nothing else in root.go.

## Tests (given, do not edit)

`cmd/mailctl/hello_test.go`: TestHelloText, TestHelloJSON, TestHelloWithoutMaild.
They start a real maild API server with `internal/rpctest`.

## Gotchas

- Print with `fmt.Fprintf(cmd.OutOrStdout(), ...)`, not `fmt.Println`: the
  tests capture the command's output writer.
- `clix.JSONOutput` is set by the `--json` flag; `clix.OutputJSON` checks it
  for you.

## Out of scope

Other commands, flags, and any change under `api/` or `internal/`.

## Done when

`make accept T=0001` and `make check` pass, and only the two listed files
changed.
