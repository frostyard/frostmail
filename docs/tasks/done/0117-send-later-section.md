---
id: "0117"
title: The Send Later section in the main window
milestone: M5
size: M
touch:
  - app/src/features/outbox/Outbox.tsx
  - app/src/app/OutboxContainer.tsx
  - app/src/app/SidebarContainer.tsx
given:
  - app/src/features/outbox/Outbox.later.test.tsx
  - app/src/app/SendLater.test.tsx
acceptance: make ui-vitest F="src/features/outbox src/app/SendLater src/app/Sidebar"
---
# T-0117: The Send Later section in the main window

## Goal

Messages waiting for their Send Later time list in their own sidebar
section, soonest first, with Edit, Send Now and Change Time…. They show no
undo toast, and the Outbox leaves them out.

## Read first

- `docs/specs/ui.md`: "Sidebar", the Send Later bullet;
  `docs/specs/compose-ui.md`: "Saving and sending".
- `app/src/features/outbox/Outbox.tsx` (`OutboxStatus`, `UndoToast`) and
  `app/src/app/OutboxContainer.tsx` (`UndoToasts`, `OutboxSection`,
  `undo`, `useNow`), `app/src/app/SidebarContainer.tsx`.
- `app/src/lib/later.ts` (`whenText`) and
  `app/src/features/later/TimeSheet.tsx`.
- `app/src/rpc/gen/api.ts`: `OutboxItem.scheduled`, `outbox.reschedule`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`Outbox.tsx`** exports `isSendLater(item)` (queued and `scheduled`)
  and `SendLaterStatus` with `SendLaterStatusProps` (`items`, `now`,
  `timeZone`, `locale`, `onEdit(item)`, `onSendNow(item)`,
  `onChangeTime(item)`): nothing without a waiting message, else a
  `section` named "Send Later" with an `h2` "Send Later" and a list in
  `OutboxStatus`'s look, soonest `sendAt` first. Each row has the subject
  (`font-semibold`), "To: " and the recipients (`formatAddressList(to,
  2)`), "Sends " and `whenText(sendAt, now, timeZone, locale)`, and the
  buttons Edit, Send Now and Change Time…. `OutboxStatus` leaves out what
  `isSendLater` takes.
- **`OutboxContainer.tsx`**: `UndoToasts` shows no toast for a scheduled
  message. A new `SendLaterSection` renders `SendLaterStatus` with the
  mail store's outbox, the system zone and `appLocale(navigator.language)`:
  Edit is the undo path (`outbox.cancel`, then `openCompose` of the
  draft), Send Now is `outbox.reschedule {id, sendAt: <now ISO>}`, and
  Change Time… opens the `TimeSheet` titled "Send Later", starting at the
  message's `sendAt`, whose OK calls `outbox.reschedule {id, sendAt}`.
- **`SidebarContainer.tsx`**: `SendLaterSection` right after
  `OutboxSection`, in the same scrolling box.

## Tests (given, do not edit)

`Outbox.later.test.tsx` and `SendLater.test.tsx`. Every other app test
must keep passing, `Outbox.test.tsx` included.

## Gotchas

- A scheduled message's `sendAt` can be days away; the undo toast's
  countdown must not take it for an undo window.
- After Send Now, maild (and the mock) make the message an ordinary send
  in its undo window: it leaves Send Later and shows the undo toast.

## Out of scope

The compose window (T-0116), Remind Me (T-0118), maild, the mock, and every
file not under `touch`.

## Done when

`make accept T=0117` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
