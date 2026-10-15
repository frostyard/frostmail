// The modular main window, shared pane resizing and keyboard dispatch.
import { type RefObject, useCallback, useEffect, useLayoutEffect, useRef } from "react";

import { useClient } from "../data/session";
import { type Pane, useMail, useUI } from "../data/stores";
import { useView } from "../data/useView";
import type { ViewModel } from "../data/view";
import { useNotify } from "../features/calendar/NotifySheet";
import { FilterBar } from "../features/list/FilterBar";
import { SortMenu } from "../features/list/SortMenu";
import { ScopeBar } from "../features/search/SearchField";
import { addDays, step } from "../lib/calendarDates";
import { flagLabel } from "../lib/flags";
import { type Command, commandFor } from "../lib/keymap";
import { vipGroups } from "../lib/mailboxTree";
import type { Client, InviteNotice } from "../rpc/gen/api";
import { type CalendarHandle, CalendarModule } from "./CalendarModule";
import { archiveMailbox, compose, getMail, moveMessages, toggleFlag, toggleRead, toggleSpam } from "./commands";
import { newEventAt, openEvent } from "./eventWindow";
import { ListContainer, type ListHandle } from "./ListContainer";
import { UndoToasts } from "./OutboxContainer";
import { watchOpenRequests } from "./openMessage";
import { type PeopleHandle, PeopleModule, peopleCommand } from "./PeopleModule";
import { undoPim, watchRestores } from "./pimUndo";
import { ReaderContainer, type ReaderHandle } from "./ReaderContainer";
import { RefusedNotices } from "./RefusedNotices";
import { watchOccurrenceRequests, watchReminders } from "./reminders";
import { SidebarContainer } from "./SidebarContainer";
import { SmartSheetContainer } from "./SmartSheetContainer";
import { Splitter } from "./Splitter";
import { openSettings } from "./settings";
import { type TasksHandle, TasksModule, tasksCommand } from "./TasksModule";
import { ToDoBarContainer } from "./ToDoBarContainer";
import { ToolbarContainer, useListQuery } from "./ToolbarContainer";
import { undo, undoableMove } from "./undo";
import { type CalendarData, useCalendar } from "./useCalendar";
import { type PeopleData, usePeople } from "./usePeople";
import { type TasksData, useTasks } from "./useTasks";

const SIDEBAR = { min: 160, max: 320 };
const LIST = { min: 280, max: 560 };
const READER_MIN = 360;
const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));

function inTextField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA";
}

interface WindowHandles {
  ask: (notice: InviteNotice) => Promise<boolean>;
  list: RefObject<ListHandle | null>;
  reader: RefObject<ReaderHandle | null>;
  people: RefObject<PeopleHandle | null>;
  calendar: RefObject<CalendarHandle | null>;
  tasks: RefObject<TasksHandle | null>;
  search: RefObject<HTMLInputElement | null>;
  sidebar: RefObject<HTMLDivElement | null>;
}

