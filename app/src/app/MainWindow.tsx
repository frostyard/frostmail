// The modular main window, shared pane resizing and keyboard dispatch.
import { type RefObject, useCallback, useEffect, useRef } from "react";

import { useClient } from "../data/session";
import { type Pane, useMail, useUI } from "../data/stores";
import { useView } from "../data/useView";
import type { ViewModel } from "../data/view";
import { ScopeBar } from "../features/search/SearchField";
import { addDays, step } from "../lib/calendarDates";
import { type Command, commandFor } from "../lib/keymap";
import type { Client } from "../rpc/gen/api";
import { type CalendarHandle, CalendarModule } from "./CalendarModule";
import { archiveMailbox, compose, getMail, moveMessages, toggleFlag, toggleRead } from "./commands";
import { ListContainer, type ListHandle } from "./ListContainer";
import { UndoToasts } from "./OutboxContainer";
import { watchOpenRequests } from "./openMessage";
import { type PeopleHandle, PeopleModule, peopleCommand } from "./PeopleModule";
import { ReaderContainer, type ReaderHandle } from "./ReaderContainer";
import { SidebarContainer } from "./SidebarContainer";
import { Splitter } from "./Splitter";
import { openSettings } from "./settings";
import { ToolbarContainer, useListQuery } from "./ToolbarContainer";
import { type CalendarData, useCalendar } from "./useCalendar";
import { type PeopleData, personEmail, usePeople, writeToPerson } from "./usePeople";

const SIDEBAR = { min: 160, max: 320 };
const LIST = { min: 280, max: 560 };
const READER_MIN = 360;
const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));

function inTextField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA";
}

interface WindowHandles {
  list: RefObject<ListHandle | null>;
  reader: RefObject<ReaderHandle | null>;
  people: RefObject<PeopleHandle | null>;
  calendar: RefObject<CalendarHandle | null>;
  search: RefObject<HTMLInputElement | null>;
  sidebar: RefObject<HTMLDivElement | null>;
}

function focusPane(handles: WindowHandles, direction: 1 | -1) {
  const ui = useUI.getState();
  const panes: Pane[] = ui.sidebarVisible ? ["sidebar", "list", "reader"] : ["list", "reader"];
  const focus = ui.module === "calendar" ? ui.calendarFocus : ui.module === "people" ? ui.peopleFocus : ui.focus;
  const index = panes.indexOf(focus);
  const next = panes[(index + direction + panes.length) % panes.length] ?? "list";
  if (ui.module === "calendar") {
    ui.setCalendarFocus(next);
    handles.calendar.current?.focus(next);
  } else if (ui.module === "people") {
    ui.setPeopleFocus(next);
    handles.people.current?.focus(next);
  } else {
    ui.setFocus(next);
    if (next === "list") handles.list.current?.focus();
    else if (next === "reader") handles.reader.current?.focus();
    else handles.sidebar.current?.querySelector<HTMLElement>("[tabindex='0'],button")?.focus();
  }
}

function mailCommand(command: Command, client: Client, model: ViewModel | null, handles: WindowHandles) {
  const ui = useUI.getState();
  const { accounts, mailboxes } = useMail.getState();
  const ids = ui.selected;
  switch (command) {
    case "escape":
      if (ui.search === "" && ui.searchDraft === "") return false;
      ui.clearSearch();
      break;
    case "getMail":
      void getMail(
        client,
        accounts.map((account) => account.id),
      );
      break;
    case "toggleRead":
      void toggleRead(client, model, ids);
      break;
    case "toggleFlag":
      void toggleFlag(client, model, ids);
      break;
    case "archive":
      void moveMessages(client, ids, archiveMailbox(model, ids, mailboxes), ui.source, mailboxes);
      break;
    case "pageDown":
    case "pageUp":
      if (ui.focus === "sidebar") return false;
      handles.reader.current?.page(command === "pageDown" ? 1 : -1);
      break;
    case "compose":
    case "reply":
    case "replyAll":
    case "forward":
      void compose(client, command, ids).catch((err: unknown) => console.warn("compose", err));
      break;
    default:
      return ui.focus === "list" && (handles.list.current?.command(command) ?? false);
  }
  return true;
}

