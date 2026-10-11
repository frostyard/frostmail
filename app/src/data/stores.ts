// The app's stores (docs/design/app.md, Data): what maild reports about
// accounts, mailboxes and sync, and the window's UI state.
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import type { OccurrenceKey } from "../features/calendar/TimeGrid";
import type { ListFilter } from "../features/list/FilterBar";
import type { SmartDraft } from "../features/organize/SmartSheet";
import type { CalendarView } from "../lib/calendarDates";
import { vipGroups } from "../lib/mailboxTree";
import type { Module } from "../lib/modules";
import type { TasksSource } from "../lib/taskText";
import type {
  Account,
  Mailbox,
  MailboxRole,
  OutboxItem,
  Rule,
  Settings,
  SmartMailbox,
  SyncStatus,
  ViewQuery,
  ViewSort,
  Vip,
} from "../rpc/gen/api";

/** Connection is the state of the link to maild. */
export type Connection = { state: "connecting" } | { state: "ready" } | { state: "lost"; reason: string };

/** MailState mirrors maild's accounts, mailboxes and sync status. */
export interface MailState {
  connection: Connection;
  accounts: Account[];
  mailboxes: Mailbox[];
  sync: Record<number, SyncStatus>;
  /** Messages not yet sent (outbox.list). */
  outbox: OutboxItem[];
  /** VIP addresses (vip.list), in its order. */
  vips: Vip[];
  /** maild's preferences (settings.get); null until loaded. */
  settings: Settings | null;
  /** Smart mailboxes (smart.list), in sidebar order. */
  smarts: SmartMailbox[];
  /** Rules (rule.list), in the order they run. */
  rules: Rule[];
}

/** useMail is the store of maild data. */
export const useMail = create<MailState>(() => ({
  connection: { state: "connecting" },
  accounts: [],
  mailboxes: [],
  sync: {},
  outbox: [],
  vips: [],
  settings: null,
  smarts: [],
  rules: [],
}));

/** Source is what the sidebar selected. */
export type Source =
  | { kind: "smart"; id: number }
  | { kind: "mailbox"; mailboxId: number }
  | { kind: "allInboxes" }
  | { kind: "role"; role: MailboxRole }
  | { kind: "flagged" }
  | { kind: "flagColor"; color: number }
  | { kind: "vips" }
  | { kind: "vip"; key: string; addresses: string[] };

/** SmartSheetState identifies the smart mailbox being made or edited. */
export type SmartSheetState = { mode: "new"; initial?: SmartDraft } | { mode: "edit"; id: number };

/** Pane is a focusable area of the window. */
export type Pane = "sidebar" | "list" | "reader";

/** UIState is the window's own state. */
export interface UIState {
  smartSheet: SmartSheetState | null;
  module: Module;
  calendarView: CalendarView;
  calendarDate: string;
  calendarSelected: OccurrenceKey | null;
  calendarFocus: Pane;
  tasksSource: TasksSource;
  tasksSelected: number | null;
  tasksFocus: Pane;
  tasksShowCompleted: boolean;
  todoBar: boolean;
  peopleBook: "all" | number;
  peopleSelected: number | null;
  peopleSearch: string;
  peopleSearchDraft: string;
  peopleFocus: Pane;
  source: Source;
  /** The committed search ("" when not searching). */
  search: string;
  /** The text in the search field. */
  searchDraft: string;
  /** "all" or "source": where a search looks. */
  searchScope: "all" | "source";
  listFilter: ListFilter;
  /** Selected message IDs; the first is the one the reader shows. */
  selected: number[];
  /** The row a Shift-click or Shift-arrow extends from. */
  anchor: number | null;
  focus: Pane;
  sidebarVisible: boolean;
  sidebarWidth: number;
  listWidth: number;
  conversations: boolean;
  sort: ViewSort;
  ascending: boolean;
  contactPhotos: boolean;
}

