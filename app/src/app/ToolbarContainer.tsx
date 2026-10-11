// The toolbar, fed by the stores and the list (docs/specs/ui.md, Toolbar).
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { type Ref, useCallback, useEffect, useMemo, useState } from "react";

import { useClient } from "../data/session";
import { listQuery, useMail, useUI } from "../data/stores";
import type { ViewModel } from "../data/view";
import { SearchField } from "../features/search/SearchField";
import { type MoveTarget, Toolbar, type ToolbarCommand } from "../features/toolbar/Toolbar";
import { step, viewTitle } from "../lib/calendarDates";
import { flagLabel } from "../lib/flags";
import { formatCount } from "../lib/format";
import { buildSidebar, vipGroups } from "../lib/mailboxTree";
import {
  archiveMailbox,
  compose,
  getMail,
  moveMessages,
  selectedSummaries,
  setFlagColor,
  toggleRead,
} from "./commands";
import { openSettings } from "./settings";
import { useCalendarFrame } from "./useCalendar";
import { type PeopleData, personEmail, writeToPerson } from "./usePeople";

import type { TasksData } from "./useTasks";

interface ToolbarContainerProps {
  tasks?: TasksData;
  onNewTask?: () => void;
  people?: PeopleData;
  model: ViewModel | null;
  onDelete: (ids: number[]) => void;
  searchRef: Ref<HTMLInputElement>;
}

function useMaximized() {
  const [maximized, setMaximized] = useState(false);
  useEffect(() => {
    if (!isTauri()) return;
    const win = getCurrentWindow();
    const update = () => void win.isMaximized().then(setMaximized);
    update();
    const off = win.onResized(update);
    return () => void off.then((stop) => stop());
  }, []);
  return maximized;
}

function mailTitle(model: ViewModel | null) {
  const ui = useUI.getState();
  const { accounts, mailboxes, vips, settings } = useMail.getState();
  const searching = ui.search !== "";
  const count = model?.ready ? model.count : null;
  if (searching) {
    return { title: "Search", subtitle: count === null ? "Searching…" : `${formatCount(count)} results` };
  }
  const src = ui.source;
  const inboxes = mailboxes.filter((mb) => mb.role === "inbox");
  const counts = (list: typeof mailboxes) => {
    const total = list.reduce((n, mb) => n + mb.total, 0);
    const unread = list.reduce((n, mb) => n + mb.unread, 0);
    return `${formatCount(total)} messages, ${formatCount(unread)} unread`;
  };
  if (src.kind === "allInboxes") return { title: "All Inboxes", subtitle: counts(inboxes) };
  if (src.kind === "role") {
    const label = buildSidebar(accounts, mailboxes)[0]?.items.find((i) => i.key === `role:${src.role}`)?.label;
    return { title: label ?? "", subtitle: counts(mailboxes.filter((mb) => mb.role === src.role)) };
  }
  if (src.kind === "flagged")
    return { title: "Flagged", subtitle: count === null ? "" : `${formatCount(count)} messages` };
  if (src.kind === "vips" || src.kind === "vip" || src.kind === "flagColor") {
    const title =
      src.kind === "vips"
        ? "VIPs"
        : src.kind === "flagColor"
          ? flagLabel(src.color, settings?.flagNames)
          : (vipGroups(vips).find((group) => group.key === src.key)?.label ?? "");
    return { title, subtitle: count === null ? "" : `${formatCount(count)} messages` };
  }
  const mb = mailboxes.find((m) => m.id === src.mailboxId);
  const label = buildSidebar(accounts, mailboxes)
    .flatMap((s) => s.items)
    .find((i) => i.mailboxId === src.mailboxId)?.label;
  const readOnly = accounts.some((a) => a.id === mb?.accountId && a.readOnly);
  const subtitle = mb ? `${counts([mb])}${readOnly ? " · Read-only" : ""}` : "";
  return { title: label ?? mb?.name ?? "", subtitle };
}

function useToolbarData(model: ViewModel | null, people?: PeopleData) {
  const { accounts, mailboxes, sync } = useMail();
  const ui = useUI();
  const rows = selectedSummaries(model, ui.selected);
  const selection = {
    count: ui.selected.length,
    seen: rows.length > 0 && rows.every((row) => row.flags.seen),
    flagColor: rows[0]?.flags.flagColor ?? 0,
  };
  const archive = archiveMailbox(model, ui.selected, mailboxes);
  const accountId = rows[0]?.accountId;
  const moveTargets = useMemo((): MoveTarget[] => {
    const section = buildSidebar(accounts, mailboxes).find((section) => section.accountId === accountId);
    return (section?.items ?? []).flatMap((item) =>
      item.mailboxId !== undefined && item.selectable
        ? [{ mailboxId: item.mailboxId, label: item.label, depth: item.depth }]
        : [],
    );
  }, [accounts, mailboxes, accountId]);
  const syncing = Object.values(sync).some(
    (status) => !["idle", "offline", "unauthorized", "failed"].includes(status.phase),
  );
  if (ui.module !== "people") return { ...mailTitle(model), selection, archive, moveTargets, syncing };
  const count = people?.people.length ?? 0;
  const title = ui.peopleBook === "all" ? "All Contacts" : (people?.books[ui.peopleBook]?.name ?? "");
  const subtitle = ui.peopleSearch
    ? `${formatCount(count)} results`
    : `${formatCount(count)} ${count === 1 ? "contact" : "contacts"}`;
  return { title, subtitle, selection, archive, moveTargets, syncing };
}

