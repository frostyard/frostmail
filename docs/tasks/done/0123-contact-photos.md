---
id: "0123"
title: Contact photos in the message list
milestone: M5
size: M
touch:
  - app/src/features/list/MessageRow.tsx
  - app/src/app/ListContainer.tsx
  - app/src/features/reader/MessageHeader.tsx
given:
  - app/src/features/list/MessageRow.photo.test.tsx
  - app/src/app/ContactPhotos.test.tsx
acceptance: make ui-vitest F="src/features/list src/app/ContactPhotos src/app/ListMenu src/features/reader"
---
# T-0123: Contact photos in the message list

## Goal

With Contact Photos on (T-0122's Sort menu), each row shows its sender's
photo from People, or the sender's initials, as Mail.app does. Photos are
the contacts' own; nothing is fetched from anywhere else.

## Read first

- `docs/specs/ui.md`: "Message list", the Contact photos bullet;
  `docs/design/app.md`: "Contact photos".
- `app/src/features/list/MessageRow.tsx`, `app/src/app/ListContainer.tsx`,
  and `app/src/features/reader/MessageHeader.tsx` (its avatar:
  `AVATAR_CLASSES`, `initials`, `avatarTone`).
- `app/src/data/stores.ts`: `contactPhotos`; `app/src/rpc/gen/api.ts`:
  `people.senders`, `people.photo`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`MessageHeader.tsx`** exports `AVATAR_CLASSES`, unchanged.
- **`MessageRow`**: an optional prop `photo?: string | null`.
  - `undefined`: no avatar, and the row's left padding stays `pl-6`.
  - Otherwise the row is padded `pl-16`. An element with `data-avatar`
    and `aria-hidden="true"`, a 32px circle at the row's left (24px in,
    8px down), holds an `img` of the URL for a string. For `null` it holds
    the sender's `initials` on `AVATAR_CLASSES[avatarTone(address)]`, as
    the reader's header does.
- **`ListContainer`**: with `contactPhotos` on, each row gets `photo`: the
  sender's photo as a `data:<contentType>;base64,<data>` URL, else
  `null`.
  - It asks `people.senders` about the rows' sender addresses (lowercased,
    each address at most once per window, at most 500 per request) and
    `people.photo` for each person found, once.
  - A row shows its photo when that arrives.
  - With `contactPhotos` off, rows get no `photo` and nothing is asked.

## Tests (given, do not edit)

`MessageRow.photo.test.tsx`, `ContactPhotos.test.tsx`. Every other app test
must keep passing.

## Gotchas

- Do not call the client during render: collect the addresses and ask in
  an effect (or a short timer), then re-render with what came back.
- Keep what was asked for the window's life, not per row: rows recycle as
  the list scrolls.

## Out of scope

Photos in the reader's header, maild, the mock, and every file not under
`touch`.

## Done when

`make accept T=0123` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
