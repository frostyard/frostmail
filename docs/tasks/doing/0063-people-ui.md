---
id: "0063"
title: Build the People module's components
milestone: M4.5
size: L
touch:
  - app/src/features/sidebar/ModuleBar.tsx
  - app/src/features/people/PeopleSidebar.tsx
  - app/src/features/people/PersonRow.tsx
  - app/src/features/people/Avatar.tsx
  - app/src/features/people/PersonPane.tsx
  - app/src/lib/peopleRows.ts
given:
  - app/src/features/sidebar/ModuleBar.test.tsx
  - app/src/features/people/PeopleSidebar.test.tsx
  - app/src/features/people/PersonRow.test.tsx
  - app/src/features/people/Avatar.test.tsx
  - app/src/features/people/PersonPane.test.tsx
  - app/src/lib/peopleRows.test.ts
acceptance: make ui-vitest F="src/features/sidebar/ModuleBar.test.tsx src/features/people src/lib/peopleRows.test.ts"
---
# T-0063: Build the People module's components

## Goal

The main window gains modules (Mail, Calendar, People, Tasks) and a People
module that lists the people maild joins from the accounts' address books
(ADR-0020). Build its presentational components: the module bar, the
People sidebar, the list's rows and letter headers, the avatar, the person
pane, and the helper that lays out the list's rows. A later card wires
them to maild.

## Read first

- `docs/specs/pim-ui.md`: "Modules", "Module bar", "People module" (all
  of it). `docs/specs/ui.md`: Tokens, type, Sidebar, Message list (rows,
  selection, dates), Reader (the avatar).
- The stubs, which fix the props: `app/src/features/sidebar/ModuleBar.tsx`,
  `app/src/features/people/*.tsx`, `app/src/lib/peopleRows.ts`;
  `app/src/lib/modules.ts` (`Module`, `MODULE_LABEL`, `MODULE_SHORTCUT`).
- Patterns to follow: `app/src/features/sidebar/Sidebar.tsx` (the tree,
  sections, rows, selection and arrow keys: the People sidebar works the
  same way), `app/src/features/list/MessageRow.tsx` (a row and its
  selection colors), `app/src/features/reader/MessageHeader.tsx` (the
  avatar's tone classes), `app/src/lib/format.ts` (`initials`,
  `avatarTone`, `formatListDate`).
- `app/src/rpc/gen/api.ts`: `PersonSummary`, `Person`, `Contact`,
  `LabeledValue`, `PostalAddress`, `MessageSummary`.
- `app/src/styles/app.css`: the theme's colors and type roles.
- The given tests.
- `docs/tasks/EXECUTOR.md` (TypeScript conventions).

## Contract

Keep each file's exported names and props; replace the stubs' "Task T-0063
writes it" comments with what the code does. Everything the given tests
check is in the spec; in brief:

- **`ModuleBar`**: a `toolbar` named "Modules", `h-[44px]`, a top
  `border-separator`, `bg-sidebar`; a 36 × 28 `button` per entry of
  `modules` in that order, named `MODULE_LABEL[m]`, titled
  `"<label> (<shortcut>)"`, `aria-pressed`; the icon (`Mail`, `Calendar`,
  `Users`, `ListChecks`, 18px) has `text-accent` when current and
  `text-secondary` otherwise. Click calls `onSelect(m)`.
- **`PeopleSidebar`**: a focusable `tree` named "Address Books"; a
  "People" section with the All Contacts row (`data-key="all"`, `Users`
  icon), then a section per account (`data-key="book:<id>"` rows,
  `BookUser` icon, a 12px `Lock` with `aria-label="Read-only"` after a
  read-only book's name). Section headers are buttons named by their title
  with `aria-expanded`, collapsing as `Sidebar`'s do; selection colors,
  click and ↑/↓/Home/End over the visible rows as `Sidebar`'s.
  `onSelect` receives `"all"` or the book's ID.
- **`PersonRow`**: an `option` with `data-person-id`, `tabIndex={-1}`,
  `h-[44px]`, `aria-selected`; a 28px `Avatar`; the name (`font-semibold`,
  the email when the name is empty) over the organization, else the email
  (`text-secondary`), the second line left out when it repeats the first.
  Selection: `bg-accent text-accent-contrast` when selected and focused,
  `bg-selection-inactive` when selected only. Click calls
  `onSelect(person.id)`.
- **`IndexHeader`**: an element with `role="presentation"`, `h-[24px]`,
  `sticky top-0`, `bg-sidebar`, the letter in 11/14 600 `text-secondary`.
- **`Avatar`**: with `photo`, an `img` (`alt=""`, `object-cover`,
  rounded-full) of the size; otherwise an `aria-hidden` circle with
  `initials({name, address: email})` on `bg-avatar-<avatarTone(email)>`.
  Sizes map to `size-7`, `size-12`, `size-16`; initials are 13/600 at 28
  and 48, 20/600 at 64.
- **`peopleRows`**: an `{kind: "index"}` row before each person whose
  `index` differs from the previous person's (the first included).
- **`PersonPane`**: as the spec's "Person pane": "No Contact Selected";
  the header (64px `Avatar` with the photo, the name as an `h2`, title and
  organization joined by " · " in `text-secondary`, the Message button
  with `bg-accent` that calls `onCompose(first email)` and is disabled
  without one); a labeled `section` per contact with its `h3`
  (`"<book> — <account>"`, `" · Read-only"`, class `uppercase`) and a `dl`
  of `dt`/`dd` rows in the spec's order and default labels (email, phone,
  address, homepage, birthday, nickname, note); emails as buttons calling
  `onCompose`; `http`/`https` URLs as links whose click is prevented and
  calls `onOpenURL`, other URLs as text; address lines as `div`s;
  birthdays with `Intl.DateTimeFormat(undefined, { dateStyle: "long" })`
  or, for `--MM-DD`, `{ month: "long", day: "numeric" }` on a day of the
  year 2000; the "Recent Mail" `section` with a `button` per message
  (subject, else "(No Subject)" in `text-tertiary`, then
  `formatListDate(date, now)`) calling `onOpenMessage(id)`, absent when
  `recent` is empty. A book missing from `books` is labeled by its
  collection ID.

## Tests (given, do not edit)

`app/src/features/sidebar/ModuleBar.test.tsx`,
`app/src/features/people/{PeopleSidebar,PersonRow,Avatar,PersonPane}.test.tsx`,
`app/src/lib/peopleRows.test.ts`.

## Gotchas

- Tailwind only generates classes it finds whole in the source: list the
  eight `bg-avatar-N` classes and the size classes literally, as
  `MessageHeader.tsx` does.
- Dates: `new Date(1815, 11, 10)` is a local date; parse `YYYY-MM-DD`
  into year, month and day numbers rather than with `new Date(string)`,
  which reads it as UTC.
- The pane scrolls; the list virtualizes in the container (next card), so
  `PersonRow` and `IndexHeader` stay plain rows.
- No `dangerouslySetInnerHTML`; the note renders as text with
  `whitespace-pre-wrap`.

## Out of scope

Containers, data loading, the main window and toolbar, keyboard shortcuts,
the contact card popover, and every file not under `touch`.

## Done when

`make accept T=0063` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
