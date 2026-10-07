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
| `SearchCriteria.GmailRaw` sends `X-GM-RAW`; `And` joins two queries with a space | `search.go`, `imapclient/search.go` |
| `Client.StoreGmailLabels` sends `STORE ±X-GM-LABELS` | `imapclient/store.go` |

To rebase onto a new upstream release: copy the release over this directory,
reapply the rows above, keep this file and `imapclient/gmail_test.go`, then run
`make test-fork` and `make engine-it`.
