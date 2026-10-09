// The app's stores (docs/design/app.md, Data): what maild reports about
// accounts, mailboxes and sync, and the window's UI state.
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import type { OccurrenceKey } from "../features/calendar/TimeGrid";
import type { CalendarView } from "../lib/calendarDates";
import type { Module } from "../lib/modules";
import type { Account, Mailbox, MailboxRole, OutboxItem, SyncStatus, ViewQuery } from "../rpc/gen/api";

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
}

/** useMail is the store of maild data. */
export const useMail = create<MailState>(() => ({
  connection: { state: "connecting" },
  accounts: [],
  mailboxes: [],
  sync: {},
  outbox: [],
}));

/** Source is what the sidebar selected. */
export type Source =
  | { kind: "mailbox"; mailboxId: number }
  | { kind: "allInboxes" }
  | { kind: "role"; role: MailboxRole }
  | { kind: "flagged" };

/** Pane is a focusable area of the window. */
export type Pane = "sidebar" | "list" | "reader";

/** UIState is the window's own state. */
export interface UIState {
  module: Module;
  calendarView: CalendarView;
  calendarDate: string;
  calendarSelected: OccurrenceKey | null;
  calendarFocus: Pane;
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
  /** Selected message IDs; the first is the one the reader shows. */
  selected: number[];
  /** The row a Shift-click or Shift-arrow extends from. */
  anchor: number | null;
  focus: Pane;
  sidebarVisible: boolean;
  sidebarWidth: number;
  listWidth: number;
  conversations: boolean;
}

/** UIActions change UIState. */
export interface UIActions {
  setCalendarView: (view: CalendarView) => void;
  setCalendarDate: (date: string) => void;
  selectOccurrence: (key: OccurrenceKey | null, date?: string) => void;
  setCalendarFocus: (pane: Pane) => void;
  setModule: (module: Module) => void;
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
  select: (ids: number[], anchor?: number | null) => void;
  setFocus: (p: Pane) => void;
  toggleSidebar: () => void;
  setWidths: (w: { sidebarWidth?: number; listWidth?: number }) => void;
}

const initialUI: UIState = {
  module: "mail",
  calendarView: "week",
  calendarDate: "",
  calendarSelected: null,
  calendarFocus: "list",
  peopleBook: "all",
  peopleSelected: null,
  peopleSearch: "",
  peopleSearchDraft: "",
  peopleFocus: "list",
  source: { kind: "allInboxes" },
  search: "",
  searchDraft: "",
  searchScope: "all",
  selected: [],
  anchor: null,
  focus: "list",
  sidebarVisible: true,
  sidebarWidth: 220,
  listWidth: 360,
  conversations: true,
};

/** useUI is the window's UI store; layout preferences persist. */
export const useUI = create<UIState & UIActions>()(
  persist(
    (set) => ({
      ...initialUI,
      setCalendarView: (calendarView) => set({ calendarView }),
      setCalendarDate: (calendarDate) => set({ calendarDate }),
      selectOccurrence: (calendarSelected, date) =>
        set({ calendarSelected, ...(date === undefined ? {} : { calendarDate: date }) }),
      setCalendarFocus: (calendarFocus) => set({ calendarFocus }),
      setModule: (module) => set({ module }),
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
      }),
    },
  ),
);

/** sourceKey is the sidebar key of a source (lib/mailboxTree.ts). */
export function sourceKey(s: Source): string {
  switch (s.kind) {
    case "allInboxes":
      return "all-inboxes";
    case "role":
      return `role:${s.role}`;
    case "flagged":
      return "flagged";
    case "mailbox":
      return `mailbox:${s.mailboxId}`;
  }
}

/** UNIFIED_ROLES are the roles a unified source can show (lib/mailboxTree.ts). */
const UNIFIED_ROLES: readonly MailboxRole[] = ["drafts", "sent", "junk", "trash", "archive"];

/** sourceFromKey parses a sidebar key; null for keys that are not sources. */
export function sourceFromKey(key: string): Source | null {
  if (key === "all-inboxes") return { kind: "allInboxes" };
  if (key === "flagged") return { kind: "flagged" };
  const role = UNIFIED_ROLES.find((r) => key === `role:${r}`);
  if (role) return { kind: "role", role };
  const m = /^mailbox:(\d+)$/.exec(key);
  return m ? { kind: "mailbox", mailboxId: Number(m[1]) } : null;
}

/** sourceQuery is the view query of a source. */
export function sourceQuery(s: Source, conversations: boolean): ViewQuery {
  const threads = conversations ? { threads: true } : {};
  switch (s.kind) {
    case "allInboxes":
      return { role: "inbox", ...threads };
    case "role":
      return { role: s.role, ...threads };
    case "flagged":
      return { flagged: true, ...threads };
    case "mailbox":
      return { mailboxId: s.mailboxId, ...threads };
  }
}

/** listQuery is the view query the list shows for the UI state. */
export function listQuery(ui: Pick<UIState, "source" | "search" | "searchScope" | "conversations">): ViewQuery {
  const base = sourceQuery(ui.source, ui.conversations);
  if (ui.search === "") return base;
  if (ui.searchScope === "all") return ui.conversations ? { text: ui.search, threads: true } : { text: ui.search };
  return { ...base, text: ui.search };
}
