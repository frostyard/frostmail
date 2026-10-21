// Which layout the main window takes (docs/specs/ui.md, Narrow windows):
// one pane at a time below 600 CSS pixels of width, the phone in lodge's
// frame. The query and the hook are lodge-ui's (@frostyard/lodge-ui/theme);
// app.css's `narrow:` variant uses the same query, and the pane rules are
// the modules' own.
import { useNarrow } from "@frostyard/lodge-ui/react";
import { isNarrow as isNarrowNow, NARROW_QUERY } from "@frostyard/lodge-ui/theme";

/** NARROW is the media query of a narrow window, from lodge-ui. */
export const NARROW = NARROW_QUERY;

/** isNarrow reports whether the window is narrow now. */
export function isNarrow(): boolean {
  return isNarrowNow();
}

export { useNarrow };

/** NarrowPane is the pane a narrow window shows in Mail. */
export type NarrowPane = "mailboxes" | "list" | "reader";

/** narrowPane is the pane for Mail's state: Mailboxes when asked for, the message when one is selected. */
export function narrowPane(mailboxesOpen: boolean, selected: readonly number[]): NarrowPane {
  if (mailboxesOpen) return "mailboxes";
  return selected.length === 1 ? "reader" : "list";
}

/** CalendarPane is the pane a narrow window shows in Calendar. */
export type CalendarPane = "calendars" | "day" | "event";

/** calendarPane is Calendar's: its month and calendars when asked for, the event when one is selected, else the day. */
export function calendarPane(calendarsOpen: boolean, selected: unknown): CalendarPane {
  if (calendarsOpen) return "calendars";
  return selected ? "event" : "day";
}

/** PeoplePane is the pane a narrow window shows in People. */
export type PeoplePane = "books" | "people" | "person";

/** peoplePane is People's: the person, or a contact being edited, else the books when asked for, else the list. */
export function peoplePane(booksOpen: boolean, selected: number | null, editing: number | "new" | null): PeoplePane {
  if (editing !== null) return "person";
  if (booksOpen) return "books";
  return selected === null ? "people" : "person";
}

/** TasksPane is the pane a narrow window shows in Tasks. */
export type TasksPane = "taskLists" | "tasks" | "task";

/** tasksPane is Tasks': the lists when asked for, the task when one is selected, else the tasks. */
export function tasksPane(listsOpen: boolean, selected: number | null): TasksPane {
  if (listsOpen) return "taskLists";
  return selected === null ? "tasks" : "task";
}
