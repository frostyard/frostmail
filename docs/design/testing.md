# Design: testing

| Layer | Where | Runs with |
| --- | --- | --- |
| Unit | every `internal/` package; `tools/` | `make test` (in `make verify`) |
| RPC wire | `internal/rpcserver/server_test.go` over real sockets via `internal/rpctest` | `make test` |
| IMAP, fast | `internal/imapx/imapxtest` (go-imap's in-memory server: no CONDSTORE, QRESYNC or Gmail extensions) | `make test` |
| IMAP fork | `third_party/go-imap/imapclient/gmail_test.go` (scripted server) | `make test-fork` |
| IMAP, real | `*_integration_test.go` (build tag `integration`) against `frostmail-mailtest` | `make engine-it` |
| UI | `app/src/**/*.test.ts` (Vitest) | `make ui-test` (nsl) |
| App shell | the M0 spike report (`FROSTMAIL_SPIKE_EXIT=1`) | `make app-run`, or natively from `build/frostmail-app` |

## The mail server

`dev/incus/mailtest.sh up` builds `frostmail-mailtest` (Debian 13, Dovecot
2.4.1, Postfix 3.10): users `test1`–`test5@mailtest.test`, password
`frostmail-test`, IMAP 143/993, submission 587, self-signed TLS (tests set
`InsecureSkipVerify`). test1's INBOX holds the five messages in
`dev/incus/seed/`. The `clean` snapshot is restored before every
`make engine-it`. Postfix delivers only to `mailtest.test`; other recipients
are rejected with 550. After login Dovecot advertises CONDSTORE, QRESYNC,
IDLE, MOVE, UIDPLUS, ESEARCH, SPECIAL-USE, LIST-STATUS and NOTIFY.

## Planned layers

- **Transcript replay (M1/M4):** `MAILD_IMAP_TRACE=dir` records each
  command connection (C1) to its own file in `dir` (0700, files 0600):
  `C: <line>` for what maild sent and `S: <line>` for what it received,
  after TLS, with LOGIN, AUTHENTICATE and SASL lines replaced by
  `[redacted]` (`internal/imapx/trace.go`). STARTTLS connections are not
  traced. Traces hold the account's mail: they stay on the machine until
  `tools/imaprec` rewrites them into replay scripts, and a replay server
  answers recorded responses. One directory per provider quirk under
  `testdata/transcripts/`.
- **Fault injection (M1):** a `faultconn` that drops, delays and truncates;
  a fake clock; kill-during-sync resume tests.
- **MIME corpus (M1):** anonymized real messages (`mailctl scrub`) with golden
  JSON for structure, chosen body, attachments and decoded headers.
- **Sanitizer (M2):** golden tests, `go test -fuzz`, and a Playwright WebKit
  suite asserting zero network requests on hostile HTML.
- **Performance budgets (M2):** 200k seeded messages; list range < 10 ms,
  search < 150 ms, open < 50 ms, maild RSS < 300 MB.
