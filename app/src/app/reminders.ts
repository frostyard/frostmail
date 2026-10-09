// The reminder window: raising it from the main window, and asking the main
// window to open an occurrence (docs/specs/pim-ui.md, Reminder window).
import { invoke, isTauri } from "@tauri-apps/api/core";
import { emitTo, listen } from "@tauri-apps/api/event";
import { useUI } from "../data/stores";
import type { Client } from "../rpc/gen/api";

declare global {
  interface Window {
    /** Set by the reminder window's initialization script. */
    __frostmailReminders?: boolean;
  }
}

/** OccurrenceRequest names an occurrence for the main window to open. */
export interface OccurrenceRequest {
  eventId: number;
  recurrenceId: string;
  /** Its date in the app's zone, YYYY-MM-DD. */
  date: string;
}

/** isRemindersWindow reports whether this window shows the reminders. */
export function isRemindersWindow(): boolean {
  return window.__frostmailReminders === true || window.location.hash === "#/reminders";
}

/** openReminders shows the reminder window, raising it when it is open. */
export async function openReminders(): Promise<void> {
  if (isTauri()) {
    await invoke("open_reminders");
    return;
  }
  window.open(`${window.location.pathname}#/reminders`, "reminders");
}

/** openOccurrenceInMain asks the main window to open an occurrence in Calendar. */
export async function openOccurrenceInMain(request: OccurrenceRequest): Promise<void> {
  if (isTauri()) {
    await emitTo("main", "open-occurrence", request);
    return;
  }
  // BroadcastChannel crosses documents; also deliver requests made in this
  // document directly, without waiting for the broadcast round trip.
  window.dispatchEvent(new CustomEvent<OccurrenceRequest>("open-occurrence", { detail: request }));
  const channel = new BroadcastChannel("frostmail");
  channel.postMessage({ kind: "open-occurrence", ...request });
  channel.close();
}

/** watchReminders raises the reminder window from the main window; it returns the cleanup. */
export function watchReminders(client: Client): () => void {
  let stopped = false;
  const raise = () => {
    if (!stopped) void openReminders().catch((err: unknown) => console.warn("open reminders", err));
  };
  const off = client.transport.onEvent((event) => {
    if (event.event === "calendar.reminders" && event.data.count > 0) raise();
  });
  void client.calendar
    .reminders({})
    .then((rows) => {
      if (rows.length > 0) raise();
    })
    .catch((err: unknown) => console.warn("reminders", err));
  return () => {
    stopped = true;
    off();
  };
}

/** watchOccurrenceRequests opens the occurrences other windows ask for; it returns the cleanup. */
export function watchOccurrenceRequests(): () => void {
  let stopped = false;
  const reveal = (request: OccurrenceRequest) => {
    if (stopped) return;
    const ui = useUI.getState();
    ui.setModule("calendar");
    ui.selectOccurrence({ eventId: request.eventId, recurrenceId: request.recurrenceId }, request.date);
  };
  const onLocalRequest = (event: Event) => {
    if (event instanceof CustomEvent) reveal(event.detail as OccurrenceRequest);
  };
  window.addEventListener("open-occurrence", onLocalRequest);
  const channel = new BroadcastChannel("frostmail");
  channel.onmessage = (event: MessageEvent<unknown>) => {
    // Browser broadcasts name the sender's origin. Do not accept messages
    // from a different document origin (including non-browser channels).
    if (event.origin !== window.location.origin) return;
    const data = event.data;
    if (typeof data !== "object" || data === null) return;
    if (!("kind" in data) || data.kind !== "open-occurrence") return;
    if (!("eventId" in data) || typeof data.eventId !== "number") return;
    if (!("recurrenceId" in data) || typeof data.recurrenceId !== "string") return;
    if (!("date" in data) || typeof data.date !== "string") return;
    reveal({ eventId: data.eventId, recurrenceId: data.recurrenceId, date: data.date });
  };
  let stop: (() => void) | null = null;
  if (isTauri()) {
    void listen<OccurrenceRequest>("open-occurrence", (event) => reveal(event.payload))
      .then((unlisten) => {
        if (stopped) unlisten();
        else stop = unlisten;
      })
      .catch((err: unknown) => console.warn("watch occurrence requests", err));
  }
  return () => {
    stopped = true;
    stop?.();
    window.removeEventListener("open-occurrence", onLocalRequest);
    channel.close();
  };
}
