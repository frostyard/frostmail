# Spec: the Mail.app parity checklist

What "Mail.app parity" means for Frostmail's mail: every feature of Mail
in macOS 26 Tahoe, one row each, with Frostmail's equivalent, its status,
and the test that shows it. M5 ([plan 0008](../plans/0008-m5-mail-app-parity.md))
ends when this list holds no M5 row and every Have row names its test, as
the [roadmap](../plans/0002-roadmap.md) requires. Calendar, contacts and
tasks are not Mail's and are not listed; their parity is
[plan 0007](../plans/0007-m4.5-people-calendar-tasks.md)'s.

## Status

| Status | Meaning |
| --- | --- |
| Have | Built. The Test column names the test that fails if it breaks. |
| M5·n | M5 builds it in phase n of plan 0008, with its test. |
| Later | After M5; the reason says why it can wait. |
| Out | Not planned; the reason says why. |

## Rules

- Every row has a status, and Later and Out rows a reason.
- A row becomes Have only in the change that adds or names its test.
- Mail.app's ⌘ is Ctrl and ⌥ is Alt; a shortcut Frostmail already uses
  for something else keeps Frostmail's meaning and is noted.
- A Mail.app feature found missing from the list is added with a status,
  not dropped.

## Mailboxes and the sidebar

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-101 | Favorites: All Inboxes and unified mailboxes per role | Favorites: All Inboxes, All Drafts, Sent, Junk, Trash, Archives | Have | `TestViewRoleAndThreads`, `mailboxTree.test.ts` (unified mailboxes) |
| P-102 | Add a mailbox to Favorites, reorder, remove | Same, kept by maild | M5·6 | |
| P-103 | Flagged, with a mailbox per flag color in use | Flagged with a row per color in use, counted | Have | `mailboxTree.sources.test.ts`, `SidebarSources.test.tsx` |
| P-104 | VIPs, with a mailbox per VIP | Same | Have | `SidebarSources.test.tsx`, `TestVIPs` |
| P-105 | Smart Mailboxes: any or all of a list of conditions; include Trash, include Sent | Same, compiled to the search SQL, live as views | Have | `TestSmartMailboxViews`, `TestSmartMailboxAcrossAccounts`, `TestSmartMailboxes`, `SmartMailboxes.test.tsx`, `ConditionEditor.test.tsx` |
| P-106 | Smart mailbox folders | Groups of smart mailboxes | Later | Grouping; few smart mailboxes in practice |
| P-107 | New, rename, delete and move mailboxes | Same, as offline ops (labels on Gmail) | M5·6 | |
| P-108 | Drag messages to a mailbox (Alt: copy) | Same; Ctrl copies too | M5·6 | |
| P-109 | Use This Mailbox For Drafts, Sent, Junk, Trash, Archive | Override a role per account | M5·6 | |
| P-110 | Erase Deleted Items, Erase Junk Mail | Same, after a confirmation | M5·6 | |
| P-111 | On My Mac (local mailboxes) | | Later | Every mailbox is on a server today; needs a local-only kind |
| P-112 | Unread counts in the sidebar | Same, inboxes summed in All Inboxes | Have | `mailboxTree.test.ts` |
| P-113 | The unread count on the app's icon (the Dock badge) | | Later | Only some desktops show a launcher badge (Unity's LauncherEntry) |

## The message list

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-201 | Organize by Conversation, with a count | Conversation mode, with a badge | Have | `TestViewRoleAndThreads`, `MessageRow.test.tsx` |
| P-202 | Turn conversations on and off | A View toggle (the state exists, unset) | M5·6 | |
| P-203 | Sort by Date, From, To, Subject, Size, Flags, Unread, Attachments; ascending or descending | Same, in the view query | M5·6 | |
| P-204 | Filter: Unread, Flagged, Attachments | The filter bar | Have | `FilterBar.test.tsx`, `ListFilter.test.tsx` |
| P-205 | Filter: To: Me, Cc: Me, Only from VIPs | More filter bar choices, as conditions | Have | `FilterBar.more.test.tsx`, `stores.filter.more.test.ts`, `ListFilter.more.test.tsx` |
| P-206 | Flag color in the row | Same | Have | `MessageRow.test.tsx` |
| P-207 | Actions on a row (swipe; Frostmail: hover) | Flag, Archive, Delete | Have | `MessageRow.actions.test.tsx`, `RowActions.test.tsx` |
| P-208 | VIP star on the sender | Same | Have | `MessageRow.vip.test.tsx`, `VipStar.test.tsx` |
| P-209 | Contact photos in the list | People photos, never fetched | M5·6 | |
| P-210 | Business logos in the list | | Out | Fetched from Apple; the app never fetches remote content |
| P-211 | List preview lines (none to 5) | | Later | A setting over a fixed row height |
| P-212 | Classic layout with columns | | Later | Deferred since M2 (plan 0004) |
| P-213 | Highlight a selected conversation's other messages | | Later | Cosmetic |

