// The reminder window's container (docs/specs/pim-ui.md, Reminder window).
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useCallback, useEffect, useRef, useState } from "react";
import { useClient } from "../data/session";
import { ReminderPanel } from "../features/calendar/ReminderPanel";
import { zoned } from "../lib/calendarDates";
import type { Reminder } from "../rpc/gen/api";
import { openOccurrenceInMain } from "./reminders";
import { useCalendarColors, useCalendarFrame } from "./useCalendar";

function closeWindow() {
  if (isTauri())
    void getCurrentWindow()
      .close()
      .catch((err: unknown) => console.warn("close reminders", err));
  else window.close();
}

function useReminders() {
  const client = useClient();
  const [rows, setRows] = useState<Reminder[]>([]);
  const pending = useRef(new Set<string>());
  const revision = useRef(0);
  const mounted = useRef(false);
  const load = useCallback(() => {
    const version = ++revision.current;
    void client.calendar
      .reminders({})
      .then((result) => {
        if (mounted.current && version === revision.current) {
          setRows(result.filter((row) => !pending.current.has(row.id)));
        }
      })
      .catch((err: unknown) => console.warn("reminders", err));
  }, [client]);
  useEffect(() => {
    mounted.current = true;
    const off = client.transport.onEvent((event) => {
      if (event.event === "calendar.reminders" || event.event === "calendar.changed") load();
    });
    load();
    return () => {
      mounted.current = false;
      ++revision.current;
      off();
    };
  }, [client, load]);
  const change = (ids: string[], until?: Date) => {
    ++revision.current;
    for (const id of ids) pending.current.add(id);
    setRows((rows) => rows.filter((row) => !pending.current.has(row.id)));
    const request = until
      ? client.calendar.snooze({ ids, until: until.toISOString() })
      : client.calendar.dismiss({ ids });
    void request
      .catch((err: unknown) => console.warn("change reminders", err))
      .finally(() => {
        for (const id of ids) pending.current.delete(id);
        if (mounted.current) load();
      });
  };
  return { rows, change };
}

/** RemindersWindow lists the due reminders and acts on them. */
export function RemindersWindow() {
  const frame = useCalendarFrame();
  const colors = useCalendarColors();
  const { rows, change } = useReminders();
  const [now, setNow] = useState(frame.now);
  const hadRows = useRef(false);
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 30_000);
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeWindow();
    };
    window.addEventListener("keydown", onKey);
    return () => {
      clearInterval(timer);
      window.removeEventListener("keydown", onKey);
    };
  }, []);
  useEffect(() => {
    if (rows.length > 0) hadRows.current = true;
    else if (hadRows.current) {
      hadRows.current = false;
      closeWindow();
    }
  }, [rows]);
  return (
    <ReminderPanel
      reminders={rows}
      colors={colors}
      timeZone={frame.timeZone}
      locale={frame.locale}
      now={now}
      onSnooze={(id, until) => change([id], until)}
      onDismiss={(ids) => change(ids)}
      onClose={closeWindow}
      onOpen={(reminder) => {
        void openOccurrenceInMain({
          eventId: reminder.eventId,
          recurrenceId: reminder.recurrenceId,
          date: reminder.allDay ? reminder.startDate : zoned(reminder.start, frame.timeZone).date,
        }).catch((err: unknown) => console.warn("open occurrence", err));
      }}
    />
  );
}
