// The arithmetic of dragging events in the calendar views (docs/specs/
// pim-ui.md, Changing events in the views): 15-minute steps in the app's
// wall time, whole days, and the EventFields a drop writes.
import type { EventFields, Occurrence } from "../rpc/gen/api";

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

function notYet(): never {
  throw new Error("lib/eventDrag comes with task card T-0151");
}

/** stepsOf is a vertical pointer move in pixels as whole steps of minutes. */
export function stepsOf(_dy: number): number {
  return notYet();
}

/** daysOf is a horizontal pointer move in pixels as whole days of columns that wide. */
export function daysOf(_dx: number, _columnWidth: number): number {
  return notYet();
}

/** minuteAt is the step a pointer y pixels below the day's top falls in, within the day. */
export function minuteAt(_y: number): number {
  return notYet();
}

/** moved is a timed range moved by days and minutes of the app's wall time, keeping its length. */
export function moved(_range: Range, _days: number, _minutes: number, _timeZone: string): Range {
  return notYet();
}

/** resized is a timed range whose end moved by minutes of wall time, ending at least a step after its start. */
export function resized(_range: Range, _minutes: number, _timeZone: string): Range {
  return notYet();
}

/** drawn is the range drawn on a day across two steps (in either order): from the earlier step to the end of the
 *  later one. */
export function drawn(_date: string, _from: number, _to: number, _timeZone: string): Range {
  return notYet();
}

/** canMove reports whether the user may drag an occurrence: in a writable calendar, and not an invitation. */
export function canMove(_o: Occurrence, _writable: ReadonlySet<number>): boolean {
  return notYet();
}

/** shiftedDays is the fields that move an occurrence by whole days, keeping its times (the month view, the all-day
 *  strip), with a timed one written in zone. */
export function shiftedDays(_o: Occurrence, _days: number, _appZone: string, _zone: string): EventFields {
  return notYet();
}

/** dropFields is what a drop of a timed range writes: its instants, in the event's zone. */
export function dropFields(_range: Range, _zone: string): EventFields {
  return notYet();
}