## The reader

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-301 | The whole conversation, related messages included | Same, with Show Earlier Messages | Have | `TestThreadMessages` |
| P-302 | Remote content blocked until asked | Blocked, loaded per message | Have | `remote.e2e.ts`, `hostile.e2e.ts` |
| P-303 | Mail Privacy Protection (proxied loads) | | Out | Frostmail blocks instead of proxying (ADR-0005) |
| P-304 | Raw Source, All Headers | Same, from the blob store | M5·6 | |
| P-305 | Unsubscribe banner (List-Unsubscribe) | After a confirmation: RFC 8058 one-click `POST` by maild, else mailto through the outbox, else the page in the browser ([ADR-0027](../adr/0027-unsubscribe-with-one-click.md)) | M5·6 | |
| P-306 | Remind Me banner and clock | Same | Have | `ReminderBanner.test.tsx`, `MessageRow.remind.test.tsx`, `RemindMe.test.tsx` |
| P-307 | Print | Same | M5·6 | |
| P-308 | Save As (raw source) and Export as PDF | Save the .eml; PDF through Print | M5·6 | |
| P-309 | Find in the message | | Later | Ctrl+F focuses search today; needs a find bar in the frame |
| P-310 | Collapse and expand messages in a conversation | | Later | Quotes already collapse (`PlainText.test.tsx`) |
| P-311 | Calendar invitations in mail | The invitation card | Have | `Invitation.test.tsx`, `Invitation.once.test.tsx` |
| P-312 | Add the sender to VIPs, to Contacts | VIPs; Add to Contacts | Have | `ContactPopover.vip.test.tsx`, `VipStar.test.tsx`, `ContactCard.test.tsx` |

## Acting on messages

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-401 | Reply, Reply All, Forward | Same | Have | `TestReplyRecipients`, `TestSubjects`, `ComposeWindow.test.tsx` |
| P-402 | Redirect | A resend with the original headers, as maild sends | M5·6 | |
| P-403 | Forward as Attachment | Same | M5·6 | |
| P-404 | Archive, Delete, Move, Copy | Same, as offline ops | Have | `TestDovecotMoveAndDelete`, `TestCopyBetweenFolders`, `TestGmailArchiveAndMoveBetweenLabels` |
| P-405 | Move to Junk, Not Junk | Mark as Spam, Not Spam | Have | `TestGmailJunkAndBack`, `ListMenu.test.tsx` |
| P-406 | Undo a move, delete or flag (⌘Z) | Ctrl+Z replays the inverse op | M5·6 | |
| P-407 | Flag with seven colors, Clear Flag | Same (`$MailFlagBit0–2`) | Have | `TestFlagsRoundTrip`, `TestFlagChangesReplayToServer` |
| P-408 | Rename flags | Names kept by maild | Have | `GeneralPane.test.tsx`, `FlagNames.test.tsx`, `TestSettings` |
| P-409 | Mark as Read, Unread | Same | Have | `TestFlagChangesReplayToServer` |
| P-410 | Mute a conversation | Same; a muted conversation never notifies | M5·6 | |
| P-411 | Block a sender: mark blocked, or move to Trash | Same, kept by maild | M5·6 | |
| P-412 | Remind Me: 1 hour, tonight, tomorrow, a chosen time | Same, fired by maild, back to the top of the inbox | Have | `TestRemindMe`, `later.test.ts`, `RemindMe.test.tsx` |
| P-413 | Follow Up: sent mail with no reply after three days returns | | Later | A suggestion heuristic; after Remind Me |
| P-414 | Apply Rules to the selection | Same, from the list's menu (no shortcut: Ctrl+Alt+L locks the screen) | Have | `TestApplyRules`, `ListMenu.rules.test.tsx`, `TestRulesApplyCommand` |
| P-415 | Move to the predicted mailbox | | Out | On-device learning |

## Writing and sending

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-501 | Compose with formatting, attachments, inline images | Same | Have | `compose.e2e.ts`, `ComposeAttachments.test.tsx` |
| P-502 | Identities and a signature per account | Same | Have | `IdentityEditor.test.tsx`, `SettingsWindow.identities.test.tsx` |
| P-503 | Several signatures per account, chosen in compose | | Later | One per identity covers the user |
| P-504 | Address completion, contacts first | Same | Have | `RecipientField.test.tsx`, `TestSuggestContactsFirst` |
| P-505 | Undo Send, with a delay of Off, 10, 20 or 30 seconds | The toast; the delay a setting in the General pane | Have | `TestUndoCancelsASend`, `TestUndoDelayFollowsTheSetting`, `GeneralPane.test.tsx`, `Outbox.test.tsx` |
| P-506 | Send Later, with its mailbox, to edit or cancel | An outbox row due at the chosen time, dated then; a Send Later section | Have | `TestSendLater`, `TestSendLaterCanBeEdited`, `ComposeWindow.later.test.tsx`, `SendLater.test.tsx` |
| P-507 | Message priority | | Later | Rarely used |
| P-508 | Reply quoting the selected text | | Later | Needs the frame's selection |
| P-509 | Mail Drop, Hide My Email, stationery | | Out | iCloud services and Apple's templates |
| P-510 | Writing Tools, Smart Reply, summaries | | Out | Apple Intelligence |

