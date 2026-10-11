---
id: "0118"
title: Remind Me in the list's menu, the row and the reader
milestone: M5
size: L
touch:
  - app/src/app/ListContainer.tsx
  - app/src/app/commands.ts
  - app/src/features/list/MessageRow.tsx
  - app/src/features/reader/MessageHeader.tsx
  - app/src/app/ReaderContainer.tsx
given:
  - app/src/features/list/MessageRow.remind.test.tsx
  - app/src/features/reader/ReminderBanner.test.tsx
  - app/src/app/RemindMe.test.tsx
  - app/src/app/ListMenu.test.tsx # T-0103's, with Remind Me after Mark as Read
  - app/src/app/ListMenu.rules.test.tsx # T-0113's, the same
acceptance: make ui-vitest F="src/app/ListMenu src/app/RemindMe src/features/list src/features/reader"
---
# T-0118: Remind Me in the list's menu, the row and the reader

## Goal

Remind Me as in Mail.app. The list's context menu sets a reminder: in an
hour, tonight, tomorrow, or a chosen time. A row with a pending reminder
shows a clock, and the reader shows a banner that changes or clears it.
maild brings the message back to the top of its inbox at that time.

## Read first

- `docs/specs/ui.md`: "Context menu" (Remind Me), "Message list" (line 1's
  clock), "Reader" (the reminder banner), "Times (M5)".
- `app/src/app/ListContainer.tsx` (`menuItems`, `onMenuSelect`) and
  `app/src/app/commands.ts`.
- `app/src/features/list/MessageRow.tsx` (line 1),
  `app/src/features/reader/MessageHeader.tsx` (`RemoteBanner`), and
  `app/src/app/ReaderContainer.tsx` (`ConversationMessage`, its
  `useCalendarFrame` zone and locale).
- `app/src/lib/later.ts` (`remindChoices`, `whenText`) and
  `app/src/features/later/TimeSheet.tsx`.
- `app/src/rpc/gen/api.ts`: `message.remind`, `MessageSummary.remindAt`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`commands.ts`**: `remindMessages(client, ids, at?: Date)` calls
  `message.remind {ids, at: at.toISOString()}`, or `{ids}` without `at`
  (clearing); nothing for no IDs.
- **The menu** (`ListContainer.tsx`): after Mark as Read or Mark as
  Unread, a submenu "Remind Me" (disabled on a read-only account, as the
  others are) holding an item per `remindChoices(new Date(), <system
  zone>)` (its label), a separator, "Remind Me Later…", and "Clear
  Reminder" when a message of the menu's set has a `remindAt`. A choice
  calls `remindMessages` with its time; "Clear Reminder" without one;
  "Remind Me Later…" opens the `TimeSheet` titled "Remind Me", starting
  at the last choice's time, whose OK calls `remindMessages` with the
  chosen time for the menu's messages.
- **`MessageRow`**: when `message.remindAt` is set, a 12px `AlarmClock`
  in the row's secondary color between the paperclip and the date,
  wrapped in a `span` with `role="img"` whose name and `title` are
  "Reminder " and `whenText(remindAt, now, <system zone>,
  appLocale(navigator.language))`.
- **`MessageHeader.tsx`** exports `ReminderBanner` (props `when`,
  `disabled?`, `onChange()`, `onClear()`): a `role="status"` strip drawn as
  `RemoteBanner` is (`bg-banner`, 12px), "Remind Me: " and `when`, then
  the buttons "Change…" and "Clear" in `RemoteBanner`'s button style, both
  disabled when `disabled`.
- **The reader** (`ReaderContainer.tsx`, per message): while the message
  has a reminder, `ReminderBanner` between the header and the remote
  content banner, with `whenText` in the reader's zone and locale, and
  `disabled` on a read-only account. Change… opens the `TimeSheet` titled
  "Remind Me" starting at the reminder; its OK calls `message.remind
  {ids: [id], at}`. Clear calls `message.remind {ids: [id]}`. The banner
  follows `message.changed` for its message (refetching
  `message.summaries`), so a reminder set or cleared from the menu shows.

## Tests (given, do not edit)

`MessageRow.remind.test.tsx`, `ReminderBanner.test.tsx`, `RemindMe.test.tsx`,
and T-0103's `ListMenu.test.tsx` and T-0113's `ListMenu.rules.test.tsx`,
each with "Remind Me" after Mark as Read. Every other app test must keep
passing.

## Gotchas

- The list's rows re-render from the view, which maild (and the mock)
  update on `message.changed`; the row's clock needs no state of its own.
- Listen for events with `client.transport.onEvent`, which returns the
  unsubscribe: return it from the effect.
- The system zone is `Intl.DateTimeFormat().resolvedOptions().timeZone`.

## Out of scope

Firing reminders (maild's), a Remind Me mailbox in the sidebar, maild, the
mock, and every file not under `touch`.

## Done when

`make accept T=0118` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
