---
id: "0014"
title: Add mailctl account add, list and rm
milestone: M1
size: M
touch:
  - cmd/mailctl/account.go
  - cmd/mailctl/root.go
given:
  - cmd/mailctl/account_test.go
acceptance: go test ./cmd/mailctl -count=1
---
# T-0014: Add mailctl account add, list and rm

## Goal

Until the app has an account wizard, accounts are managed from the command
line. Add `mailctl account` with `add`, `list` and `rm` subcommands over the
`account.*` RPC methods.

## Read first

- `cmd/mailctl/hello.go` and `cmd/mailctl/root.go`: the command pattern
  (`newXxxCmd(opts)`, `opts.dial`, `clix.OutputJSON`, `cmd.OutOrStdout()`).
- `docs/specs/rpc-api.md`, section "account": `create`, `list`, `delete`,
  `setPassword`, and the `ServerConfig` and `TLSMode` types.
- The given test `cmd/mailctl/account_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `cmd/mailctl/account.go` with `newAccountCmd(opts *rootOptions)
*cobra.Command` (`Use: "account"`, no `RunE`) holding three subcommands, and
register it in `root.go` after `newHelloCmd(opts)`. Also define:

**`parseServer(s, username string) (api.ServerConfig, error)`** for
`host:port[/mode]`:
- Split an optional `/mode` suffix at the last `/`; mode must be `tls`,
  `starttls` or `insecure`.
- Split host and port with `net.SplitHostPort` (so `[::1]:143` works). The
  host must not be empty; the port must be 1–65535.
- Without a mode, ports 993 and 465 mean `tls`; 143, 587 and 25 mean
  `starttls`; any other port is an error asking for an explicit mode.
- `Username` is the given username.

**`account add EMAIL`** (`cobra.ExactArgs(1)`), flags: `--name` (display
name, default empty), `--imap` and `--smtp` (required: return an error if
either is empty), `--username` (default: EMAIL), `--password-stdin` (bool).
Call `account.create` with kind `imap`, auth `password`, and both servers
from `parseServer`. With `--password-stdin`, read the first line of
`cmd.InOrStdin()`, trim `\r\n`, and call `account.setPassword`. Print
`added account <id> (<email>)`; with `--json` output the created
`api.Account` instead.

**`account list`**: call `account.list`. With `--json` output the slice.
Otherwise print a table with `text/tabwriter`
(`tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)`), header `ID EMAIL IMAP SMTP`,
one row per account; a server is shown as `host:port/mode`.

**`account rm ID`** (`cobra.ExactArgs(1)`): parse ID with
`strconv.ParseInt` (an error for non-numbers), call `account.delete`, print
`removed account <id>`. A maild error is returned unchanged.

## Tests (given, do not edit)

`cmd/mailctl/account_test.go`: TestParseServer (7 good, 7 bad inputs),
TestAccountAddListRemove, TestAccountAddNeedsServers.

## Gotchas

- Every subcommand dials with `opts.dial(cmd.Context())` and closes the
  client with `defer`.
- Use `bufio.NewReader(cmd.InOrStdin()).ReadString('\n')`; at end of input
  without a newline, `io.EOF` with text read is not an error.
- The table's last column has no trailing spaces with the tabwriter settings
  above.

## Out of scope

OAuth, provider autoconfiguration (M4), editing accounts, and every file
except the two listed.

## Done when

`make accept T=0014` and `make check` pass, and only the two listed files
changed.
