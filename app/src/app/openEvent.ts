// Opening events in the event editor window (docs/specs/pim-ui.md, Event
// editor window). In the app each event gets its own Tauri window, labeled
// event-<id> (event-<id>-<occurrence> for one occurrence of a series,
// event-new-<n> for a new event), which learns what it edits from an
// initialization script; in a browser (the dev gateway) it opens in a new
// tab at #/event/<id>?recurrenceId=… or #/event/new?….
import { invoke, isTauri } from "@tauri-apps/api/core";

import { addDays, instantAt, zoned } from "../lib/calendarDates";

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

declare global {
  interface Window {
    /** Set by the event editor window's initialization script. */
    __frostmailEvent?: EditorTarget;
  }
}

function fromHash(hash: string): EditorTarget | null {
  const m = /^#\/event\/(new|\d+)(?:\?(.*))?$/.exec(hash);
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

function toHash(target: EditorTarget): string {
  if (target.kind === "edit") {
    return `#/event/${target.id}${target.recurrenceId ? `?${new URLSearchParams({ recurrenceId: target.recurrenceId })}` : ""}`;
  }
  const q = new URLSearchParams({ allDay: target.allDay ? "1" : "0" });
  for (const key of ["start", "end", "startDate", "endDate"] as const) if (target[key]) q.set(key, target[key]);
  if (target.calendarId !== undefined) q.set("calendarId", String(target.calendarId));
  return `#/event/new?${q}`;
}

/** editorTarget returns what this window edits, or null when it is not an event editor. */
export function editorTarget(): EditorTarget | null {
  return window.__frostmailEvent ?? fromHash(window.location.hash);
}

/** windowLabel is the editor window's Tauri label for a target. */
export function windowLabel(target: EditorTarget, now = Date.now()): string {
  if (target.kind === "new") return `event-new-${now}`;
  const occurrence = target.recurrenceId.replace(/[^A-Za-z0-9]+/g, "-").replace(/-+$/, "");
  return occurrence ? `event-${target.id}-${occurrence}` : `event-${target.id}`;
}

/** openEvent shows an event (or a new one) in its editor window, focusing the window that already shows it. */
export async function openEvent(target: EditorTarget, title = ""): Promise<void> {
  const label = windowLabel(target);
  if (isTauri()) {
    await invoke("open_event", { label, target, title: title || (target.kind === "new" ? "New Event" : "Event") });
    return;
  }
  window.open(`${window.location.pathname}${toHash(target)}`, label);
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
