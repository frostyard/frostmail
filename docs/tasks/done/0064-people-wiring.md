---
id: "0064"
title: Wire the People module into the main window
milestone: M4.5
size: L
touch:
  - app/src/data/stores.ts
  - app/src/lib/keymap.ts
  - app/src/app/MainWindow.tsx
  - app/src/app/PeopleModule.tsx
  - app/src/app/usePeople.ts
  - app/src/app/SidebarContainer.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/features/toolbar/Toolbar.tsx
  - app/src/features/search/SearchField.tsx
given:
  - app/src/app/PeopleModule.test.tsx
  - app/src/lib/keymap.modules.test.ts
  - app/src/features/toolbar/Toolbar.people.test.tsx
  - app/src/features/search/SearchField.placeholder.test.tsx
acceptance: make ui-vitest F="src/app/PeopleModule.test.tsx src/lib/keymap src/features/toolbar src/features/search"
---
# T-0064: Wire the People module into the main window

## Goal

The main window becomes modular (ADR-0020): Mail and People, switched with
Ctrl+1 and Ctrl+3 or the module bar under each sidebar. People shows the
address books, the people list and the selected person from maild, with
search, the toolbar's title and count, writing to a person, and opening
their recent mail in Mail. The components exist (T-0063); this card
connects them.

## Read first

- `docs/specs/pim-ui.md`: "Modules", "Module bar", "People module" (all of
  it, Behavior included). `docs/specs/ui.md`: Layout, Behavior, Keyboard
  map.
- `app/src/app/MainWindow.tsx`, `SidebarContainer.tsx`,
  `ToolbarContainer.tsx`, `ListContainer.tsx` (how Mail's list
  virtualizes, keeps its selection and handles keys), `openMessage.ts`
  (`revealMessage`), `compose.ts` (`startDraft`, `composeTo`).
- `app/src/data/stores.ts` (`useUI`), `session.tsx` (`useClient`, how
  events are subscribed with `client.transport.onEvent`).
- `app/src/lib/keymap.ts`, `modules.ts`, `peopleRows.ts`.
- The components: `app/src/features/people/*`,
  `features/sidebar/ModuleBar.tsx`, `features/toolbar/Toolbar.tsx`,
  `features/search/SearchField.tsx`.
- `app/src/rpc/mock/people.ts` and `fixture.ts`: what the tests run
  against (address books 101 Contacts and 102 Shared, read-only; twelve
  people).
- The given tests.
- `docs/tasks/EXECUTOR.md` (TypeScript conventions).

## Contract

- **Store (`stores.ts`):** `UIState` gains `module: Module` (initially
  `"mail"`, not persisted) and `setModule(m)`, and the People module's own
  state (the selected book, `"all"` or its ID; the selected person ID or
  null; the committed search and the field's text; the focused pane), kept
  while another module is shown. Mail's state is unchanged; `setSource`
  still clears Mail's search and selection only.
- **Keys (`keymap.ts`):** commands `showCalendar`, `showPeople` and
  `showTasks` on Ctrl+2, Ctrl+3 and Ctrl+4 (Ctrl alone), allowed in text
  fields as Ctrl+1 is. `MainWindow`: Ctrl+1 shows Mail and All Inboxes,
  Ctrl+3 People; Ctrl+2 and Ctrl+4 do nothing until those modules exist.
  Mail's other commands act only while Mail is shown; in People, Tab and
  Shift+Tab move between its panes, Ctrl+F (and Ctrl+Alt+F) focus the
  search field, Escape clears the People search, Ctrl+N composes (to the
  selected person when the list has focus), Ctrl+, opens Settings.
- **Module bar:** `SidebarContainer` (Mail) and `PeopleModule` end their
  sidebar with `ModuleBar` listing the built modules, `["mail",
  "people"]`, current pressed; selecting calls `setModule` (selecting Mail
  keeps Mail's source).
- **`Toolbar`:** an optional `mode?: "mail" | "people"` (default
  `"mail"`). In `"people"` the sidebar segment has no Get Mail and the
  reader segment has none of the message actions (Delete, Archive, Flag,
  Mark, Move, Reply, Reply All, Forward); the rest is unchanged.
- **`SearchField`:** an optional `placeholder` (default `"Search"`); its
  accessible name stays "Search".
- **`ToolbarContainer`:** in People, `mode="people"`, the title ("All
  Contacts" or the selected book's name), the subtitle ("1 contact", "N
  contacts", "N results" while a search is committed), New Message as
  `composeTo(first email)` of the selected person (or a blank new message),
  and the search field bound to the People search with the placeholder
  "Search Contacts".
- **`PeopleModule` (and `usePeople`):** the People panes in Mail's widths
  and splitters (sidebar hidden with Ctrl+Alt+S as in Mail):
  - the sidebar: `PeopleSidebar` with an "Address Books" section per
    account that has address books (`account.collections` with
    `kind: "addressbook"`, titled by the account's display name, else
    email), then `ModuleBar`; refetched on `account.changed`;
  - the list: a `listbox` named "Contacts" with `peopleRows(people)` as
    `IndexHeader` and `PersonRow` rows, virtualized with
    `@tanstack/react-virtual` (headers 24, rows 44). A scroll element that
    measures 0 high (hidden, or in happy-dom) counts as 800 high, so rows
    render. ↑/↓, Home and End on the listbox move the selection; click
    selects; focus colors as Mail's list;
  - the data: `people.list` with only the parameters that apply
    (`collectionId` for a book, `query` for a committed search; `{}` for
    every person), refetched when either changes and 100 ms after the last
    `people.changed` event; the selection follows the person's ID and
    clears when they are gone;
  - the pane: `PersonPane` with `people.get` of the selection,
    `people.card` of the person's first email for its `recent`,
    `people.photo` when `hasPhoto`, `books` from the collections and
    accounts; Message and email clicks call `composeTo`; URLs open as the
    reader's links do (`invoke("open_link", { url })` in Tauri, see
    `ReaderContainer.tsx`; nothing outside Tauri); a recent message calls
    `revealMessage(client, id)` and `setModule("mail")`.
  - Requests fail quietly (`console.warn`), as the other containers do.

## Tests (given, do not edit)

`app/src/app/PeopleModule.test.tsx` (through `MainWindow` and
`MockTransport`), `app/src/lib/keymap.modules.test.ts`,
`app/src/features/toolbar/Toolbar.people.test.tsx`,
`app/src/features/search/SearchField.placeholder.test.tsx`. The existing
tests (`Toolbar.test.tsx`, `keymap.test.ts`, `SearchField.test.tsx`, …)
must keep passing.

## Gotchas

- The given test resets the store with `useUI.setState(useUI.getInitialState())`;
  keep `useUI` a single zustand store so that works.
- Do not add dependencies.
- `composeTo` opens a compose window, which the tests cannot; call it
  without awaiting the window in a way that cannot throw out of a click
  handler.
- Keep functions under 60 lines and components presentational where they
  live under `features/`.

## Out of scope

The contact card popover (next card), Add to Contacts, Calendar and Tasks,
maild, and every file not under `touch`.

## Done when

`make accept T=0064` and `make ui-check` pass (taskrun runs them), and only
the files under `touch` changed.
