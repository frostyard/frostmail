// The Calendar module's event pane (docs/specs/pim-ui.md, Event pane). Task
// T-0071 builds it.
import type { CalendarEvent } from "../../rpc/gen/api";

/** EventCalendar is the calendar an event is in. */
export interface EventCalendar {
  name: string;
  color: string;
  /** The account's email. */
  account: string;
}

/** EventPaneProps are the event pane's inputs. */
export interface EventPaneProps {
  event: CalendarEvent | null;
  calendar: EventCalendar | null;
  /** The app's time zone. */
  timeZone: string;
  locale: string;
  /** Open the contact card for a person, anchored at their name. */
  onPerson: (email: string, name: string, anchor: HTMLElement) => void;
}

/** EventPane shows the selected event's details. */
export function EventPane(_props: EventPaneProps) {
  return null;
}