function focusPane(handles: WindowHandles, direction: 1 | -1) {
  const ui = useUI.getState();
  const panes: Pane[] = ui.sidebarVisible ? ["sidebar", "list", "reader"] : ["list", "reader"];
  const focus =
    ui.module === "tasks"
      ? ui.tasksFocus
      : ui.module === "calendar"
        ? ui.calendarFocus
        : ui.module === "people"
          ? ui.peopleFocus
          : ui.focus;
  const index = panes.indexOf(focus);
  const next = panes[(index + direction + panes.length) % panes.length] ?? "list";
  if (ui.module === "tasks") {
    ui.setTasksFocus(next);
    handles.tasks.current?.focus(next);
  } else if (ui.module === "calendar") {
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
    case "undo":
      void undo(client).catch((err: unknown) => console.warn("undo", err));
      break;
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
    case "junk":
      void toggleSpam(client, model, ids, ui.source, mailboxes);
      break;
    case "redirect":
      return handles.list.current?.command("redirect") ?? false;
    case "print":
      if (ids.length !== 1) return false;
      window.print();
      break;
    case "rawSource":
    case "allHeaders":
      if (ids.length !== 1) return false;
      handles.reader.current?.more(command === "rawSource" ? "source" : "headers");
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
  if (command === "showTasks") ui.setModule("tasks");
  else if (command === "showCalendar") ui.setModule("calendar");
  else if (command === "showPeople") ui.setModule("people");
  else if (command === "allInboxes") {
    ui.setModule("mail");
    ui.setSource({ kind: "allInboxes" });
  } else if (command === "undo") {
    void undoPim(client, "people").catch((err: unknown) => console.warn("undo people", err));
  } else if (command === "escape") ui.clearPeopleSearch();
  else if (command === "compose") {
    if (people.writableBooks.length > 0) ui.editContact("new");
  } else return ui.peopleFocus === "list" && peopleCommand(command, people.people);
  return true;
}

function canCreateEvent(data: CalendarData): boolean {
  return data.sections.some((section) => section.calendars.some((calendar) => !calendar.readOnly));
}

function newCalendarEvent(data: CalendarData) {
  const date = useUI.getState().calendarDate || data.today;
  void openEvent(newEventAt(date, data.today, data.now, data.timeZone)).catch((err: unknown) =>
    console.warn("new event", err),
  );
}

function calendarCommand(command: Command, client: Client, frame: CalendarData, handles: WindowHandles): boolean {
  const ui = useUI.getState();
  const date = ui.calendarDate || frame.today;
  switch (command) {
    case "undo":
      void undoPim(client, "calendar", handles.ask).catch((err: unknown) => console.warn("undo calendar", err));
      break;
    case "compose":
      if (!canCreateEvent(frame)) return false;
      newCalendarEvent(frame);
      break;
    case "delete":
      if (!ui.calendarSelected) return false;
      return handles.calendar.current?.remove() ?? false;
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
      if (ui.calendarSelected && handles.calendar.current?.edit()) return true;
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

function useWindowEvents(
  handles: WindowHandles,
  model: ViewModel | null,
  people: PeopleData,
  calendar: CalendarData,
  tasks: TasksData,
) {
  const client = useClient();
  useEffect(() => watchOpenRequests(client), [client]);
  useEffect(() => watchReminders(client), [client]);
  useEffect(() => watchRestores(), []);
  useLayoutEffect(() => watchOccurrenceRequests(), []);
  // Install input handlers at commit: the visible window must already be
  // ready for keys and occurrence requests before passive data effects run.
  useLayoutEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const ui = useUI.getState();
      if (ui.smartSheet) return;
      const field = inTextField(event.target);
      const command = commandFor(
        event,
        field &&
          !(
            (ui.module === "people" && ["f", "n"].includes(event.key.toLowerCase())) ||
            ((ui.module === "tasks" || ui.module === "calendar") && event.key.toLowerCase() === "n")
          ),
      );
      if (!command) return;
      if (ui.module === "people" && ui.peopleEditing !== null) {
        // Keep native editing, form submission and Tab navigation in the editor.
        if (command !== "undo" && (event.ctrlKey || event.metaKey || event.altKey)) event.preventDefault();
        return;
      }
      let handled = true;
      if (command === "focusSearch") {
        if (ui.module === "calendar" || ui.module === "tasks") return;
        handles.search.current?.focus();
        handles.search.current?.select();
      } else if (command === "toggleSidebar") ui.toggleSidebar();
      else if (command === "nextPane" || command === "previousPane")
        focusPane(handles, command === "nextPane" ? 1 : -1);
      else if (command === "settings") void openSettings().catch((err: unknown) => console.warn("settings", err));
      else if (
        command === "showTasks" ||
        command === "showCalendar" ||
        command === "showPeople" ||
        command === "allInboxes" ||
        ui.module === "people"
      ) {
        handled = moduleCommand(command, client, people);
      } else if (ui.module === "tasks") handled = tasksCommand(command, tasks, handles.tasks.current);
      else if (ui.module === "calendar") handled = calendarCommand(command, client, calendar, handles);
      else handled = mailCommand(command, client, model, handles);
      if (handled) event.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [client, handles, model, people, calendar, tasks]);
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
  const client = useClient();
  const ui = useUI();
  const { mailboxes, vips, settings, smarts } = useMail();
  const source = ui.source;
  const label =
    source.kind === "smart"
      ? (smarts.find((smart) => smart.id === source.id)?.name ?? "")
      : source.kind === "mailbox"
        ? (mailboxes.find((mailbox) => mailbox.id === source.mailboxId)?.name ?? "Mailbox")
        : source.kind === "allInboxes"
          ? "All Inboxes"
          : source.kind === "vips"
            ? "VIPs"
            : source.kind === "vip"
              ? (vipGroups(vips).find((group) => group.key === source.key)?.label ?? "")
              : source.kind === "flagColor"
                ? flagLabel(source.color, settings?.flagNames)
                : "Flagged";
  if (ui.search === "") return null;
  return (
    <ScopeBar
      scopes={[
        { key: "all", label: "All Mailboxes" },
        { key: "source", label },
      ]}
      selected={ui.searchScope}
      onSave={() => {
        const text = ui.search;
        const mailbox = ui.searchScope === "source" && source.kind === "mailbox" ? { mailboxId: source.mailboxId } : {};
        void client.smart
          .fromSearch({ text, ...mailbox })
          .then((conditions) => {
            ui.openSmartSheet({
              mode: "new",
              initial: { name: text, conditions, includeTrash: true, includeSent: true },
            });
          })
          .catch((err: unknown) => console.warn("save search", err));
      }}
      onSelect={(key) => ui.setSearchScope(key === "source" ? "source" : "all")}
    />
  );
}

function MailPanes(props: {
  tasks: TasksData;
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
      <div className="flex h-full shrink-0 flex-col" style={{ width: ui.listWidth }}>
        <div className="flex h-[32px] shrink-0 items-center border-b border-separator bg-window pr-3">
          <div className="min-w-0 flex-1 overflow-x-auto [&>div]:w-max [&>div]:border-b-0">
            <FilterBar filter={ui.listFilter} onChange={ui.setListFilter} />
          </div>
          <SortMenu
            sort={ui.sort}
            ascending={ui.ascending}
            conversations={ui.conversations}
            contactPhotos={ui.contactPhotos}
            onSort={ui.setSort}
            onAscending={ui.setAscending}
            onConversations={ui.setConversations}
            onContactPhotos={ui.setContactPhotos}
          />
        </div>
        <div className="min-h-0 flex-1">
          <ListContainer ref={props.handles.list} model={props.model} onDelete={props.onDelete} />
        </div>
      </div>
      <Splitter label="Resize message list" onResize={props.widths.resizeList} onEnd={props.widths.endDrag} />
      <div className="h-full min-w-0 flex-1">
        <ReaderContainer ref={props.handles.reader} />
      </div>
      {ui.todoBar && <ToDoBarContainer tasks={props.tasks} />}
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
  const { ask, sheet } = useNotify();
  const module = useUI((state) => state.module);
  const people = usePeople();
  const calendar = useCalendar();
  const tasks = useTasks();
  const model = useMailModel();
  const handles: WindowHandles = {
    ask,
    list: useRef<ListHandle>(null),
    reader: useRef<ReaderHandle>(null),
    people: useRef<PeopleHandle>(null),
    calendar: useRef<CalendarHandle>(null),
    tasks: useRef<TasksHandle>(null),
    search: useRef<HTMLInputElement>(null),
    sidebar: useRef<HTMLDivElement>(null),
  };
  const widths = usePaneWidths();
  useWindowEvents(handles, model, people, calendar, tasks);
  const onDelete = useCallback(
    (ids: number[]) => {
      void undoableMove(client, ids, () => client.message.delete({ ids })).catch((err: unknown) =>
        console.warn("delete", err),
      );
    },
    [client],
  );
  return (
    <div className="flex h-full flex-col">
      <ToolbarContainer
        key={module}
        tasks={tasks}
        onNewTask={() => handles.tasks.current?.focusNewTask()}
        onNewEvent={canCreateEvent(calendar) ? () => newCalendarEvent(calendar) : undefined}
        people={people}
        model={model}
        onDelete={onDelete}
        searchRef={handles.search}
      />
      {module === "mail" && <MailScope />}
      {module === "people" ? (
        <PeopleModule ref={handles.people} data={people} {...widths} />
      ) : module === "tasks" ? (
        <TasksModule ref={handles.tasks} data={tasks} {...widths} />
      ) : module === "calendar" ? (
        <CalendarModule ref={handles.calendar} data={calendar} {...widths} />
      ) : (
        <MailPanes tasks={tasks} handles={handles} model={model} onDelete={onDelete} widths={widths} />
      )}
      <UndoToasts />
      <RefusedNotices />
      <SmartSheetContainer />
      {sheet}
    </div>
  );
}
