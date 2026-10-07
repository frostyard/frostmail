---
id: "0031"
title: Draw the reader header, attachments and remote banner
milestone: M2
size: M
touch:
  - app/src/features/reader/MessageHeader.tsx
given:
  - app/src/features/reader/MessageHeader.test.tsx
acceptance: make ui-vitest F=src/features/reader/MessageHeader.test.tsx
---
# T-0031: Draw the reader header, attachments and remote banner

## Goal

Each message in the reader shows a Mail.app header (avatar, sender, date,
subject, recipients), a banner when remote content was held back, and its
attachments as chips (`docs/specs/ui.md`, Reader). Implement the three
presentational components in `MessageHeader.tsx`.

## Read first

- `app/src/features/reader/MessageHeader.tsx`: the three props types and
  stubs.
- `docs/specs/ui.md`, "Reader" (Header, Remote content banner,
  Attachments) and the `--avatar-N` tokens.
- `app/src/lib/format.ts`: `displayName`, `formatAddressList`,
  `formatHeaderDate`, `formatSize`, `initials`, `avatarTone`.
- `app/src/rpc/gen/api.ts`: `Message`, `Part`.
- The given test `app/src/features/reader/MessageHeader.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names; replace the stubs' doc comments and the file's
"Task T-0031 …" sentence with descriptions of the components.

**`MessageHeader({ message })`**: a `header` with `flex gap-3 px-5 py-4`:

- the avatar: a `div` with `aria-hidden="true"`, classes `size-10 shrink-0
  rounded-full flex items-center justify-center text-[13px] font-semibold
  text-white` and `bg-avatar-<n>` for `n = avatarTone(from.address)` (a
  lookup of the eight full class names), showing `initials(from)`;
- a `div` with `min-w-0 flex-1 select-text` holding:
  - line 1 (`flex items-baseline gap-2`): the sender, `displayName(from)`, in
    a `span` with `text-reader-sender truncate flex-1` and
    `title={from.address}`; the date,
    `formatHeaderDate(new Date(summary.date))`, in a `span` with
    `text-reader-meta text-secondary shrink-0`;
  - the subject in a `div` with `text-reader-subject`, or `(No Subject)`
    when it is blank;
  - when `to` is not empty, a `div` with `text-reader-meta text-secondary
    truncate` reading `To: ` + `formatAddressList(to)`;
  - the same for `cc` with `Cc: `.

**`AttachmentStrip({ parts, onOpen })`**: the attachments are the parts
whose `disposition` is `"attachment"`, or whose `filename` is not empty and
whose disposition is not `"inline"`, in order. None: render `null`.
Otherwise a `div` with `role="list"`, `aria-label="Attachments"`, classes
`flex flex-wrap gap-2 px-5 pb-4`; per attachment a `div` with
`role="listitem"` holding a `button` (`type="button"`, `title` = the
filename, classes `flex h-8 items-center gap-2 rounded-md bg-banner px-2`)
with a Lucide `File` (16px, `text-secondary`), the filename (`Untitled` when
empty) in a `span` with `max-w-[200px] truncate text-[12px]`, and
`formatSize(part.size)` in a `span` with `text-[12px] text-secondary
tabular-nums`. Clicking calls `onOpen(part)`.

**`RemoteBanner({ remote, trackers, loading, onLoad })`**: `null` when both
counts are 0. Otherwise a `div` with `role="status"`, classes `flex
items-center gap-2 bg-banner px-5 py-2 text-[12px]`, containing one `span`
whose text is the sentences that apply, joined by a space:
`This message contains remote content.` when `remote > 0`, and
`N trackers blocked.` (`1 tracker blocked.` for one) when `trackers > 0`.
When `remote > 0`, a `button` follows (`ml-auto h-6 rounded border
border-separator bg-window px-2`) reading `Load Remote Content`, or
`Loading…` and `disabled` while `loading`; clicking calls `onLoad()`.

## Tests (given, do not edit)

`app/src/features/reader/MessageHeader.test.tsx`.

## Gotchas

- `summary.date` is an RFC 3339 string: pass `new Date(summary.date)`.
- List the eight `bg-avatar-0` … `bg-avatar-7` classes in full so
  Tailwind generates them.

## Out of scope

Loading messages and rendering bodies (the planner's `ReaderContainer`,
`MessageFrame`, and `PlainText` from T-0028), and every file except
`app/src/features/reader/MessageHeader.tsx`.

## Done when

`make accept T=0031`, `make check` and `make ui-check` pass, and only
`app/src/features/reader/MessageHeader.tsx` changed.
