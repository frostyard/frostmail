// The toolbar is the window's title bar: one 52px strip split into three
// segments aligned with the panes below it, carrying Mail.app's actions,
// the search field and our own window controls (docs/specs/ui.md, Layout:
// Toolbar). The strip and its empty space drag the window; double-clicking
// empty space toggles maximize. A narrow window's toolbar holds its one
// pane's controls instead, with the message actions in a bar at the foot
// (docs/specs/ui.md, Narrow windows). The planner's containers decide what
// each command does.
import {
  Archive,
  ChevronLeft,
  ChevronRight,
  Copy,
  Flag,
  FolderInput,
  Forward,
  type LucideIcon,
  Mail,
  MailOpen,
  Minus,
  PanelLeft,
  PanelRight,
  Plus,
  RefreshCw,
  Reply,
  ReplyAll,
  Settings,
  Square,
  SquarePen,
  Trash2,
  UserPlus,
  X,
} from "lucide-react";
import { type MouseEvent, type ReactNode, useState } from "react";

import type { CalendarPane, NarrowPane, PeoplePane, TasksPane } from "../../app/narrow";
import type { CalendarView } from "../../lib/calendarDates";
import { FLAG_NAMES, flagLabel } from "../../lib/flags";
import { ContextMenu, type MenuItem } from "../menu/ContextMenu";

/** ToolbarCommand is what a toolbar control asks for. */
export type ToolbarCommand =
  | { kind: "newContact" }
  | { kind: "newEvent" }
  | { kind: "newTask" }
  | { kind: "toggleCompleted" }
  | { kind: "toggleTodoBar" }
  | { kind: "today" }
  | { kind: "previousPeriod" }
  | { kind: "nextPeriod" }
  | { kind: "calendarView"; view: CalendarView }
  | { kind: "toggleSidebar" }
  | { kind: "getMail" }
  | { kind: "delete" }
  | { kind: "archive" }
  /** color 0 clears the flag. */
  | { kind: "flag"; color: number }
  | { kind: "toggleRead" }
  | { kind: "move"; mailboxId: number }
  | { kind: "compose" }
  | { kind: "reply" }
  | { kind: "replyAll" }
  | { kind: "forward" }
  | { kind: "minimize" }
  | { kind: "toggleMaximize" }
  | { kind: "close" }
  | { kind: "settings" };

/** MoveTarget is one entry of the Move menu. */
export interface MoveTarget {
  mailboxId: number;
  label: string;
  depth: number;
}

/** ToolbarSelection describes the selected messages. */
export interface ToolbarSelection {
  count: number;
  /** Every selected message is seen. */
  seen: boolean;
  /** The first selected message's flag color; 0 when not flagged. */
  flagColor: number;
}

/** ToolbarCalendar describes the calendar view and event pane. */
export interface ToolbarCalendar {
  view: CalendarView;
  title: string;
  canCreate?: boolean;
  paneWidth: number;
}

/** ToolbarTasks describes the task source and detail pane. */
export interface ToolbarTasks {
  title: string;
  canCreate?: boolean;
  showCompleted: boolean | null;
  paneWidth: number;
}

/**
 * ToolbarNarrow is a narrow window's toolbar: the pane it is over, and
 * Back. Mail's list goes back to Mailboxes, and a message to its list
 * (named by the toolbar's title); a module's pane names what its Back
 * returns to in back. A module's top pane has no Back: the module tabs at
 * the window's foot move between modules.
 */
export interface ToolbarNarrow {
  pane: NarrowPane | CalendarPane | PeoplePane | TasksPane | "module";
  /** What a module pane's Back returns to; none for a top pane. */
  back?: string;
  /** The title, where the toolbar's own does not fit the pane: a calendar day's. */
  title?: string;
  onBack: () => void;
}

/** ToolbarProps are the toolbar's inputs. */
export interface ToolbarProps {
  /** Set in a narrow window. */
  narrow?: ToolbarNarrow;
  /** Minimize, Maximize and Close; only the desktop app's windows have them (default true). */
  windowControls?: boolean;
  flagNames?: readonly string[];
  mode?: "mail" | "people" | "calendar" | "tasks";
  calendar?: ToolbarCalendar;
  tasks?: ToolbarTasks;
  people?: { canCreate: boolean };
  todoBar?: boolean;
  /** The sidebar's width, 0 while it is hidden. */
  sidebarWidth: number;
  listWidth: number;
  title: string;
  subtitle: string;
  syncing: boolean;
  selection: ToolbarSelection;
  canArchive: boolean;
  moveTargets: MoveTarget[];
  maximized: boolean;
  /** The search field, placed before the window controls. */
  search: ReactNode;
  onCommand: (cmd: ToolbarCommand) => void;
}

