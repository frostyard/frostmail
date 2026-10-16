// Calendar panes share the main window's sidebar width.
import { forwardRef, type RefObject, useEffect, useImperativeHandle, useRef, useState } from "react";
import { useClient } from "../data/session";
import { type Pane, useUI } from "../data/stores";
import { CalendarSidebar } from "../features/calendar/CalendarSidebar";
import { EventPane } from "../features/calendar/EventPane";
import { MonthGrid } from "../features/calendar/MonthGrid";
import { useNotify } from "../features/calendar/NotifySheet";
import { ScopeSheet } from "../features/calendar/ScopeSheet";
import { TimeGrid } from "../features/calendar/TimeGrid";
import { ModuleBar } from "../features/sidebar/ModuleBar";
import { addDays, monthGrid, viewRange, zoned } from "../lib/calendarDates";
import { canMove, dropFields, type Range, shiftedDays } from "../lib/eventDrag";
import { withNotify } from "../lib/notify";
import type {
  CalendarDeleteParams,
  CalendarEvent,
  CalendarUpdateParams,
  EventFields,
  Occurrence,
} from "../rpc/gen/api";
import { ContactCardContainer, type ContactCardContainerProps } from "./ContactCardContainer";
import { openEvent } from "./openEvent";
import { rememberRestore } from "./pimUndo";
import { Splitter } from "./Splitter";
import type { CalendarData } from "./useCalendar";

/** CalendarHandle lets MainWindow focus panes, scroll hours and edit or delete the selection. */
export interface CalendarHandle {
  focus: (pane: Pane) => void;
  scroll: (hours: number) => void;
  edit: () => boolean;
  remove: () => boolean;
}

