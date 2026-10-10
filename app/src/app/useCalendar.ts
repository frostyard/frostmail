// Calendar data with debounced refreshes and cancellation of stale answers.
import { useCallback, useEffect, useMemo, useState } from "react";
import { useClient } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { CalendarSection } from "../features/calendar/CalendarSidebar";
import { addDays, appLocale, localeWeekStart, monthGrid, today, viewRange, zoned } from "../lib/calendarDates";
import { busyDates } from "../lib/eventLayout";
import type { CalendarEvent, Client, Collection, Event, Occurrence } from "../rpc/gen/api";

/** useCalendarFrame supplies the local calendar date, locale and minute clock. */
export function useCalendarFrame() {
  const selected = useUI((state) => state.calendarDate);
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 60_000);
    return () => clearInterval(timer);
  }, []);
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const locale = appLocale(navigator.language);
  const current = today(timeZone, now);
  return { timeZone, locale, weekStart: localeWeekStart(locale), date: selected || current, today: current, now };
}

function useRefresh(client: Client, active: boolean, eventName: Event["event"], load: () => () => void) {
  useEffect(() => {
    if (!active) return;
    let stop = load();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const off = client.transport.onEvent((event) => {
      if (event.event !== eventName) return;
      clearTimeout(timer);
      timer = setTimeout(() => {
        stop();
        stop = load();
      }, 100);
    });
    return () => {
      clearTimeout(timer);
      stop();
      off();
    };
  }, [client, active, eventName, load]);
}

function useCalendarCollections(client: Client, active: boolean) {
  const [collections, setCollections] = useState<Collection[]>([]);
  const load = useCallback(() => {
    let stopped = false;
    void client.account
      .collections({ kind: "calendar" })
      .then((result) => {
        if (!stopped) setCollections(result);
      })
      .catch((err: unknown) => console.warn("calendars", err));
    return () => {
      stopped = true;
    };
  }, [client]);
  useRefresh(client, active, "account.changed", load);
  return collections;
}

/** useCalendarColors loads calendar colors and refreshes them on account changes. */
export function useCalendarColors() {
  const collections = useCalendarCollections(useClient(), true);
  return useMemo(() => new Map(collections.map((calendar) => [calendar.id, calendar.color])), [collections]);
}

/** openOccurrence selects an occurrence in Calendar, keeping the current view. */
export function openOccurrence(occurrence: Occurrence, timeZone: string) {
  const ui = useUI.getState();
  ui.setModule("calendar");
  ui.selectOccurrence(
    { eventId: occurrence.eventId, recurrenceId: occurrence.recurrenceId },
    occurrence.allDay ? occurrence.startDate : zoned(occurrence.start, timeZone).date,
  );
}

function useCalendars(client: Client, active: boolean) {
  const accounts = useMail((state) => state.accounts);
  const collections = useCalendarCollections(client, active);
  return useMemo(() => {
    const sections: CalendarSection[] = accounts
      .map((account) => ({
        accountId: account.id,
        title: account.email,
        calendars: collections
          .filter((calendar) => calendar.accountId === account.id)
          .map((calendar) => ({
            id: calendar.id,
            name: calendar.name,
            color: calendar.color,
            enabled: calendar.enabled,
            isDefault: calendar.isDefault,
            readOnly: calendar.readOnly || account.readOnly,
          })),
      }))
      .filter((section) => section.calendars.length > 0);
    const colors = new Map(collections.map((calendar) => [calendar.id, calendar.color]));
    return { sections, colors };
  }, [accounts, collections]);
}

/** useCalendarRange loads occurrences and refreshes on changes or a clock tick. */
export function useCalendarRange(
  client: Client,
  active: boolean,
  from: string,
  to: string,
  timeZone: string,
  refreshAt = 0,
) {
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const request = useMemo(() => ({ from, to, timeZone, refreshAt }), [from, to, timeZone, refreshAt]);
  const load = useCallback(() => {
    let stopped = false;
    void client.calendar
      .range({ from: request.from, to: request.to, timeZone: request.timeZone })
      .then((result) => {
        if (!stopped) setOccurrences(result);
      })
      .catch((err: unknown) => console.warn("calendar range", err));
    return () => {
      stopped = true;
    };
  }, [client, request]);
  useRefresh(client, active, "calendar.changed", load);
  return occurrences;
}

function useEvent(client: Client, active: boolean) {
  const selected = useUI((state) => state.calendarSelected);
  const [event, setEvent] = useState<CalendarEvent | null>(null);
  const load = useCallback(() => {
    let stopped = false;
    if (selected === null) setEvent(null);
    else
      void client.calendar
        .event({ id: selected.eventId, ...(selected.recurrenceId ? { recurrenceId: selected.recurrenceId } : {}) })
        .then((result) => {
          if (!stopped) setEvent(result);
        })
        .catch(() => {
          if (!stopped) {
            setEvent(null);
            useUI.getState().selectOccurrence(null);
          }
        });
    return () => {
      stopped = true;
    };
  }, [client, selected]);
  useRefresh(client, active, "calendar.changed", load);
  return active && event?.id === selected?.eventId && event?.recurrenceId === selected?.recurrenceId ? event : null;
}

/** useCalendar loads the active view, small month's busy days and event details. */
export function useCalendar() {
  const client = useClient();
  const active = useUI((state) => state.module === "calendar");
  const view = useUI((state) => state.calendarView);
  const frame = useCalendarFrame();
  const range = viewRange(view, frame.date, frame.weekStart);
  const days = useMemo(() => monthGrid(frame.date, frame.weekStart), [frame.date, frame.weekStart]);
  const occurrences = useCalendarRange(client, active, range.from, range.to, frame.timeZone);
  const smallMonth = useCalendarRange(
    client,
    active,
    days[0] ?? frame.date,
    addDays(days.at(-1) ?? frame.date, 1),
    frame.timeZone,
  );
  const busy = useMemo(() => busyDates(smallMonth, days, frame.timeZone), [smallMonth, days, frame.timeZone]);
  const event = useEvent(client, active);
  const calendars = useCalendars(client, active);
  return { ...frame, ...calendars, occurrences, busy, event };
}

/** CalendarData is the connected Calendar module's data. */
export type CalendarData = ReturnType<typeof useCalendar>;