type MenuKind = "flag" | "move";

interface MenuState {
  kind: MenuKind;
  x: number;
  y: number;
}

const BUTTON_CLASS =
  "flex size-7 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive disabled:opacity-40";
// A narrow window's buttons are touch targets: 44 high, with 20px icons.
const TOUCH_CLASS =
  "flex size-11 shrink-0 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive disabled:opacity-40";
const BAR_CLASS =
  "flex h-11 min-w-0 flex-1 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive disabled:opacity-40";

// The Flag menu: the seven colors as checkable items, then Clear Flag.
function flagItems(selection: ToolbarSelection, flagNames?: readonly string[]): MenuItem[] {
  const items: MenuItem[] = FLAG_NAMES.map((_, i) => ({
    kind: "item",
    id: `flag:${i + 1}`,
    label: flagLabel(i + 1, flagNames),
    checked: selection.flagColor === i + 1,
  }));
  items.push({ kind: "separator" });
  items.push({ kind: "item", id: "flag:0", label: "Clear Flag", disabled: selection.flagColor === 0 });
  return items;
}

// The Move menu: one item per mailbox, indented by its depth.
function moveItems(targets: MoveTarget[]): MenuItem[] {
  return targets.map((target) => ({
    kind: "item",
    id: `move:${target.mailboxId}`,
    label: " ".repeat(target.depth) + target.label,
  }));
}

interface ToolbarButtonProps {
  label: string;
  className?: string;
  shortcut?: string;
  icon: ReactNode;
  disabled?: boolean;
  hasMenu?: boolean;
  pressed?: boolean;
  onClick?: (event: MouseEvent<HTMLButtonElement>) => void;
}

function ToolbarButton(props: ToolbarButtonProps) {
  const { label, shortcut, icon, disabled, hasMenu, onClick, className } = props;
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={props.pressed}
      title={shortcut ? `${label} (${shortcut})` : label}
      aria-haspopup={hasMenu ? "menu" : undefined}
      disabled={disabled}
      onClick={onClick}
      className={className ?? BUTTON_CLASS}
    >
      {icon}
    </button>
  );
}

// The segment over the sidebar: the sidebar toggle and Get Mail. It keeps
// its buttons, and its width collapses, while the sidebar is hidden.
function SidebarSegment({
  mode,
  sidebarWidth,
  syncing,
  onCommand,
}: {
  mode: "mail" | "people" | "calendar" | "tasks";
  sidebarWidth: number;
  syncing: boolean;
  onCommand: (cmd: ToolbarCommand) => void;
}) {
  return (
    <div
      data-segment="sidebar"
      data-tauri-drag-region
      className="flex h-full shrink-0 items-center gap-1 px-2"
      style={sidebarWidth > 0 ? { width: `${sidebarWidth + 1}px` } : undefined}
    >
      <ToolbarButton
        label="Toggle Sidebar"
        shortcut="Ctrl+Alt+S"
        icon={<PanelLeft size={16} />}
        onClick={() => onCommand({ kind: "toggleSidebar" })}
      />
      {mode === "mail" && (
        <ToolbarButton
          label="Get Mail"
          shortcut="Ctrl+Shift+N"
          icon={<RefreshCw size={16} className={syncing ? "animate-spin" : undefined} />}
          onClick={() => onCommand({ kind: "getMail" })}
        />
      )}
    </div>
  );
}

// The segment over the list: the source title above its subtitle, and
// New Message.
function ListSegment({
  mode,
  people,
  listWidth,
  title,
  subtitle,
  onCommand,
}: {
  mode: ToolbarProps["mode"];
  people: ToolbarProps["people"];
  listWidth: number;
  title: string;
  subtitle: string;
  onCommand: (cmd: ToolbarCommand) => void;
}) {
  return (
    <div
      data-segment="list"
      data-tauri-drag-region
      className="flex h-full shrink-0 items-center px-3"
      style={{ width: `${listWidth + 1}px` }}
    >
      <div data-tauri-drag-region className="flex min-w-0 flex-1 flex-col">
        <span className="text-toolbar-title truncate">{title}</span>
        <span className="text-toolbar-subtitle truncate text-secondary">{subtitle}</span>
      </div>
      {mode === "people" && (
        <ToolbarButton
          label="New Contact"
          shortcut="Ctrl+N"
          icon={<UserPlus size={16} />}
          disabled={!people?.canCreate}
          onClick={() => onCommand({ kind: "newContact" })}
        />
      )}
      <ToolbarButton
        label="New Message"
        shortcut={mode === "people" ? undefined : "Ctrl+N"}
        icon={<SquarePen size={16} />}
        onClick={() => onCommand({ kind: "compose" })}
      />
    </div>
  );
}