function moduleCommand(command: Command, client: Client, people: PeopleData) {
  const ui = useUI.getState();
  if (command === "showCalendar") ui.setModule("calendar");
  else if (command === "showPeople") ui.setModule("people");
  else if (command === "allInboxes") {
    ui.setModule("mail");
    ui.setSource({ kind: "allInboxes" });
  } else if (command === "escape") ui.clearPeopleSearch();
  else if (command === "compose") writeToPerson(client, ui.peopleFocus === "list" ? personEmail(people.person) : "");
  else return ui.peopleFocus === "list" && peopleCommand(command, people.people);
  return true;
}

function calendarCommand(command: Command, frame: CalendarData, handles: WindowHandles): boolean {
  const ui = useUI.getState();
  const date = ui.calendarDate || frame.today;
  switch (command) {
    case "dayView":
      ui.setCalendarView("day");
      break;
    case "weekView":
      ui.setCalendarView("week");
      break;
    case "monthView":
      ui.setCalendarView("month");
      break;
    case "today":
      ui.setCalendarDate(frame.today);
      break;
    case "previousPeriod":
    case "nextPeriod":
      ui.setCalendarDate(step(ui.calendarView, date, command === "previousPeriod" ? -1 : 1));
      break;
    case "left":
    case "right":
      if (ui.calendarFocus !== "list") return false;
      ui.setCalendarDate(addDays(date, command === "left" ? -1 : 1));
      break;
    case "previous":
    case "next": {
      if (ui.calendarFocus !== "list") return false;
      const direction = command === "previous" ? -1 : 1;
      if (ui.calendarView === "month") ui.setCalendarDate(addDays(date, direction * 7));
      else handles.calendar.current?.scroll(direction);
      break;
    }
    case "open":
      if (ui.calendarFocus !== "list") return false;
      ui.setCalendarDate(date);
      ui.setCalendarView("day");
      break;
    case "escape":
      ui.selectOccurrence(null);
      break;
    default:
      return false;
  }
  return true;
}

function useWindowEvents(handles: WindowHandles, model: ViewModel | null, people: PeopleData, calendar: CalendarData) {
  const client = useClient();
  useEffect(() => watchOpenRequests(client), [client]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const ui = useUI.getState();
      const field = inTextField(event.target);
      const command = commandFor(
        event,
        field && !(ui.module === "people" && ["f", "n"].includes(event.key.toLowerCase())),
      );
      if (!command || command === "showTasks") return;
      let handled = true;
      if (command === "focusSearch") {
        if (ui.module === "calendar") return;
        handles.search.current?.focus();
        handles.search.current?.select();
      } else if (command === "toggleSidebar") ui.toggleSidebar();
      else if (command === "nextPane" || command === "previousPane")
        focusPane(handles, command === "nextPane" ? 1 : -1);
      else if (command === "settings") void openSettings().catch((err: unknown) => console.warn("settings", err));
      else if (
        command === "showCalendar" ||
        command === "showPeople" ||
        command === "allInboxes" ||
        ui.module === "people"
      ) {
        handled = moduleCommand(command, client, people);
      } else if (ui.module === "calendar") handled = calendarCommand(command, calendar, handles);
      else handled = mailCommand(command, client, model, handles);
      if (handled) event.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [client, handles, model, people, calendar]);
  useEffect(() => {
    const block = (event: MouseEvent) => {
      if (!inTextField(event.target)) event.preventDefault();
    };
    window.addEventListener("contextmenu", block);
    return () => window.removeEventListener("contextmenu", block);
  }, []);
}

