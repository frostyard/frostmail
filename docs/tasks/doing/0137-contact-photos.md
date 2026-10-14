---
id: "0137"
title: Contact photos, Delete Contact and refused changes
milestone: M5.5
size: M
touch:
  - app/src/lib/photo.ts
  - app/src/features/people/PhotoWell.tsx
  - app/src/features/people/RefusedNotice.tsx
  - app/src/app/RefusedNotices.tsx
  - app/src/app/PeopleModule.tsx
  - app/src/app/MainWindow.tsx
given:
  - app/src/features/people/PhotoWell.test.tsx
  - app/src/app/PeopleModule.photo.test.tsx
acceptance: make ui-vitest F="src/features/people src/app/PeopleModule"
---
# T-0137: Contact photos, Delete Contact and refused changes

## Goal

Finish contact editing (docs/specs/pim-ui.md, "Editing a contact" and
People "Behavior"): a photo well that sets and removes a contact's photo,
scaled in the app (ADR-0030); Delete Contact after asking; and a notice
when the server refused a contact or event change (`people.refused`,
`calendar.refused`).

## Read first

- `docs/specs/pim-ui.md`: "Editing a contact (`ContactEditor`,
  `PhotoWell`)" and People "Behavior" (Refused changes).
- `app/src/app/PeopleModule.tsx`: the editor's container from T-0136.
- `app/src/features/people/Avatar.tsx`, `app/src/features/outbox/Outbox.tsx`
  (`UndoToast`, the notice's frame), and `app/src/app/SidebarContainer.tsx`
  (`confirmMailbox`, the way the app asks).
- `schema/rpc/people.yaml`: `people.setPhoto`, `people.delete`, `Photo`;
  `schema/rpc/calendar.yaml` and `people.yaml`: the `refused` events.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`app/src/lib/photo.ts`:**
  - `PhotoChange = Photo | null | undefined` (new, removed, kept).
  - `PHOTO_SIZE = 512`.
  - `scalePhoto(file: Blob): Promise<Photo>` crops the image to its
    centered square and scales it to at most 512 × 512 on a canvas
    (`createImageBitmap`, `drawImage`, `toBlob` JPEG at quality 0.9).
    It returns `{contentType: "image/jpeg", data: <base64>}`. The tests
    stub it, since the test DOM cannot draw.
  - `photoURL(photo)` is a `data:` URL.
- **`PhotoWell`** `{name, email, photo?, onPick(file), onRemove?}`:
  - a button "Change Photo" holding the 64px `Avatar`, which clicks a
    hidden file input named "Photo file"
    (`accept="image/jpeg,image/png"`). Picking a file calls `onPick`
    and clears the input;
  - a text button "Remove Photo" while there is a photo and `onRemove`.
- **`RefusedNotice`:**
  - `refusedText(name, reason)` is exactly
    `“<name>” was changed on another device, so your change wasn't saved.`
    for `changed`, `“<name>” was deleted on another device.` for
    `deleted`, and `The server didn't accept your change to “<name>”.`
    otherwise;
  - `RefusedNotice {text, onDismiss}` is an `output` in the undo toast's
    frame, with a 14px `X` button named "Dismiss".
- **`RefusedNotices`** (mounted in `MainWindow` beside `UndoToasts`)
  listens for `people.refused` (its `name`) and `calendar.refused` (its
  `summary`). It shows each as a notice, stacked at the bottom center,
  until Dismiss or for 8 seconds (`NOTICE_MS`), and clears its timers on
  unmount.
- **`PeopleModule`'s editor:**
  - passes a `PhotoWell` as `photo`. It shows the picked photo, the
    person's current one (`data.photo`, for an existing contact), or
    none once removed;
  - picking scales the file (`scalePhoto`) and keeps it pending, and
    Remove Photo makes the pending change a removal;
  - Done writes the fields as T-0136 does (a contact with only a photo
    change makes no `people.update`), then, when the photo changed,
    `people.setPhoto {id, photo}` or `{id}` to remove. For a new contact
    the ID is the created one;
  - passes `onDelete` for an existing contact. It asks first, with
    `window.confirm` outside Tauri, and inside Tauri the dialog plugin's
    `ask` titled "Delete Contact?" with OK labeled Delete. The text is
    `Delete "<display name>" from <book> — <account>? It is removed from
    every device that syncs this address book.` On yes it sends
    `people.delete {id}` and closes the editor.

## Tests (given, do not edit)

`app/src/features/people/PhotoWell.test.tsx` and
`app/src/app/PeopleModule.photo.test.tsx`. T-0136's
`PeopleModule.edit.test.tsx` must still pass.

## Gotchas

- The test stubs `confirm` with `vi.stubGlobal`. Call `window.confirm`,
  not a reference to it saved earlier.
- The test mocks `../lib/photo`'s `scalePhoto` and keeps the rest.
  Import `scalePhoto` and `photoURL` from it by name.
- Clear the pending photo when a new edit starts.

## Out of scope

Undo of these changes (M5.5 Phase 4), and every file not under `touch`.

## Done when

`make accept T=0137` and `make ui-check` pass, and only the files under
`touch` changed.
