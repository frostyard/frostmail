// The calendar domain of MockTransport: the fixture's events and their
// occurrences, as maild answers calendar.range and calendar.event
// (docs/specs/pim-ui.md; internal/engine/calendar.go). Task T-0072 builds
// MockCalendar.
import type { CalendarEvent } from "../gen/api";

/** MockEvent is a stored event: the CalendarEvent of its first occurrence
 *  (recurrenceId ""), repeating every day or week `count` times when
 *  `every` is set. */
export interface MockEvent {
  event: CalendarEvent;
  every?: "day" | "week";
  count?: number;
}

/** MockCalendarData is the events the mock serves; their calendars are
 *  collections in MockPeopleData. */
export interface MockCalendarData {
  events: MockEvent[];
  /** The clock contact cards' upcoming occurrences count from. */
  now: string;
}