function windowCommand(command: ToolbarCommand): boolean {
  const ui = useUI.getState();
  switch (command.kind) {
    case "toggleSidebar":
      ui.toggleSidebar();
      break;
    case "settings":
      void openSettings().catch((err: unknown) => console.warn("settings", err));
      break;
    case "minimize":
      if (isTauri()) void getCurrentWindow().minimize();
      break;
    case "toggleMaximize":
      if (isTauri()) void getCurrentWindow().toggleMaximize();
      break;
    case "close":
      if (isTauri()) void getCurrentWindow().close();
      break;
    default:
      return false;
  }
  return true;
}

function useToolbarCommand(props: ToolbarContainerProps) {
  const client = useClient();
  const frame = useCalendarFrame();
  return useCallback(
    (command: ToolbarCommand) => {
      if (windowCommand(command)) return;
      const ui = useUI.getState();
      if (command.kind === "toggleTodoBar") {
        ui.toggleTodoBar();
        return;
      }
      if (ui.module === "tasks") {
        if (command.kind === "newTask") props.onNewTask?.();
        else if (command.kind === "toggleCompleted") ui.toggleShowCompleted();
        return;
      }
      if (ui.module === "calendar") {
        if (command.kind === "today") ui.setCalendarDate(frame.today);
        else if (command.kind === "calendarView") ui.setCalendarView(command.view);
        else if (command.kind === "previousPeriod" || command.kind === "nextPeriod")
          ui.setCalendarDate(
            step(ui.calendarView, ui.calendarDate || frame.today, command.kind === "previousPeriod" ? -1 : 1),
          );
        return;
      }
      if (ui.module === "people") {
        if (command.kind === "compose") writeToPerson(client, personEmail(props.people?.person ?? null));
        return;
      }
      const { accounts, mailboxes } = useMail.getState();
      const ids = ui.selected;
      switch (command.kind) {
        case "getMail":
          void getMail(
            client,
            accounts.map((account) => account.id),
          );
          break;
        case "delete":
          if (ids.length > 0) props.onDelete(ids);
          break;
        case "archive":
          void moveMessages(client, ids, archiveMailbox(props.model, ids, mailboxes), ui.source, mailboxes);
          break;
        case "flag":
          void setFlagColor(client, ids, command.color);
          break;
        case "toggleRead":
          void toggleRead(client, props.model, ids);
          break;
        case "move":
          void moveMessages(
            client,
            ids,
            mailboxes.find((mailbox) => mailbox.id === command.mailboxId),
            ui.source,
            mailboxes,
          );
          break;
        case "compose":
        case "reply":
        case "replyAll":
        case "forward":
          void compose(client, command.kind, ids).catch((err: unknown) => console.warn("compose", err));
          break;
      }
    },
    [client, props, frame.today],
  );
}

function ToolbarSearch({ searchRef }: Pick<ToolbarContainerProps, "searchRef">) {
  const ui = useUI();
  const inPeople = ui.module === "people";
  const onSearch = (text: string) => {
    if (inPeople) {
      if (text === "") ui.clearPeopleSearch();
      else ui.commitPeopleSearch(text);
    } else if (text === "") ui.clearSearch();
    else ui.commitSearch(text);
  };
  return (
    <SearchField
      value={inPeople ? ui.peopleSearchDraft : ui.searchDraft}
      placeholder={inPeople ? "Search Contacts" : "Search"}
      onChange={inPeople ? ui.setPeopleSearchDraft : ui.setSearchDraft}
      onSearch={onSearch}
      onClear={inPeople ? ui.clearPeopleSearch : ui.clearSearch}
      inputRef={searchRef}
    />
  );
}

/** ToolbarContainer connects Toolbar to the active module and the window. */
export function ToolbarContainer(props: ToolbarContainerProps) {
  const ui = useUI();
  const flagNames = useMail((s) => s.settings?.flagNames);
  const data = useToolbarData(props.model, props.people);
  const frame = useCalendarFrame();
  const maximized = useMaximized();
  const onCommand = useToolbarCommand(props);
  return (
    <Toolbar
      mode={ui.module}
      tasks={
        ui.module === "tasks"
          ? {
              title: props.tasks?.title ?? "",
              canCreate:
                ui.tasksSource !== "flagged" &&
                !props.tasks?.sections.some((section) =>
                  section.lists.some((list) => list.id === ui.tasksSource && list.readOnly),
                ),
              showCompleted: ui.tasksSource === "today" || ui.tasksSource === "flagged" ? null : ui.tasksShowCompleted,
              paneWidth: 320,
            }
          : undefined
      }
      todoBar={ui.module === "mail" ? ui.todoBar : undefined}
      calendar={
        ui.module === "calendar"
          ? {
              view: ui.calendarView,
              title: viewTitle(ui.calendarView, frame.date, frame.weekStart, frame.locale),
              paneWidth: 320,
            }
          : undefined
      }
      sidebarWidth={ui.sidebarVisible ? ui.sidebarWidth : 0}
      listWidth={ui.listWidth}
      title={data.title}
      subtitle={data.subtitle}
      syncing={data.syncing}
      selection={data.selection}
      flagNames={flagNames}
      canArchive={data.archive !== undefined}
      moveTargets={data.moveTargets}
      maximized={maximized}
      onCommand={onCommand}
      search={<ToolbarSearch searchRef={props.searchRef} />}
    />
  );
}

/** useListQuery is the list's view query for the UI state. */
export function useListQuery() {
  const source = useUI((state) => state.source);
  const search = useUI((state) => state.search);
  const searchScope = useUI((state) => state.searchScope);
  const conversations = useUI((state) => state.conversations);
  const listFilter = useUI((state) => state.listFilter);
  return useMemo(
    () => listQuery({ source, search, searchScope, conversations, listFilter }),
    [source, search, searchScope, conversations, listFilter],
  );
}