interface MessageActionsProps {
  /** The narrow window's bar: touch-sized buttons sharing its width. */
  bar?: boolean;
  selection: ToolbarSelection;
  canArchive: boolean;
  moveTargets: MoveTarget[];
  onCommand: (command: ToolbarCommand) => void;
  onOpenMenu: (kind: MenuKind) => (event: MouseEvent<HTMLButtonElement>) => void;
}

interface MessageAction {
  label: string;
  shortcut: string;
  icon: LucideIcon;
  kind: "delete" | "archive" | "reply" | "replyAll" | "forward";
}

const MESSAGE_ACTIONS: MessageAction[] = [
  { label: "Delete", shortcut: "Delete", icon: Trash2, kind: "delete" },
  { label: "Archive", shortcut: "Ctrl+Alt+A", icon: Archive, kind: "archive" },
  { label: "Reply", shortcut: "Ctrl+R", icon: Reply, kind: "reply" },
  { label: "Reply All", shortcut: "Ctrl+Shift+R", icon: ReplyAll, kind: "replyAll" },
  { label: "Forward", shortcut: "Ctrl+Shift+F", icon: Forward, kind: "forward" },
];

function MessageActions({ bar, selection, canArchive, moveTargets, onCommand, onOpenMenu }: MessageActionsProps) {
  const none = selection.count === 0;
  const size = bar ? 20 : 16;
  const className = bar ? BAR_CLASS : undefined;
  return (
    <>
      {MESSAGE_ACTIONS.map(({ label, shortcut, icon: Icon, kind }) => (
        <ToolbarButton
          key={kind}
          label={label}
          shortcut={shortcut}
          className={className}
          icon={<Icon size={size} />}
          disabled={none || (kind === "archive" && !canArchive)}
          onClick={() => onCommand({ kind })}
        />
      ))}
      <ToolbarButton
        label="Flag"
        shortcut="Ctrl+Shift+L"
        className={className}
        icon={<Flag size={size} />}
        disabled={none}
        hasMenu
        onClick={onOpenMenu("flag")}
      />
      <ToolbarButton
        label={selection.seen ? "Mark as Unread" : "Mark as Read"}
        shortcut="Ctrl+Shift+U"
        className={className}
        icon={selection.seen ? <Mail size={size} /> : <MailOpen size={size} />}
        disabled={none}
        onClick={() => onCommand({ kind: "toggleRead" })}
      />
      <ToolbarButton
        label="Move"
        className={className}
        icon={<FolderInput size={size} />}
        disabled={none || moveTargets.length === 0}
        hasMenu
        onClick={onOpenMenu("move")}
      />
    </>
  );
}