/** UIActions change UIState. */
export interface UIActions {
  openSmartSheet: (state: SmartSheetState) => void;
  closeSmartSheet: () => void;
  setCalendarView: (view: CalendarView) => void;
  setCalendarDate: (date: string) => void;
  selectOccurrence: (key: OccurrenceKey | null, date?: string) => void;
  setCalendarFocus: (pane: Pane) => void;
  setModule: (module: Module) => void;
  setTasksSource: (source: TasksSource) => void;
  selectTask: (id: number | null) => void;
  setTasksFocus: (pane: Pane) => void;
  toggleShowCompleted: () => void;
  toggleTodoBar: () => void;
  setPeopleBook: (peopleBook: "all" | number) => void;
  selectPerson: (peopleSelected: number | null) => void;
  setPeopleSearchDraft: (peopleSearchDraft: string) => void;
  commitPeopleSearch: (peopleSearch: string) => void;
  clearPeopleSearch: () => void;
  setPeopleFocus: (peopleFocus: Pane) => void;
  setSource: (s: Source) => void;
  setSearchDraft: (text: string) => void;
  commitSearch: (text: string) => void;
  clearSearch: () => void;
  setSearchScope: (scope: "all" | "source") => void;
  setListFilter: (filter: ListFilter) => void;
  setSort: (sort: ViewSort) => void;
  setAscending: (ascending: boolean) => void;
  setConversations: (conversations: boolean) => void;
  setContactPhotos: (contactPhotos: boolean) => void;
  select: (ids: number[], anchor?: number | null) => void;
  setFocus: (p: Pane) => void;
  toggleSidebar: () => void;
  setWidths: (w: { sidebarWidth?: number; listWidth?: number }) => void;
}

const initialUI: UIState = {
  smartSheet: null,
  module: "mail",
  calendarView: "week",
  calendarDate: "",
  calendarSelected: null,
  calendarFocus: "list",
  tasksSource: "today",
  tasksSelected: null,
  tasksFocus: "list",
  tasksShowCompleted: false,
  todoBar: false,
  peopleBook: "all",
  peopleSelected: null,
  peopleSearch: "",
  peopleSearchDraft: "",
  peopleFocus: "list",
  source: { kind: "allInboxes" },
  search: "",
  searchDraft: "",
  searchScope: "all",
  listFilter: "all",
  selected: [],
  anchor: null,
  focus: "list",
  sidebarVisible: true,
  sidebarWidth: 220,
  listWidth: 360,
  conversations: true,
  sort: "date",
  ascending: false,
  contactPhotos: false,
};

/** useUI is the window's UI store; layout preferences persist. */
export const useUI = create<UIState & UIActions>()(
  persist(
    (set) => ({
      ...initialUI,
      openSmartSheet: (smartSheet) => set({ smartSheet }),
      closeSmartSheet: () => set({ smartSheet: null }),
      setCalendarView: (calendarView) => set({ calendarView }),
      setCalendarDate: (calendarDate) => set({ calendarDate }),
      selectOccurrence: (calendarSelected, date) =>
        set({ calendarSelected, ...(date === undefined ? {} : { calendarDate: date }) }),
      setCalendarFocus: (calendarFocus) => set({ calendarFocus }),
      setModule: (module) => set({ module }),
      setTasksSource: (tasksSource) => set({ tasksSource, tasksSelected: null }),
      selectTask: (tasksSelected) => set({ tasksSelected }),
      setTasksFocus: (tasksFocus) => set({ tasksFocus }),
      toggleShowCompleted: () => set((s) => ({ tasksShowCompleted: !s.tasksShowCompleted })),
      toggleTodoBar: () => set((s) => ({ todoBar: !s.todoBar })),
      setPeopleBook: (peopleBook) => set({ peopleBook }),
      selectPerson: (peopleSelected) => set({ peopleSelected }),
      setPeopleSearchDraft: (peopleSearchDraft) => set({ peopleSearchDraft }),
      commitPeopleSearch: (peopleSearch) => set({ peopleSearch }),
      clearPeopleSearch: () => set({ peopleSearch: "", peopleSearchDraft: "" }),
      setPeopleFocus: (peopleFocus) => set({ peopleFocus }),
      setSource: (source) => set({ source, search: "", searchDraft: "", selected: [], anchor: null }),
      setSearchDraft: (searchDraft) => set({ searchDraft }),
      commitSearch: (search) => set({ search, selected: [], anchor: null }),
      clearSearch: () => set({ search: "", searchDraft: "", selected: [], anchor: null }),
      setSearchScope: (searchScope) => set({ searchScope, selected: [], anchor: null }),
      setListFilter: (listFilter) => set({ listFilter, selected: [], anchor: null }),
      setSort: (sort) => set({ sort, ascending: sort === "from" || sort === "to" || sort === "subject" }),
      setAscending: (ascending) => set({ ascending }),
      setConversations: (conversations) => set({ conversations }),
      setContactPhotos: (contactPhotos) => set({ contactPhotos }),
      select: (selected, anchor) => set((s) => ({ selected, anchor: anchor === undefined ? s.anchor : anchor })),
      setFocus: (focus) => set({ focus }),
      toggleSidebar: () => set((s) => ({ sidebarVisible: !s.sidebarVisible })),
      setWidths: (w) => set(w),
    }),
    {
      name: "frostmail.ui",
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({
        sidebarVisible: s.sidebarVisible,
        sidebarWidth: s.sidebarWidth,
        listWidth: s.listWidth,
        conversations: s.conversations,
        sort: s.sort,
        ascending: s.ascending,
        contactPhotos: s.contactPhotos,
        todoBar: s.todoBar,
      }),
    },
  ),
);

