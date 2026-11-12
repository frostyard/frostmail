// Opening events in the event editor window (docs/specs/pim-ui.md, Event
// editor window): a route the frame shows, /event/<id>?recurrenceId=… or
// /event/new?….
import { addDays, instantAt, zoned } from "../lib/calendarDates";
import { openFrameWindow } from "../lodge/frameWindow";

/** EditorTarget is what an event editor window edits: a stored event (one
 *  occurrence of a series when recurrenceId is set), or a new event with
 *  its first times. */
export type EditorTarget =
  | { kind: "edit"; id: number; recurrenceId: string }
  | {
      kind: "new";
      /** The calendar to start in; the default calendar when absent. */
      calendarId?: number;
      allDay: boolean;
      /** Timed: instants. */
      start: string;
      end: string;
      /** All-day: the first day, and the day after the last. */
      startDate: string;
      endDate: string;
    };

function parseRoute(route: string): EditorTarget | null {
  const m = /^\/event\/(new|\d+)(?:\?(.*))?$/.exec(route);
  if (!m?.[1]) return null;
  const q = new URLSearchParams(m[2] ?? "");
  if (m[1] !== "new") return { kind: "edit", id: Number(m[1]), recurrenceId: q.get("recurrenceId") ?? "" };
  const calendarId = Number(q.get("calendarId"));
  return {
    kind: "new",
    ...(Number.isSafeInteger(calendarId) && calendarId > 0 ? { calendarId } : {}),
    allDay: q.get("allDay") === "1",
    start: q.get("start") ?? "",
    end: q.get("end") ?? "",
    startDate: q.get("startDate") ?? "",
    endDate: q.get("endDate") ?? "",
  };
}

/** eventRoute is a target's route: /event/<id>?… or /event/new?…. */
export function eventRoute(target: EditorTarget): string {
  if (target.kind === "edit") {
    return `/event/${target.id}${target.recurrenceId ? `?${new URLSearchParams({ recurrenceId: target.recurrenceId })}` : ""}`;
  }
  const q = new URLSearchParams({ allDay: target.allDay ? "1" : "0" });
  for (const key of ["start", "end", "startDate", "endDate"] as const) if (target[key]) q.set(key, target[key]);
  if (target.calendarId !== undefined) q.set("calendarId", String(target.calendarId));
  return `/event/new?${q}`;
}

/** eventFromRoute is the target a route names, or null. */
export function eventFromRoute(route: string): EditorTarget | null {
  return parseRoute(route);
}

/** openEvent shows an event (or a new one) in the editor window, in place of the main window. */
export async function openEvent(target: EditorTarget): Promise<void> {
  openFrameWindow({ kind: "event", target });
}

/** newEventAt is a new event's first times on a date: an hour from the next whole hour today, else from 9:00 AM. */
export function newEventAt(date: string, today: string, now: Date, timeZone: string): EditorTarget {
  let day = date;
  let minutes = 9 * 60;
  if (date === today) {
    minutes = (Math.floor(zoned(now, timeZone).minutes / 60) + 1) * 60;
    if (minutes >= 24 * 60) {
      day = addDays(date, 1);
      minutes -= 24 * 60;
    }
  }
  const start = instantAt(day, minutes, timeZone);
  return {
    kind: "new",
    allDay: false,
    start: start.toISOString(),
    end: new Date(start.getTime() + 3_600_000).toISOString(),
    startDate: "",
    endDate: "",
  };
}