// The segment over the reader: the message actions, then the search field
// and the window controls at the far right.
function ReaderSegment({
  mode,
  selection,
  canArchive,
  moveTargets,
  maximized,
  search,
  onCommand,
  onOpenMenu,
  paneWidth,
  todoBar,
  windowControls,
}: {
  paneWidth?: number;
  todoBar?: boolean;
  windowControls: boolean;
  mode: "mail" | "people" | "calendar" | "tasks";
  selection: ToolbarSelection;
  canArchive: boolean;
  moveTargets: MoveTarget[];
  maximized: boolean;
  search: ReactNode;
  onCommand: (cmd: ToolbarCommand) => void;
  onOpenMenu: (kind: MenuKind) => (event: MouseEvent<HTMLButtonElement>) => void;
}) {
  const fixedPane = mode === "calendar" || mode === "tasks";
  return (
    <div
      data-segment={fixedPane ? "pane" : "reader"}
      data-tauri-drag-region
      className={`flex h-full min-w-0 items-center gap-1 px-2 ${fixedPane ? "shrink-0" : "flex-1"}`}
      style={fixedPane ? { width: `${(paneWidth ?? 320) + 1}px` } : undefined}
    >
      {mode === "mail" && (
        <MessageActions
          selection={selection}
          canArchive={canArchive}
          moveTargets={moveTargets}
          onCommand={onCommand}
          onOpenMenu={onOpenMenu}
        />
      )}
      <div data-tauri-drag-region className="flex-1" />
      {mode === "mail" && (
        <ToolbarButton
          label="To-Do Bar"
          icon={<PanelRight size={16} />}
          pressed={todoBar ?? false}
          onClick={() => onCommand({ kind: "toggleTodoBar" })}
        />
      )}
      {!fixedPane && search}
      <ToolbarButton
        label="Settings"
        icon={<Settings size={16} />}
        shortcut="Ctrl+,"
        onClick={() => onCommand({ kind: "settings" })}
      />
      {windowControls && (
        <>
          <ToolbarButton label="Minimize" icon={<Minus size={16} />} onClick={() => onCommand({ kind: "minimize" })} />
          <ToolbarButton
            label={maximized ? "Restore" : "Maximize"}
            icon={maximized ? <Copy size={16} /> : <Square size={16} />}
            onClick={() => onCommand({ kind: "toggleMaximize" })}
          />
          <ToolbarButton label="Close" icon={<X size={16} />} onClick={() => onCommand({ kind: "close" })} />
        </>
      )}
    </div>
  );
}

function CalendarSegment({
  calendar,
  onCommand,
}: {
  calendar: ToolbarCalendar;
  onCommand: (cmd: ToolbarCommand) => void;
}) {
  const labels: Record<CalendarView, string> = { day: "Day", week: "Week", month: "Month" };
  const views: CalendarView[] = ["day", "week", "month"];
  return (
    <div data-segment="calendar" data-tauri-drag-region className="flex h-full min-w-0 flex-1 items-center gap-1 px-3">
      <ToolbarButton
        label="New Event"
        shortcut="Ctrl+N"
        icon={<Plus size={16} />}
        disabled={calendar.canCreate === false}
        onClick={() => onCommand({ kind: "newEvent" })}
      />
      <button
        type="button"
        className="h-7 rounded-md px-[10px] text-[13px] hover:bg-selection-inactive"
        onClick={() => onCommand({ kind: "today" })}
      >
        Today
      </button>
      <ToolbarButton
        label={`Previous ${labels[calendar.view]}`}
        icon={<ChevronLeft size={16} />}
        onClick={() => onCommand({ kind: "previousPeriod" })}
      />
      <ToolbarButton
        label={`Next ${labels[calendar.view]}`}
        icon={<ChevronRight size={16} />}
        onClick={() => onCommand({ kind: "nextPeriod" })}
      />
      <span data-tauri-drag-region className="min-w-0 flex-1 truncate px-2 text-[15px] leading-5 font-semibold">
        {calendar.title}
      </span>
      <div role="radiogroup" aria-label="View" className="flex h-7 shrink-0 items-center rounded-md bg-badge p-0.5">
        {views.map((view) => (
          // biome-ignore lint/a11y/useSemanticElements: a segmented control: buttons with radio semantics, named by their text.
          <button
            key={view}
            type="button"
            role="radio"
            aria-checked={calendar.view === view}
            title={`${labels[view]} (Ctrl+Alt+${views.indexOf(view) + 1})`}
            className={`h-6 rounded px-[10px] text-[13px] ${calendar.view === view ? "bg-window shadow-sm" : "text-secondary"}`}
            onClick={() => onCommand({ kind: "calendarView", view })}
          >
            {labels[view]}
          </button>
        ))}
      </div>
    </div>
  );
}

function TasksSegment({ tasks, onCommand }: { tasks: ToolbarTasks; onCommand: ToolbarProps["onCommand"] }) {
  return (
    <div data-segment="tasks" data-tauri-drag-region className="flex h-full min-w-0 flex-1 items-center gap-1 px-3">
      <ToolbarButton
        label="New Task"
        shortcut="Ctrl+N"
        icon={<Plus size={16} />}
        disabled={tasks.canCreate === false}
        onClick={() => onCommand({ kind: "newTask" })}
      />
      <span data-tauri-drag-region className="min-w-0 flex-1 truncate px-2 text-[15px] leading-5 font-semibold">
        {tasks.title}
      </span>
      {tasks.showCompleted !== null && (
        <button
          type="button"
          aria-pressed={tasks.showCompleted}
          className="h-7 shrink-0 rounded-md px-[10px] text-[13px] hover:bg-selection-inactive"
          onClick={() => onCommand({ kind: "toggleCompleted" })}
        >
          Show Completed
        </button>
      )}
    </div>
  );
}

