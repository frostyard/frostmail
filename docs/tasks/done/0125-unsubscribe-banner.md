---
id: "0125"
title: The reader's unsubscribe banner
milestone: M5
size: M
touch:
  - app/src/features/reader/UnsubscribeBanner.tsx
  - app/src/app/ReaderContainer.tsx
given:
  - app/src/features/reader/UnsubscribeBanner.test.tsx
  - app/src/app/Unsubscribe.test.tsx
acceptance: make ui-vitest F="src/features/reader src/app/Unsubscribe src/app/ReaderMore src/app/RemindMe"
---
# T-0125: The reader's unsubscribe banner

## Goal

A banner on mail from a mailing list, as in Mail.app: it names the list
and offers Unsubscribe, which asks first, then leaves the list the best
way the message offers (one click, a message, or the list's page).

## Read first

- `docs/specs/ui.md`: "Reader", the Unsubscribe banner bullet;
  `docs/design/send.md`: "Unsubscribing" (what maild does).
- `app/src/rpc/gen/api.ts`: `message.unsubscribeInfo`,
  `message.unsubscribe`, `Unsubscribe`, `UnsubscribeMethod`,
  `UnsubscribeResult`, `Message.listUnsubscribe`.
- `app/src/features/reader/MessageHeader.tsx` (`RemoteBanner` and
  `ReminderBanner`, drawn as this banner is),
  `app/src/features/later/TimeSheet.tsx` (the sheet frame),
  `app/src/app/ReaderContainer.tsx` (`ConversationMessage`, `openLink`),
  and `app/src/features/settings/labels.ts` (`BUTTON`, `PRIMARY_BUTTON`).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`UnsubscribeBanner`** (`UnsubscribeBanner.tsx`), props `info:
  Unsubscribe`, `method?: UnsubscribeMethod` (the method tried first;
  absent disables the button), `busy: boolean`, `failed: boolean`,
  `onUnsubscribe()`:
  - With `info.done`: a `role="status"` banner (`bg-banner`, 12px, as
    `ReminderBanner`) whose text is exactly "You unsubscribed from
    <list>." and no button.
  - Else with no methods: nothing.
  - Else: the banner says "This message is from the mailing list
    <list>.", followed by " Couldn't unsubscribe." when `failed`, with a
    button at the right, "Unsubscribe" ("Unsubscribing…" when `busy`),
    disabled when `busy` or without `method`.
  - The button opens a dialog (TimeSheet's frame, 360 wide, outside the
    status element) named "Unsubscribe from <list>?" with the spec's
    sentence for `method` (the web sentence names the host of
    `info.url`), Cancel, and Unsubscribe, which has focus. Unsubscribe
    closes it and calls `onUnsubscribe`; Cancel and Escape close it.
    Key events stop at the dialog.
- **`ReaderContainer`**, per message:
  - When the message's `listUnsubscribe` is not empty,
    `message.unsubscribeInfo {id}`, shown in the banner after the
    reminder banner (before the invitation card and the remote content
    banner). Never call it for a message whose `listUnsubscribe` is
    empty.
  - The methods are `info.methods` without `mail` on a read-only account;
    the first is the banner's `method`.
  - On `onUnsubscribe`: busy; `message.unsubscribe {id, method}` for each
    method in order until one succeeds; a result's `url` opens with
    `openLink`. After a success, `message.unsubscribeInfo` again and show
    it; when all failed, `failed` until the next try.

## Tests (given, do not edit)

`UnsubscribeBanner.test.tsx`, `Unsubscribe.test.tsx`. Every other app
test must keep passing.

## Gotchas

- Outside Tauri, `openLink` calls `window.open(url, "_blank",
  "noopener,noreferrer")`; the test spies on it. Use `openLink`.
- Keep `useState` calls above the banner's early returns.
- A failed method only logs (`console.warn`); go on to the next.

## Out of scope

maild, the mock, the list's menu, and every file not under `touch`.

## Done when

`make accept T=0125` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
