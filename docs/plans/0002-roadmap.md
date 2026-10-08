# Plan 0002: roadmap to Mail.app parity

Each milestone gets its own plan with task cards before it starts. Planner
work stays with Claude; executor work goes on cards
([agent-workflow](../design/agent-workflow.md)).

| Milestone | Planner | Executor | Done when |
| --- | --- | --- | --- |
| **M1 Headless read path** (one generic IMAP account; [plan 0003](0003-m1-headless-read-path.md); done 2026-10-07) | account actor and scheduler, folder sync, the CONDSTORE reconcile pass, UIDVALIDITY handling, offline-op replay core, JWZ threading, view snapshots | store queries, MIME helpers against golden tests, blob store, FTS indexer, `mailctl sync/ls/show/search/flag/mv` | 50k messages synced from Dovecot; external flag changes, expunges and new mail appear within 5 s; `kill -9` during sync resumes cleanly; at least 150 MIME fixtures pass |
| **M2 UI read path** ([plan 0004](0004-m2-ui-read-path.md); done 2026-10-07) | transport and data flow, `view.delta` in the virtual list, HTML sanitizer, reader iframe, remote-content and tracker blocking, `ui-spec.md` (Mail.app measurements and tokens), a dev-mode substitute for `mailpart://` | sidebar, list rows, thread view, toolbar, context menus, keyboard map, Playwright specs | daily reading of the Dovecot account; 100k rows scroll smoothly; the hostile-HTML suite makes zero requests |
| **M3 Compose and send** ([plan 0005](0005-m3-compose-and-send.md)) | outbox state machine with undo delay, crash-safe sending, Sent-copy detection, drafts | TipTap compose, address autocomplete, attachments, reply and forward quoting, signatures | mail sent through Postfix lands in Dovecot; offline sends go out on reconnect; 25 MB attachments; drafts survive a restart |
| **M4 Daily driver** | Google and Microsoft OAuth, Secret Service, Gmail strategy, provider quirk profiles, multi-account scheduling, unified views, the search language, notifications | account wizard (autoconfig, ISPDB, SRV), settings, `mailctl auth`, replay tests from recordings | Gmail, Microsoft 365/Outlook, iCloud and a generic account are the user's only client for 7 days with no data loss; search under 150 ms |
| **M5 Mail.app parity** | rules, the smart-mailbox compiler, Remind Me and Send Later scheduling | VIP, flag colors (`$MailFlagBit0-2`), rule and smart-mailbox editors, undo-send, send-later and remind-me UI | a parity checklist with a test per item; a categories spike |
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