function ToolbarSegments({
  props,
  onOpenMenu,
}: {
  props: ToolbarProps;
  onOpenMenu: MessageActionsProps["onOpenMenu"];
}) {
  return (
    <>
      <SidebarSegment
        mode={props.mode ?? "mail"}
        sidebarWidth={props.sidebarWidth}
        syncing={props.syncing}
        onCommand={props.onCommand}
      />
      {props.mode === "calendar" ? (
        <CalendarSegment
          calendar={props.calendar ?? { view: "week", title: props.title, paneWidth: 320 }}
          onCommand={props.onCommand}
        />
      ) : props.mode === "tasks" ? (
        <TasksSegment
          tasks={props.tasks ?? { title: props.title, showCompleted: null, paneWidth: 320 }}
          onCommand={props.onCommand}
        />
      ) : (
        <ListSegment
          mode={props.mode}
          people={props.people}
          listWidth={props.listWidth}
          title={props.title}
          subtitle={props.subtitle}
          onCommand={props.onCommand}
        />
      )}
      <ReaderSegment
        paneWidth={props.mode === "tasks" ? props.tasks?.paneWidth : props.calendar?.paneWidth}
        todoBar={props.todoBar}
        mode={props.mode ?? "mail"}
        selection={props.selection}
        canArchive={props.canArchive}
        moveTargets={props.moveTargets}
        maximized={props.maximized}
        search={props.search}
        onCommand={props.onCommand}
        onOpenMenu={onOpenMenu}
        windowControls={props.windowControls ?? true}
      />
    </>
  );
}

function BackButton({ label, onBack }: { label: string; onBack: () => void }) {
  return (
    <button
      type="button"
      aria-label={`Back to ${label}`}
      onClick={onBack}
      className="flex h-11 min-w-0 max-w-[45%] shrink-0 items-center rounded-md pr-2 text-[15px] leading-5 text-accent hover:bg-selection-inactive"
    >
      <ChevronLeft size={24} className="shrink-0" />
      <span className="truncate">{label}</span>
    </button>
  );
}

// A narrow window's toolbar: Back and the pane's own controls, and in the
// list, the search field on a row of its own under it.
function NarrowToolbar({ props, narrow }: { props: ToolbarProps; narrow: ToolbarNarrow }) {
  const { pane } = narrow;
  const back = pane === "list" ? "Mailboxes" : pane === "reader" ? props.title : narrow.back;
  const titles: Partial<Record<ToolbarNarrow["pane"], string>> = {
    mailboxes: "Mailboxes",
    list: props.title,
    module: props.calendar?.title ?? props.tasks?.title ?? props.title,
    calendars: "Calendars",
    taskLists: "Lists",
    tasks: props.tasks?.title,
    books: "Address Books",
    people: props.title,
  };
  const title = narrow.title ?? titles[pane] ?? "";
  const subtitle = pane === "list" || pane === "people" ? props.subtitle : "";
  const command = (cmd: ToolbarCommand) => () => props.onCommand(cmd);
  return (
    <>
      <div
        role="toolbar"
        aria-label="Toolbar"
        className="flex h-[52px] shrink-0 items-center gap-1 border-b border-separator bg-toolbar px-1"
      >
        {back !== undefined && <BackButton label={back} onBack={narrow.onBack} />}
        <div className={`flex min-w-0 flex-1 flex-col ${back !== undefined ? "pl-1" : "pl-3"}`}>
          <span className="text-toolbar-title truncate">{title}</span>
          {subtitle && <span className="text-toolbar-subtitle truncate text-secondary">{subtitle}</span>}
        </div>
        {pane === "mailboxes" && (
          <>
            <ToolbarButton
              label="Get Mail"
              className={TOUCH_CLASS}
              icon={<RefreshCw size={20} className={props.syncing ? "animate-spin" : undefined} />}
              onClick={command({ kind: "getMail" })}
            />
            <ToolbarButton
              label="Settings"
              className={TOUCH_CLASS}
              icon={<Settings size={20} />}
              onClick={command({ kind: "settings" })}
            />
          </>
        )}
        {pane === "list" && (
          <ToolbarButton
            label="New Message"
            className={TOUCH_CLASS}
            icon={<SquarePen size={20} />}
            onClick={command({ kind: "compose" })}
          />
        )}
        {pane === "day" && (
          <>
            <button
              type="button"
              className="h-11 shrink-0 rounded-md px-3 text-[15px] leading-5 text-accent hover:bg-selection-inactive"
              onClick={command({ kind: "today" })}
            >
              Today
            </button>
            <ToolbarButton
              label="New Event"
              className={TOUCH_CLASS}
              icon={<Plus size={20} />}
              disabled={props.calendar?.canCreate === false}
              onClick={command({ kind: "newEvent" })}
            />
          </>
        )}
        {pane === "people" && (
          <ToolbarButton
            label="New Contact"
            className={TOUCH_CLASS}
            icon={<UserPlus size={20} />}
            disabled={!props.people?.canCreate}
            onClick={command({ kind: "newContact" })}
          />
        )}
        {pane === "tasks" && (
          <ToolbarButton
            label="New Task"
            className={TOUCH_CLASS}
            icon={<Plus size={20} />}
            disabled={props.tasks?.canCreate === false}
            onClick={command({ kind: "newTask" })}
          />
        )}
      </div>
      {(pane === "list" || pane === "people") && (
        <div className="flex h-11 shrink-0 items-center border-b border-separator bg-toolbar px-3">{props.search}</div>
      )}
    </>
  );
}