function useMonthLines(view: RefObject<HTMLElement | null>, month: boolean) {
  const [lines, setLines] = useState(4);
  useEffect(() => {
    const element = view.current;
    if (!element || !month) return;
    const measure = () => {
      const height = element.querySelector("[role='gridcell']")?.getBoundingClientRect().height ?? 0;
      setLines(height > 0 ? Math.max(2, Math.floor((height - 32) / 18)) : 4);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [view, month]);
  return lines;
}

function useCalendarDrag(data: CalendarData) {
  const client = useClient();
  const { ask, sheet } = useNotify();
  const [changing, setChanging] = useState<{ event: CalendarEvent; fields: EventFields } | null>(null);
  const writable = new Set(
    data.sections.flatMap((section) =>
      section.calendars.filter((calendar) => !calendar.readOnly).map((calendar) => calendar.id),
    ),
  );
  const update = (params: CalendarUpdateParams) => {
    void withNotify((notify) => client.calendar.update({ ...params, ...(notify ? { notify } : {}) }), ask)
      .then((result) => {
        if (result !== null) rememberRestore({ module: "calendar", restoreId: result.restoreId });
      })
      .catch((err: unknown) => console.warn("move event", err));
  };
  const drop = (occurrence: Occurrence, fieldsIn: (zone: string) => EventFields) => {
    if (!canMove(occurrence, writable)) return;
    void client.calendar
      .event({
        id: occurrence.eventId,
        ...(occurrence.recurrenceId ? { recurrenceId: occurrence.recurrenceId } : {}),
      })
      .then((event) => {
        const fields = fieldsIn(event.timeZone || data.timeZone);
        if (event.recurring && event.recurrenceId) setChanging({ event, fields });
        else update({ id: event.id, fields });
      })
      .catch((err: unknown) => console.warn("move event", err));
  };
  return {
    notifySheet: sheet,
    canMove: (occurrence: Occurrence) => canMove(occurrence, writable),
    onMove: (occurrence: Occurrence, range: Range) => drop(occurrence, (zone) => dropFields(range, zone)),
    onMoveDays: (occurrence: Occurrence, days: number) =>
      drop(occurrence, (zone) => shiftedDays(occurrence, days, data.timeZone, zone)),
    onDraw: (range: Range) => {
      void openEvent({
        kind: "new",
        allDay: false,
        start: range.start,
        end: range.end,
        startDate: "",
        endDate: "",
      }).catch((err: unknown) => console.warn("new event", err));
    },
    scopeSheet: changing && (
      <ScopeSheet
        action="change"
        first={changing.event.first ?? false}
        onChoose={(scope) => {
          update({ id: changing.event.id, recurrenceId: changing.event.recurrenceId, scope, fields: changing.fields });
          setChanging(null);
        }}
        onCancel={() => setChanging(null)}
      />
    ),
  };
}

function CalendarView({ data, elementRef }: { data: CalendarData; elementRef: RefObject<HTMLElement | null> }) {
  const ui = useUI();
  const { scopeSheet, notifySheet, onMove, onDraw, ...drag } = useCalendarDrag(data);
  const lines = useMonthLines(elementRef, ui.calendarView === "month");
  const range = viewRange(ui.calendarView, data.date, data.weekStart);
  const days =
    ui.calendarView === "month"
      ? monthGrid(data.date, data.weekStart)
      : Array.from({ length: ui.calendarView === "day" ? 1 : 7 }, (_, index) => addDays(range.from, index));
  const props = {
    ...drag,
    days,
    occurrences: data.occurrences,
    colors: data.colors,
    timeZone: data.timeZone,
    locale: data.locale,
    today: data.today,
    selected: ui.calendarSelected,
    onSelect: (occurrence: CalendarData["occurrences"][number]) =>
      ui.selectOccurrence(
        { eventId: occurrence.eventId, recurrenceId: occurrence.recurrenceId },
        occurrence.allDay ? occurrence.startDate : zoned(occurrence.start, data.timeZone).date,
      ),
    onShowDay: (date: string) => {
      ui.setCalendarDate(date);
      ui.setCalendarView("day");
    },
  };
  return (
    <>
      {ui.calendarView === "month" ? (
        <MonthGrid {...props} selectedDate={data.date} lines={lines} onSelectDate={ui.setCalendarDate} />
      ) : (
        <TimeGrid {...props} now={data.now} onMove={onMove} onDraw={onDraw} />
      )}
      {scopeSheet}
      {notifySheet}
    </>
  );
}

interface CalendarModuleProps {
  data: CalendarData;
  resizeSidebar: (delta: number) => void;
  endDrag: () => void;
}

/** CalendarModule shows calendars, occurrences and the selected event. */
export const CalendarModule = forwardRef<CalendarHandle, CalendarModuleProps>(function CalendarModule(props, ref) {
  const ui = useUI();
  const client = useClient();
  const { ask, sheet } = useNotify();
  const sidebar = useRef<HTMLFieldSetElement>(null);
  const view = useRef<HTMLElement>(null);
  const reader = useRef<HTMLElement>(null);
  const [card, setCard] = useState<Omit<ContactCardContainerProps, "onClose"> | null>(null);
  const [deleting, setDeleting] = useState<CalendarEvent | null>(null);
  const data = props.data;
  const section = data.sections.find((section) =>
    section.calendars.some((calendar) => calendar.id === data.event?.calendarId),
  );
  const calendar = section?.calendars.find((calendar) => calendar.id === data.event?.calendarId);
  const event = data.event;
  const writable = !!event && !!calendar && !calendar.readOnly && !event.readOnly;
  const invitation = !!event?.organizer && !event.organizer.isUser;
  const edit = () => {
    if (!writable || !event) return false;
    void openEvent({ kind: "edit", id: event.id, recurrenceId: event.recurrenceId }, event.summary).catch(
      (err: unknown) => console.warn("edit event", err),
    );
    return true;
  };
  const deleteEvent = (params: CalendarDeleteParams) => {
    void withNotify((notify) => client.calendar.delete({ ...params, ...(notify ? { notify } : {}) }), ask)
      .then((result) => {
        if (result === null) return;
        rememberRestore({ module: "calendar", restoreId: result.restoreId });
        ui.selectOccurrence(null);
      })
      .catch((err: unknown) => console.warn("delete event", err));
  };
  const remove = () => {
    if (!writable || invitation || !event) return false;
    if (event.recurring && event.recurrenceId) setDeleting(event);
    else deleteEvent({ id: event.id });
    return true;
  };
  useImperativeHandle(ref, () => ({
    focus: (pane) => {
      if (pane === "sidebar") sidebar.current?.focus();
      else if (pane === "list") view.current?.focus();
      else reader.current?.focus();
    },
    scroll: (hours) => {
      const scroller = view.current?.querySelector<HTMLElement>(".overflow-y-auto");
      if (scroller) scroller.scrollTop += hours * 48;
    },
    edit,
    remove,
  }));
  return (
    <div className="flex min-h-0 flex-1">
      {ui.sidebarVisible && (
        <>
          <fieldset
            ref={sidebar}
            aria-label="Calendars"
            tabIndex={-1}
            onFocus={() => ui.setCalendarFocus("sidebar")}
            onClick={() => ui.setCalendarFocus("sidebar")}
            onKeyDown={() => ui.setCalendarFocus("sidebar")}
            className="flex h-full shrink-0 flex-col bg-sidebar outline-none"
            style={{ width: ui.sidebarWidth }}
          >
            <div className="min-h-0 flex-1 overflow-y-auto">
              <CalendarSidebar
                sections={data.sections}
                selected={data.date}
                today={data.today}
                weekStart={data.weekStart}
                busy={data.busy}
                locale={data.locale}
                onSelect={ui.setCalendarDate}
                onToggle={(id, enabled) => {
                  void client.account
                    .setCollection({ id, enabled })
                    .catch((err: unknown) => console.warn("calendar visibility", err));
                }}
                onMakeDefault={(id) => {
                  void client.account
                    .setCollection({ id, isDefault: true })
                    .catch((err: unknown) => console.warn("calendar default", err));
                }}
              />
            </div>
            <ModuleBar modules={["mail", "calendar", "people", "tasks"]} current={ui.module} onSelect={ui.setModule} />
          </fieldset>
          <Splitter label="Resize sidebar" onResize={props.resizeSidebar} onEnd={props.endDrag} />
        </>
      )}
      <section
        ref={view}
        aria-label="Calendar view"
        tabIndex={-1}
        onFocus={() => ui.setCalendarFocus("list")}
        onClick={() => ui.setCalendarFocus("list")}
        onKeyDown={() => ui.setCalendarFocus("list")}
        className="h-full min-w-0 flex-1 outline-none"
      >
        <CalendarView data={data} elementRef={view} />
      </section>
      <section
        ref={reader}
        aria-label="Event details"
        tabIndex={-1}
        onFocus={() => ui.setCalendarFocus("reader")}
        onClick={() => ui.setCalendarFocus("reader")}
        onKeyDown={() => ui.setCalendarFocus("reader")}
        className="h-full w-[320px] shrink-0 border-l border-separator outline-none"
      >
        <EventPane
          event={data.event}
          calendar={calendar && section ? { ...calendar, account: section.title } : null}
          timeZone={data.timeZone}
          locale={data.locale}
          onEdit={writable ? edit : undefined}
          onDelete={writable && !invitation ? remove : undefined}
          onPerson={(email, name, anchor) => {
            const rect = anchor.getBoundingClientRect();
            setCard({ address: { address: email, name }, at: { x: rect.left, y: rect.bottom } });
          }}
        />
      </section>
      {card && <ContactCardContainer key={card.address.address} {...card} onClose={() => setCard(null)} />}
      {deleting && (
        <ScopeSheet
          action="delete"
          first={deleting.first ?? false}
          onChoose={(scope) => {
            deleteEvent({ id: deleting.id, recurrenceId: deleting.recurrenceId, scope });
            setDeleting(null);
          }}
          onCancel={() => setDeleting(null)}
        />
      )}
      {sheet}
    </div>
  );
});
