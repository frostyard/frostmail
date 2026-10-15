// The arithmetic of dragging events in the calendar views (docs/specs/
// pim-ui.md, Changing events in the views): 15-minute steps in the app's
// wall time, whole days, and the EventFields a drop writes.
import type { EventFields, Occurrence } from "../rpc/gen/api";
import { addDays, instantAt, zoned } from "./calendarDates";

/** STEP is a drag's step, in minutes. */
export const STEP = 15;

/** PX_PER_MINUTE is the time grid's scale: 48 pixels an hour. */
export const PX_PER_MINUTE = 0.8;

/** THRESHOLD is how far, in pixels, a pointer moves before a press becomes a drag. */
export const THRESHOLD = 4;

/** Range is a timed range of instants, as ISO strings. */
export interface Range {
  start: string;
  end: string;
}

/** stepsOf is a vertical pointer move in pixels as whole steps of minutes. */
export function stepsOf(dy: number): number {
  return Math.round(dy / PX_PER_MINUTE / STEP) * STEP;
}

/** daysOf is a horizontal pointer move in pixels as whole days of columns that wide. */
export function daysOf(dx: number, columnWidth: number): number {
  return columnWidth > 0 ? Math.round(dx / columnWidth) : 0;
}

/** minuteAt is the step a pointer y pixels below the day's top falls in, within the day. */
export function minuteAt(y: number): number {
  return Math.max(0, Math.min(24 * 60 - STEP, Math.floor(y / PX_PER_MINUTE / STEP) * STEP));
}

/** moved is a timed range moved by days and minutes of the app's wall time, keeping its length. */
export function moved(range: Range, days: number, minutes: number, timeZone: string): Range {
  const wall = zoned(range.start, timeZone);
  const start = instantAt(addDays(wall.date, days), wall.minutes + minutes, timeZone);
  const duration = new Date(range.end).getTime() - new Date(range.start).getTime();
  return { start: start.toISOString(), end: new Date(start.getTime() + duration).toISOString() };
}

/** resized is a timed range whose end moved by minutes of wall time, ending at least a step after its start. */
export function resized(range: Range, minutes: number, timeZone: string): Range {
  const wall = zoned(range.end, timeZone);
  const end = instantAt(wall.date, wall.minutes + minutes, timeZone);
  const minimum = new Date(range.start).getTime() + STEP * 60_000;
  return { start: range.start, end: new Date(Math.max(end.getTime(), minimum)).toISOString() };
}

/** drawn is the range drawn on a day across two steps (in either order): from the earlier step to the end of the
 *  later one. */
export function drawn(date: string, from: number, to: number, timeZone: string): Range {
  return {
    start: instantAt(date, Math.min(from, to), timeZone).toISOString(),
    end: instantAt(date, Math.min(Math.max(from, to) + STEP, 24 * 60), timeZone).toISOString(),
  };
}

/** canMove reports whether the user may drag an occurrence: in a writable calendar, and not an invitation. */
export function canMove(o: Occurrence, writable: ReadonlySet<number>): boolean {
  return writable.has(o.calendarId) && o.answer === undefined;
}

/** shiftedDays is the fields that move an occurrence by whole days, keeping its times (the month view, the all-day
 *  strip), with a timed one written in zone. */
export function shiftedDays(o: Occurrence, days: number, appZone: string, zone: string): EventFields {
  if (o.allDay) {
    return { allDay: true, startDate: addDays(o.startDate, days), endDate: addDays(o.endDate, days) };
  }
  return dropFields(moved(o, days, 0, appZone), zone);
}

/** dropFields is what a drop of a timed range writes: its instants, in the event's zone. */
export function dropFields(range: Range, zone: string): EventFields {
  return { allDay: false, start: range.start, end: range.end, timeZone: zone };
}
