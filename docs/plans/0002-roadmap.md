# Plan 0002: roadmap to Mail.app parity

Each milestone gets its own plan with task cards before it starts. Planner
work stays with Claude; executor work goes on cards
([agent-workflow](../design/agent-workflow.md)).

| Milestone | Planner | Executor | Done when |
| --- | --- | --- | --- |
| **M1 Headless read path** (one generic IMAP account; [plan 0003](0003-m1-headless-read-path.md); done 2026-10-07) | account actor and scheduler, folder sync, the CONDSTORE reconcile pass, UIDVALIDITY handling, offline-op replay core, JWZ threading, view snapshots | store queries, MIME helpers against golden tests, blob store, FTS indexer, `mailctl sync/ls/show/search/flag/mv` | 50k messages synced from Dovecot; external flag changes, expunges and new mail appear within 5 s; `kill -9` during sync resumes cleanly; at least 150 MIME fixtures pass |
| **M2 UI read path** ([plan 0004](0004-m2-ui-read-path.md); done 2026-10-07) | transport and data flow, `view.delta` in the virtual list, HTML sanitizer, reader iframe, remote-content and tracker blocking, `ui-spec.md` (Mail.app measurements and tokens), a dev-mode substitute for `mailpart://` | sidebar, list rows, thread view, toolbar, context menus, keyboard map, Playwright specs | daily reading of the Dovecot account; 100k rows scroll smoothly; the hostile-HTML suite makes zero requests |
| **M3 Compose and send** ([plan 0005](0005-m3-compose-and-send.md); done 2026-10-07) | outbox state machine with undo delay, crash-safe sending, Sent-copy detection, drafts | TipTap compose, address autocomplete, attachments, reply and forward quoting, signatures | mail sent through Postfix lands in Dovecot; offline sends go out on reconnect; 25 MB attachments; drafts survive a restart |
| **M4 Daily driver** ([plan 0006](0006-m4-daily-driver.md)) | Google OAuth (the user's client), Secret Service, the Gmail strategy, provider profiles and quirks, read-only accounts and the safety check, the search language, notifications, the user service | discovery (autoconfig, ISPDB, SRV), search parser, settings, `mailctl oauth` and `verify`, unified mailboxes | Gmail and iCloud are the user's only client for 7 days with no data loss; search under 150 ms (Microsoft waits for an account that needs it) |
| **M4.5 People, calendar and tasks** ([plan 0007](0007-m4.5-people-calendar-tasks.md); runs during M4's trial week) | CardDAV and CalDAV sync (`davx`, `davtest`), Google Tasks, stored sources with surgical edits, occurrences and time zones, invitations through server scheduling or iTIP, reminders, contracts and given tests for every card | (Codex, [ADR-0021](../adr/0021-run-task-cards-with-codex.md)) parsers, the people, calendar and tasks modules, the To-Do bar, contact cards, the invitation card, the reminder window, `mailctl` commands | the user's contacts, calendars and Google Tasks in Frostmail with clean verifies; invitations answered from the reader with exactly one reply; reminders on time with the app closed |
| **M5 Mail.app parity** ([plan 0008](0008-m5-mail-app-parity.md); runs during M4's trial week) | rules, the smart-mailbox compiler, Remind Me and Send Later scheduling | VIP, flag colors (`$MailFlagBit0-2`), rule and smart-mailbox editors, undo-send, send-later and remind-me UI | a parity checklist with a test per item; a categories spike |
| **M6 Packaging** | Flatpak with portals (secrets, background, OpenURI, notifications), Google and Microsoft verification | AppStream metadata, icons, docs, CI | clean installs on Silverblue and Debian; Google verification approved, or bring-your-own client ID documented |

## Top risks

1. go-imap is beta and forked: pinned, small patch set, everything behind
   `imapx`, replay tests.
2. Gmail and Microsoft OAuth policy: app passwords or an unverified
   production app for personal use; start verification early in M5; document
   tenant admin consent.
3. WebKitGTK regressions on AMD: `FROSTMAIL_WEBKIT_SAFE`, and the spike rerun
   on each WebKitGTK update.
4. HTML security: three independent layers ([ADR-0005](../adr/0005-html-mail-rendering.md)).
5. Executor drift: contract tests first, protected given files, small cards,
   milestone reviews.