function usePaneWidths() {
  const ui = useUI();
  const dragStart = useRef({ sidebar: ui.sidebarWidth, list: ui.listWidth });
  const width = typeof window === "undefined" ? 1200 : window.innerWidth;
  const resizeSidebar = (delta: number) =>
    ui.setWidths({ sidebarWidth: clamp(dragStart.current.sidebar + delta, SIDEBAR.min, SIDEBAR.max) });
  const resizeList = (delta: number) =>
    ui.setWidths({
      listWidth: clamp(
        dragStart.current.list + delta,
        LIST.min,
        Math.min(LIST.max, width - READER_MIN - (ui.sidebarVisible ? ui.sidebarWidth : 0)),
      ),
    });
  const endDrag = () => {
    dragStart.current = { sidebar: useUI.getState().sidebarWidth, list: useUI.getState().listWidth };
  };
  return { resizeSidebar, resizeList, endDrag };
}

function MailScope() {
  const ui = useUI();
  const mailboxes = useMail((state) => state.mailboxes);
  const source = ui.source;
  const label =
    source.kind === "mailbox"
      ? (mailboxes.find((mailbox) => mailbox.id === source.mailboxId)?.name ?? "Mailbox")
      : source.kind === "allInboxes"
        ? "All Inboxes"
        : "Flagged";
  if (ui.search === "") return null;
  return (
    <ScopeBar
      scopes={[
        { key: "all", label: "All Mailboxes" },
        { key: "source", label },
      ]}
      selected={ui.searchScope}
      onSelect={(key) => ui.setSearchScope(key === "source" ? "source" : "all")}
    />
  );
}

function MailPanes(props: {
  handles: WindowHandles;
  model: ViewModel | null;
  onDelete: (ids: number[]) => void;
  widths: ReturnType<typeof usePaneWidths>;
}) {
  const ui = useUI();
  return (
    <div className="flex min-h-0 flex-1">
      {ui.sidebarVisible && (
        <>
          <div ref={props.handles.sidebar} className="h-full shrink-0" style={{ width: ui.sidebarWidth }}>
            <SidebarContainer />
          </div>
          <Splitter label="Resize sidebar" onResize={props.widths.resizeSidebar} onEnd={props.widths.endDrag} />
        </>
      )}
      <div className="h-full shrink-0" style={{ width: ui.listWidth }}>
        <ListContainer ref={props.handles.list} model={props.model} onDelete={props.onDelete} />
      </div>
      <Splitter label="Resize message list" onResize={props.widths.resizeList} onEnd={props.widths.endDrag} />
      <div className="h-full min-w-0 flex-1">
        <ReaderContainer ref={props.handles.reader} />
      </div>
    </div>
  );
}

// Keep a newly revealed selection until its matching Mail view has opened.
function useMailModel() {
  const query = useListQuery();
  const model = useView(query);
  useEffect(() => model?.ensure(0, 50), [model]);
  const matches = model && JSON.stringify(model.query) === JSON.stringify(query);
  return matches && model.ready ? model : null;
}

/** MainWindow lays out the active module and owns Mail's view and People's data. */
export function MainWindow() {
  const client = useClient();
  const module = useUI((state) => state.module);
  const people = usePeople();
  const calendar = useCalendar();
  const model = useMailModel();
  const handles: WindowHandles = {
    list: useRef<ListHandle>(null),
    reader: useRef<ReaderHandle>(null),
    people: useRef<PeopleHandle>(null),
    calendar: useRef<CalendarHandle>(null),
    search: useRef<HTMLInputElement>(null),
    sidebar: useRef<HTMLDivElement>(null),
  };
  const widths = usePaneWidths();
  useWindowEvents(handles, model, people, calendar);
  const onDelete = useCallback(
    (ids: number[]) => {
      void client.message.delete({ ids }).catch((err: unknown) => console.warn("delete", err));
    },
    [client],
  );
  return (
    <div className="flex h-full flex-col">
      <ToolbarContainer key={module} people={people} model={model} onDelete={onDelete} searchRef={handles.search} />
      {module === "mail" && <MailScope />}
      {module === "people" ? (
        <PeopleModule ref={handles.people} data={people} {...widths} />
      ) : module === "calendar" ? (
        <CalendarModule ref={handles.calendar} data={calendar} {...widths} />
      ) : (
        <MailPanes handles={handles} model={model} onDelete={onDelete} widths={widths} />
      )}
      <UndoToasts />
    </div>
  );
}
