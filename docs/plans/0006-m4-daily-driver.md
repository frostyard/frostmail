# Plan 0006: M4 daily driver

M4 makes Frostmail the user's only mail client for a week: a personal Gmail
account (OAuth with the user's own client, app password as a fallback) and
an iCloud account, alongside the generic test account. It adds the secret
store, provider profiles and discovery, the Gmail sync path, read-only
accounts for a cautious rollout, a safety check against the server, the
search language, notifications, unified mailboxes, settings, and an
install that runs maild as a user service. It follows
[M3](0005-m3-compose-and-send.md) in the [roadmap](0002-roadmap.md).
Microsoft accounts are not in the user's set; their profile waits (Later).

## Decisions taken for M4

- [ADR-0011](../adr/0011-sign-in-and-credentials.md): sign-in is
  `password` or `oauth2` (XOAUTH2); the user registers their own Google
  client until M6; maild runs the PKCE loopback flow and refreshes tokens;
  secrets move to the Secret Service; provider profiles carry servers and
  quirks.
- [ADR-0012](../adr/0012-gmail-labels-as-memberships.md): Gmail syncs All
  Mail, Spam and Trash keyed by `X-GM-MSGID`; every other mailbox is a label
  whose memberships come from `X-GM-LABELS`; operations are label edits and
  moves to Trash or Spam.
- [design/accounts.md](../design/accounts.md): profiles, discovery,
  sign-in, secrets, read-only accounts, the safety check, recordings and
  replay, and the user's setup steps.
- [design/desktop.md](../design/desktop.md): `make install`, the user
  service, notifications, one app instance.
- [specs/search.md](../specs/search.md): the search language.
- **Rollout (the user's choice):** throwaway accounts first (a spare Gmail
  account; recorded sessions become replay tests), then the real accounts
  read-only for a day, then read-write.
- **API, protocol 1, additive:** `account.readOnly`, `account.notify`,
  auth kind `oauth2`, `account.authorize`, `account.discover`,
  `account.verify`, `oauth.setClient`, `oauth.getClient`.

## The user's setup (needed by Phase 4)

1. A throwaway Gmail account, with 2-Step Verification and an app password
   (to test the fallback).
2. A personal Google OAuth client (Desktop app, consent screen published "In
   production", scope `https://mail.google.com/`), as in
   [design/accounts.md](../design/accounts.md#setting-up-a-personal-google-client).
3. An iCloud app-specific password for the real iCloud account (used
   read-only first).

## Phase 1 — Contracts and foundations (planner)

- ADRs 0011 and 0012, design/accounts.md, design/desktop.md, specs/search.md,
  this plan.
- Schema and `make gen`; migration 0004 (`accounts.read_only`,
  `accounts.notify`, `oauth_clients`, auth kind `oauth2`); engine stubs.
- Package skeletons with stubs: `internal/search`, `internal/discover`,
  `internal/oauth`, `internal/notify`, `internal/providers`.
- Cards with stubs and given tests, each checked against a reference
  implementation.
- **Done when:** `make check` and `make ui-check` are green and the cards
  are ready.

## Phase 2 — Executor batch (runs alongside Phase 3)

| Card | Area | What |
| --- | --- | --- |
| T-0048 | `internal/search` | Parse the search language: tokens, operators, dates, Match and Exclude |
| T-0049 | `internal/discover` | Read Mozilla autoconfig XML into server settings |
| T-0050 | `internal/discover` | Read RFC 6186 SRV records into server settings |
| T-0051 | `internal/oauth` | PKCE, the authorization URL, token responses, the XOAUTH2 response and its errors |
| T-0052 | `internal/notify` | Notification texts and grouping |
| T-0053 | `cmd/mailctl` | `mailctl oauth`, `mailctl account connect`, `mailctl account authorize` |
| ~~T-0054~~ | `lib/mailboxTree` | Unified mailboxes: done by the planner with the source type it needed (no card) |
| T-0055 | `features/settings` | `AccountList` |
| T-0056 | `features/settings` | `ServerFields` |
| T-0057 | `features/settings` | `AccountForm`: address, discovery, sign-in, servers, read-only, notifications |
| T-0058 | `features/settings` | `IdentityEditor` |
| T-0059 | `features/settings` | `OAuthClientForm` |

- **Done when:** every card is merged with `make check` and `make ui-check`
  green.

## Phase 3 — Engine and app (planner)

- **Secrets:** the Secret Service store, the choice and the migration from
  `secrets.json`.
- **Sign-in:** OAuth client storage, `account.authorize` with the loopback
  listener, the token manager, XOAUTH2 in `imapx` and `smtpx`, unauthorized
  handling; app passwords for Gmail.
- **Providers:** profiles, `account.discover` over the T-0049/T-0050
  readers, iCloud's quirks in sync.
- **Gmail:** the sync path of ADR-0012 (All Mail, Spam, Trash, labels,
  `X-GM-THRID` threads, IDLE on All Mail) and its operations.
- **Safety:** read-only accounts; `account.verify`.
- **Search:** the parser wired into the store, measured on a 200,000-message
  fixture.
- **Desktop:** notifications from maild, one app instance with
  `--open-message`, `make install` with the user service and desktop entry.
- **App:** a settings window (accounts, Google client, identities and
  signatures), the add-account flow with discovery and Google sign-in,
  unified mailboxes, "Sign in again", read-only display.
- **Recordings:** the trace format, `tools/imaprec` and the replay server.
- **Done when:** the generic test account, a Gmail replay test and an
  iCloud replay test pass, and `make install` runs Frostmail on the user's
  desktop against the test account.

## Phase 4 — Throwaway trials

- The throwaway Gmail account in Frostmail, read-write: OAuth sign-in and
  the app-password fallback; initial sync; labels, archive, move, delete,
  junk, flags both from Frostmail and from Gmail's web UI; sending, drafts,
  search, notifications; `mailctl verify` after each session.
- Its recorded sessions, sanitized, become replay tests.
- The real iCloud account read-only: sync, search, notifications,
  `mailctl verify`; recordings as above.
- **Done when:** every operation behaved as in ADR-0012 on Gmail, verify
  reports no difference, and the replay tests are in `make check`.

## Phase 5 — Real accounts

- The real Gmail and iCloud accounts read-only for a day, then read-write.
- Seven days as the user's only client, with `mailctl verify` daily and
  every surprise logged as an issue.

## Phase 6 — Exit evidence

- **Done when**, each shown by a test or a recorded run:
  1. Gmail (personal) and iCloud were the user's only client for 7 days;
     daily `mailctl verify` found no lost or misplaced mail.
  2. Gmail sign-in with the user's OAuth client survived the week without
     signing in again; the app-password fallback works.
  3. Search answers in under 150 ms (p95) on the user's mail and on a
     200,000-message fixture.
  4. New mail notifies with the app closed, and clicking opens it.
  5. Gmail and iCloud replay tests pass in `make check`.

### Evidence so far

- **Search, 200,000-message fixture (2026-10-08):** `tools/uifixture -n
  200000 -seed 7`, then `tools/searchbench -runs 20` against maild on the
  Framework (Strix Halo): p95 94.5 ms over nine queries covering words,
  prefixes, phrases, columns, negation, flags, dates and `in:`; the worst
  query (`le`, 101,848 results) has p95 97.7 ms. The user's own mail is
  measured in Phase 5.

- **Throwaway Gmail, first session (2026-10-08):** OAuth sign-in with the
  user's client through `mailctl account connect --oauth`; the refresh
  token in the Secret Service survived a maild restart. Gmail offered
  X-GM-EXT-1, CONDSTORE, MOVE, UIDPLUS, ESEARCH and no QRESYNC; its MODSEQ
  is account-wide. Run from mailctl, each verified with `mailctl verify`
  (no differences) and read back from the trace: star (`STORE +FLAGS` on
  All Mail, Gmail adds `\Starred`), archive (`-X-GM-LABELS \Inbox`), back
  to INBOX (`+X-GM-LABELS \Inbox`), to Spam (`UID MOVE`, COPYUID), not
  junk (`UID MOVE` Spam → INBOX: Gmail restores it to All Mail with
  `\Inbox` under a new UID; same message row), delete (`UID MOVE` to
  Trash) and restore. Found: verify leaked an empty trace file (fixed).
- **Throwaway Gmail, with the user (2026-10-08):** the app (local
  Flatpak) opened; mail from the user's other address landed in Gmail's
  Spam and "not junk" from the app moved it to INBOX; a reply went out
  through Gmail's SMTP with XOAUTH2 and Gmail filed it in Sent (nothing
  appended); an open draft's copies were appended to Drafts and the
  replaced ones moved to Trash and expunged there. A label and an archive
  made in the web UI synced; a web label applies to the whole
  conversation. Found: Gmail's IDLE reports no label changes (the label
  arrived with the 5-minute poll, which then skipped All Mail on Gmail);
  fixed by polling All Mail, Spam and Trash every minute on Gmail.
  New mail arrived through IDLE on All Mail within seconds; maild
  announced it on GNOME's desktop and a click opened the message in the
  running app (single instance). Found: notifications named the wrong
  desktop entry and icon, and maild accepted click signals from any
  program on the session bus (both fixed); a click on a notification sent
  by a maild that has since restarted does nothing.

## Later / ideas

- Microsoft (Outlook.com, Microsoft 365) profile and OAuth: when an
  account needs it; tenant admin consent documented.
- Gmail categories (Primary, Promotions) via `X-GM-RAW`: M5 spike.
- Contacts beyond seen addresses (CardDAV): after M6.

## References

- Implements: [ADR-0011](../adr/0011-sign-in-and-credentials.md),
  [ADR-0012](../adr/0012-gmail-labels-as-memberships.md),
  [design/accounts.md](../design/accounts.md),
  [design/desktop.md](../design/desktop.md),
  [specs/search.md](../specs/search.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