/** sourceKey is the sidebar key of a source (lib/mailboxTree.ts). */
export function sourceKey(s: Source): string {
  switch (s.kind) {
    case "smart":
      return `smart:${s.id}`;
    case "allInboxes":
      return "all-inboxes";
    case "role":
      return `role:${s.role}`;
    case "flagged":
      return "flagged";
    case "flagColor":
      return `flag:${s.color}`;
    case "vips":
      return "vips";
    case "vip":
      return s.key;
    case "mailbox":
      return `mailbox:${s.mailboxId}`;
  }
}

/** UNIFIED_ROLES are the roles a unified source can show (lib/mailboxTree.ts). */
const UNIFIED_ROLES: readonly MailboxRole[] = ["drafts", "sent", "junk", "trash", "archive"];

/** sourceFromKey parses a sidebar key; null for keys that are not sources. */
export function sourceFromKey(key: string, vips: Vip[] = []): Source | null {
  if (key === "all-inboxes") return { kind: "allInboxes" };
  if (key === "flagged") return { kind: "flagged" };
  if (key === "vips") return { kind: "vips" };
  const color = /^flag:([1-7])$/.exec(key);
  if (color) return { kind: "flagColor", color: Number(color[1]) };
  const group = vipGroups(vips).find((group) => group.key === key);
  if (group) return { kind: "vip", key: group.key, addresses: group.addresses };
  const role = UNIFIED_ROLES.find((r) => key === `role:${r}`);
  if (role) return { kind: "role", role };
  const smart = /^smart:(\d+)$/.exec(key);
  if (smart) return { kind: "smart", id: Number(smart[1]) };
  const m = /^(?:mailbox|favorite):(\d+)$/.exec(key);
  return m ? { kind: "mailbox", mailboxId: Number(m[1]) } : null;
}

/** sourceQuery is the view query of a source. */
export function sourceQuery(s: Source, conversations: boolean): ViewQuery {
  const threads = conversations ? { threads: true } : {};
  switch (s.kind) {
    case "smart":
      return { smartMailboxId: s.id, ...threads };
    case "allInboxes":
      return { role: "inbox", ...threads };
    case "role":
      return { role: s.role, ...threads };
    case "flagged":
      return { flagged: true, ...threads };
    case "flagColor":
      return {
        conditions: { match: "all", conditions: [{ field: "color", op: "is", value: String(s.color) }] },
        ...threads,
      };
    case "vips":
      return { conditions: { match: "all", conditions: [{ field: "vip", op: "is", value: "true" }] }, ...threads };
    case "vip":
      return {
        conditions: { match: "any", conditions: s.addresses.map((value) => ({ field: "from", op: "is", value })) },
        ...threads,
      };
    case "mailbox":
      return { mailboxId: s.mailboxId, ...threads };
  }
}

function filterQuery(filter: ListFilter = "all"): ViewQuery {
  switch (filter) {
    case "all":
      return {};
    case "unread":
      return { unread: true };
    case "flagged":
      return { flagged: true };
    case "attachments":
      return { hasAttachments: true };
    case "toMe":
    case "ccMe":
    case "vips": {
      const field = filter === "toMe" ? "tome" : filter === "ccMe" ? "ccme" : "vip";
      return { filter: { match: "all", conditions: [{ field, op: "is", value: "true" }] } };
    }
  }
}

/** listQuery is the view query the list shows for the UI state. */
export function listQuery(
  ui: Pick<UIState, "source" | "search" | "searchScope" | "conversations"> &
    Partial<Pick<UIState, "listFilter" | "sort" | "ascending">>,
): ViewQuery {
  const sort = ui.sort ?? "date";
  const order: ViewQuery =
    sort === "date" && !ui.ascending ? {} : { sort, ...(ui.ascending ? { ascending: true } : {}) };
  const filter = { ...filterQuery(ui.listFilter), ...order };
  const base = { ...sourceQuery(ui.source, ui.conversations), ...filter };
  if (ui.search === "") return base;
  if (ui.searchScope === "all")
    return ui.conversations ? { text: ui.search, threads: true, ...filter } : { text: ui.search, ...filter };
  return { ...base, text: ui.search };
}