## Search

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-601 | Search all mailboxes or the current one | The search language and scope bar | Have | `TestViewSearchLanguage`, `TestParseTerms`, `SearchField.test.tsx` |
| P-602 | Save a search as a smart mailbox | Same | Have | `TestSearchAsConditions`, `TestSmartMailboxes`, `SmartMailboxes.test.tsx` |
| P-603 | Search suggestions as tokens (People, Subjects) | | Later | The language covers the operators |
| P-604 | Search the server beyond what is stored | | Later | Plan 0006 Later: mail outside the sync window |

## Rules

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-701 | Rules: any or all conditions, actions in order, Stop Evaluating Rules, the list's order | Same, run by maild once on new inbox mail before it notifies | Have | `TestRulesActOnNewInboxMail`, `TestRulesRunOnce`, `TestDovecotRules`, `TestGmailRulesMoveToALabel`, `RulesPane.test.tsx`, `SettingsWindow.rules.test.tsx` |
| P-702 | Conditions on From, To, Cc, any recipient, Subject, content, dates, account, VIP, Contacts, previous recipients, attachments, junk | The smart mailbox conditions; junk is Mailbox Type is Junk; no Previous Recipients list | Have | `TestCompileEveryCondition`, `RuleSheet.test.tsx` |
| P-703 | Move, Copy, Mark as Read, Mark as Flagged (color), Delete, Send Notification | Same | Have | `TestRulesActOnNewInboxMail`, `TestDovecotRules`, `ActionEditor.test.tsx`, `TestRules` |
| P-704 | Reply, Forward, Redirect as rule actions | | Later | They send with nobody looking; the user's choice (plan 0008) |
| P-705 | Set Color, Play Sound, Bounce Icon | | Later | Cosmetic |
| P-706 | Run AppleScript | | Out | macOS only; a command action would be a new trust boundary |
| P-707 | Rules shared through iCloud Drive | | Out | Rules, VIPs and smart mailboxes are maild's, on this machine |

## Notifications

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-801 | New mail notifications; clicking opens the message | Same; one per message, a group for four or more | Have | `TestDesktopAnnouncesAndOpens`, `TestNotesGroupFourOrMore` |
| P-802 | Notify for Inbox only, VIPs, Contacts or All Mailboxes | A scope chosen in Settings | Have | `TestNotifyScope`, `GeneralPane.test.tsx` |
| P-803 | Notify for a smart mailbox | The scope may be one | Have | `TestNotifySmartScope`, `GeneralPane.smart.test.tsx`, `SettingsWindow.smart.test.tsx` |
| P-804 | Per account on or off | Same | Have | `TestNotifyOffAnnouncesNothing`, `TestNewMailIsAnnounced` |
| P-805 | Archive, Trash and Reply on the notification | | Later | Notification actions beyond Open |

## Categories

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-901 | Primary, Transactions, Updates, Promotions | Decided by a spike | M5·7 | The spike's ADR |

## Settings, accounts and the rest

| ID | Mail.app | Frostmail | Status | Test |
| --- | --- | --- | --- | --- |
| P-951 | Accounts, servers, sign-in | Same, with discovery and Google sign-in | Have | `SettingsWindow.add.test.tsx`, `AccountForm.test.tsx` |
| P-952 | New accounts are writable | Same; Read only is a checkbox when adding | Have | `SettingsWindow.writable.test.tsx` |
| P-953 | Junk mail filter and its settings | The server's filtering | Out | Gmail and iCloud filter on the server; Frostmail moves and reports |
| P-954 | Connection Doctor, Activity | `mailctl verify`, the sync state in the sidebar | Have | `TestVerifyAfterACleanSync`, `TestGmailVerify` |
| P-955 | Import Mailboxes, Export Mailbox (mbox) | | Later | Not needed by the user's accounts |
| P-956 | Mail.app's shortcuts with Ctrl for ⌘ | The keymap; ⌘1–9 favorites are Ctrl+1 and the modules | M5·6 | Phase 6 adds Redirect, Undo, Print, raw source |
| P-957 | Fonts and Colors, Viewing settings | | Later | Tokens are fixed today |

## References

- Plan: [plans/0008](../plans/0008-m5-mail-app-parity.md),
  [plans/0002](../plans/0002-roadmap.md) (M5's done-when)
- Context: [specs/ui.md](ui.md), [specs/compose-ui.md](compose-ui.md),
  [specs/search.md](search.md), [design/send.md](../design/send.md)
