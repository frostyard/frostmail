# Frostmail patches to go-imap

This is github.com/emersion/go-imap/v2 at v2.0.0-beta.8 (MIT, see LICENSE),
vendored by the root `go.mod` `replace` directive. Why it is forked:
[docs/adr/0004-go-imap-fork.md](../../docs/adr/0004-go-imap-fork.md).
Every change is marked "frostmail patch" in a comment and covered by
`imapclient/gmail_test.go`. Keep the set small; send each upstream.

| Change | Files |
| --- | --- |
| `FetchOptions.GmailMsgID/GmailThreadID/GmailLabels` request `X-GM-MSGID`, `X-GM-THRID`, `X-GM-LABELS` | `fetch.go`, `imapclient/fetch.go` |
| FETCH parses those attributes into `FetchItemDataGmail*` and `FetchMessageBuffer.Gmail*`; labels keep `\System` names and decode user labels from modified UTF-7 | `imapclient/fetch.go` |
| FETCH skips unknown attributes with `DiscardValue` instead of failing the response | `imapclient/fetch.go` |
| FETCH parses the `<origin>` of a partial `BINARY[part]<origin>` response, as it already did for BODY (upstream bug: Dovecot's partial BINARY answers failed to parse) | `imapclient/fetch.go` |
| FETCH reads a BODYSTRUCTURE whole (`Decoder.RawValue`) before parsing it; a structure the parser rejects comes back as `FetchItemDataBodyStructure.Unparsed`/`Err` (`FetchMessageBuffer.BodyStructureUnparsed`/`BodyStructureErr`) instead of failing the response and the connection | `internal/imapwire/decoder.go`, `imapclient/fetch.go` |
| A `message/rfc822` (or `global`) part with NIL where the envelope belongs, as Gmail sends the returned message in a bounce, is read as a basic body (`Decoder.Peek`) | `internal/imapwire/decoder.go`, `imapclient/fetch.go` |
| `SessionTracker.Idle` (the server side, used by the in-memory test server) writes the updates queued before it registered: the client hears `+ idling` first, and a message appended in between woke no one (a lost wake-up that made IDLE tests flaky) | `imapserver/tracker.go` |
| `SearchCriteria.GmailRaw` sends `X-GM-RAW`; `And` joins two queries with a space | `search.go`, `imapclient/search.go` |
| `Client.StoreGmailLabels` sends `STORE ±X-GM-LABELS` | `imapclient/store.go` |

To rebase onto a new upstream release: copy the release over this directory,
reapply the rows above, keep this file and `imapclient/gmail_test.go`, then run
`make test-fork` and `make engine-it`.