/** MessageBarProps are the narrow window's message bar's inputs. */
export type MessageBarProps = Pick<
  ToolbarProps,
  "selection" | "canArchive" | "moveTargets" | "flagNames" | "onCommand"
>;

/** MessageBar is a narrow window's message actions, in a bar at the foot of Mail. */
export function MessageBar(props: MessageBarProps) {
  const [menu, setMenu] = useState<MenuState | null>(null);
  const openMenu = (kind: MenuKind) => (event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    setMenu({ kind, x: rect.left, y: rect.top });
  };
  const choose = (id: string) => {
    if (id.startsWith("flag:")) props.onCommand({ kind: "flag", color: Number(id.slice(5)) });
    else if (id.startsWith("move:")) props.onCommand({ kind: "move", mailboxId: Number(id.slice(5)) });
  };
  return (
    <div
      role="toolbar"
      aria-label="Message Actions"
      className="flex shrink-0 items-center gap-1 border-t border-separator bg-toolbar px-1 pt-1 pb-[max(4px,env(safe-area-inset-bottom))]"
    >
      <MessageActions
        bar
        selection={props.selection}
        canArchive={props.canArchive}
        moveTargets={props.moveTargets}
        onCommand={props.onCommand}
        onOpenMenu={openMenu}
      />
      {menu && (
        <ContextMenu
          items={menu.kind === "flag" ? flagItems(props.selection, props.flagNames) : moveItems(props.moveTargets)}
          x={menu.x}
          y={menu.y}
          onSelect={choose}
          onClose={() => setMenu(null)}
        />
      )}
    </div>
  );
}

/** Toolbar is the window's top strip, aligned with the panes. */
export function Toolbar(props: ToolbarProps) {
  const [menu, setMenu] = useState<MenuState | null>(null);
  if (props.narrow) return <NarrowToolbar props={props} narrow={props.narrow} />;

  const openMenu = (kind: MenuKind) => (event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    setMenu({ kind, x: rect.left, y: rect.bottom });
  };

  const choose = (id: string) => {
    if (id.startsWith("flag:")) props.onCommand({ kind: "flag", color: Number(id.slice(5)) });
    else if (id.startsWith("move:")) props.onCommand({ kind: "move", mailboxId: Number(id.slice(5)) });
  };

  return (
    <div
      role="toolbar"
      aria-label="Toolbar"
      data-tauri-drag-region
      className="flex h-[52px] shrink-0 items-center border-b border-separator bg-toolbar"
    >
      <ToolbarSegments props={props} onOpenMenu={openMenu} />
      {menu && (props.mode ?? "mail") === "mail" ? (
        <ContextMenu
          items={menu.kind === "flag" ? flagItems(props.selection, props.flagNames) : moveItems(props.moveTargets)}
          x={menu.x}
          y={menu.y}
          onSelect={choose}
          onClose={() => setMenu(null)}
        />
      ) : null}
    </div>
  );
}
