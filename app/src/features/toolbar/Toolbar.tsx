// The toolbar is the window's title bar: one 52px strip split into three
// segments aligned with the panes below it, carrying Mail.app's actions,
// the search field and our own window controls (docs/specs/ui.md, Layout:
// Toolbar). The strip and its empty space drag the window; double-clicking
// empty space toggles maximize. A narrow window's toolbar holds its one
// pane's controls instead, with the message actions in a bar at the foot
// (docs/specs/ui.md, Narrow windows). The planner's containers decide what
// each command does. Message actions that do not fit move into a "»" menu
// from their end, as Mail.app does.

import { Menu as ContextMenu, IconButton, type MenuEntry as MenuItem, WindowControls } from "@frostyard/lodge-ui/react";
import {
  Archive,
  ChevronLeft,
  ChevronRight,
  ChevronsRight,
  Flag,
  FolderInput,
  Forward,
  type LucideIcon,
  Mail,
  MailOpen,
  PanelLeft,
  PanelRight,
  Plus,
  RefreshCw,
  Reply,
  ReplyAll,
  Settings,
  SquarePen,
  Trash2,
  UserPlus,
} from "lucide-react";
import { type MouseEvent, type ReactNode, type RefObject, useLayoutEffect, useRef, useState } from "react";
import type { CalendarPane, NarrowPane, PeoplePane, TasksPane } from "../../app/narrow";
import type { CalendarView } from "../../lib/calendarDates";
import { FLAG_NAMES, flagLabel } from "../../lib/flags";
import { fitItems } from "./fit";

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
  "flex h-11 min-w-11 flex-1 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive disabled:opacity-40";
// A narrow window's message buttons are 44 wide at least, so they stay touch
// targets even when the bar cannot share out its width.
const BAR_WIDTH = 44;

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
  /** The menu this button opens is open; only with hasMenu. */
  expanded?: boolean;
  pressed?: boolean;
  tabIndex?: number;
  onClick?: (event: MouseEvent<HTMLButtonElement>) => void;
}

/**
 * ToolbarButton is the toolbar's icon button where lodge-ui's IconButton
 * does not fit: a menu button (aria-haspopup), a class of its own (a narrow
 * window's 44px touch bar and its 20px icons), or an icon node that
 * animates. The plain ones are lodge-ui's IconButton.
 */
function ToolbarButton(props: ToolbarButtonProps) {
  const { label, shortcut, icon, disabled, hasMenu, onClick, className } = props;
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={props.pressed}
      title={shortcut ? `${label} (${shortcut})` : label}
      aria-haspopup={hasMenu ? "menu" : undefined}
      aria-expanded={hasMenu ? props.expanded : undefined}
      tabIndex={props.tabIndex}
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
      <IconButton
        label="Toggle Sidebar"
        keys="Ctrl+Alt+S"
        icon={PanelLeft}
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
        <IconButton
          label="New Contact"
          keys="Ctrl+N"
          icon={UserPlus}
          disabled={!people?.canCreate}
          onClick={() => onCommand({ kind: "newContact" })}
        />
      )}
      <IconButton
        label="New Message"
        {...(mode === "people" ? {} : { keys: "Ctrl+N" })}
        icon={SquarePen}
        onClick={() => onCommand({ kind: "compose" })}
      />
    </div>
  );
}

type MessageActionKind = "delete" | "archive" | "reply" | "replyAll" | "forward" | "flag" | "toggleRead" | "move";
type PlainKind = "delete" | "archive" | "reply" | "replyAll" | "forward" | "toggleRead";
type ImmediateKind = Exclude<PlainKind, "toggleRead">;

// MESSAGE_ACTION_ORDER is the message actions in toolbar order. Actions that
// do not fit go into the » menu from the end, and the menu lists them in
// this same order.
const MESSAGE_ACTION_ORDER: readonly MessageActionKind[] = [
  "delete",
  "archive",
  "reply",
  "replyAll",
  "forward",
  "flag",
  "toggleRead",
  "move",
];

interface MessageAction {
  label: string;
  keys: string;
  icon: LucideIcon;
}

const MESSAGE_ACTIONS: Record<ImmediateKind, MessageAction> = {
  delete: { label: "Delete", keys: "Delete", icon: Trash2 },
  archive: { label: "Archive", keys: "Ctrl+Alt+A", icon: Archive },
  reply: { label: "Reply", keys: "Ctrl+R", icon: Reply },
  replyAll: { label: "Reply All", keys: "Ctrl+Shift+R", icon: ReplyAll },
  forward: { label: "Forward", keys: "Ctrl+Shift+F", icon: Forward },
};

// ACTION_ICONS draws every action in the hidden measuring row.
const ACTION_ICONS: Record<MessageActionKind, LucideIcon> = {
  delete: Trash2,
  archive: Archive,
  reply: Reply,
  replyAll: ReplyAll,
  forward: Forward,
  flag: Flag,
  toggleRead: MailOpen,
  move: FolderInput,
};

/** messageCommand is the command a plain menu id runs. */
function messageCommand(kind: PlainKind): ToolbarCommand {
  switch (kind) {
    case "delete":
      return { kind: "delete" };
    case "archive":
      return { kind: "archive" };
    case "reply":
      return { kind: "reply" };
    case "replyAll":
      return { kind: "replyAll" };
    case "forward":
      return { kind: "forward" };
    case "toggleRead":
      return { kind: "toggleRead" };
  }
}

/** commandFor reads a menu id, or null when the id names no command. */
function commandFor(id: string): ToolbarCommand | null {
  switch (id) {
    case "delete":
    case "archive":
    case "reply":
    case "replyAll":
    case "forward":
    case "toggleRead":
      return messageCommand(id);
  }
  if (id.startsWith("flag:")) return { kind: "flag", color: Number(id.slice(5)) };
  if (id.startsWith("move:")) return { kind: "move", mailboxId: Number(id.slice(5)) };
  return null;
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

interface MessageActionsWithCountProps extends MessageActionsProps {
  /** How many actions show, from MESSAGE_ACTION_ORDER's start. */
  count?: number;
}

/** actionDisabled applies a button's disabled rule to its menu row too. */
function actionDisabled(
  kind: MessageActionKind,
  selection: ToolbarSelection,
  canArchive: boolean,
  moveTargets: MoveTarget[],
): boolean {
  if (kind === "archive") return selection.count === 0 || !canArchive;
  if (kind === "move") return selection.count === 0 || moveTargets.length === 0;
  return selection.count === 0;
}

function MessageActionButton(props: MessageActionsWithCountProps & { kind: MessageActionKind }) {
  const { kind, bar, selection, canArchive, moveTargets, onCommand, onOpenMenu } = props;
  const size = bar ? 20 : 16;
  const className = bar ? BAR_CLASS : undefined;
  const disabled = actionDisabled(kind, selection, canArchive, moveTargets);
  switch (kind) {
    case "delete":
    case "archive":
    case "reply":
    case "replyAll":
    case "forward": {
      const action = MESSAGE_ACTIONS[kind];
      const Icon = action.icon;
      return bar ? (
        <ToolbarButton
          label={action.label}
          shortcut={action.keys}
          className={className}
          icon={<Icon size={size} />}
          disabled={disabled}
          onClick={() => onCommand(messageCommand(kind))}
        />
      ) : (
        <IconButton
          label={action.label}
          keys={action.keys}
          icon={Icon}
          disabled={disabled}
          onClick={() => onCommand(messageCommand(kind))}
        />
      );
    }
    case "flag":
      return (
        <ToolbarButton
          label="Flag"
          shortcut="Ctrl+Shift+L"
          className={className}
          icon={<Flag size={size} />}
          disabled={disabled}
          hasMenu
          onClick={onOpenMenu("flag")}
        />
      );
    case "toggleRead": {
      const label = selection.seen ? "Mark as Unread" : "Mark as Read";
      const Icon = selection.seen ? Mail : MailOpen;
      return bar ? (
        <ToolbarButton
          label={label}
          shortcut="Ctrl+Shift+U"
          className={className}
          icon={<Icon size={size} />}
          disabled={disabled}
          onClick={() => onCommand({ kind: "toggleRead" })}
        />
      ) : (
        <IconButton
          label={label}
          keys="Ctrl+Shift+U"
          icon={Icon}
          disabled={disabled}
          onClick={() => onCommand({ kind: "toggleRead" })}
        />
      );
    }
    case "move":
      return (
        <ToolbarButton
          label="Move"
          className={className}
          icon={<FolderInput size={size} />}
          disabled={disabled}
          hasMenu
          onClick={onOpenMenu("move")}
        />
      );
  }
}

function MessageActions({
  bar,
  selection,
  canArchive,
  moveTargets,
  onCommand,
  onOpenMenu,
  count = MESSAGE_ACTION_ORDER.length,
}: MessageActionsWithCountProps) {
  return (
    <>
      {MESSAGE_ACTION_ORDER.slice(0, count).map((kind) => (
        <MessageActionButton
          key={kind}
          kind={kind}
          bar={bar}
          selection={selection}
          canArchive={canArchive}
          moveTargets={moveTargets}
          onCommand={onCommand}
          onOpenMenu={onOpenMenu}
        />
      ))}
    </>
  );
}

/** overflowItems lists hidden actions in toolbar order, as » menu entries. */
function overflowItems(
  hidden: readonly MessageActionKind[],
  selection: ToolbarSelection,
  canArchive: boolean,
  moveTargets: MoveTarget[],
  flagNames?: readonly string[],
): MenuItem[] {
  return hidden.map((kind): MenuItem => {
    const disabled = actionDisabled(kind, selection, canArchive, moveTargets);
    switch (kind) {
      case "flag":
        return { kind: "submenu", id: "flag", label: "Flag", disabled, items: flagItems(selection, flagNames) };
      case "move":
        return { kind: "submenu", id: "move", label: "Move", disabled, items: moveItems(moveTargets) };
      case "toggleRead":
        return {
          kind: "item",
          id: "toggleRead",
          label: selection.seen ? "Mark as Unread" : "Mark as Read",
          keys: "Ctrl+Shift+U",
          disabled,
        };
      default: {
        const action = MESSAGE_ACTIONS[kind];
        return { kind: "item", id: kind, label: action.label, keys: action.keys, disabled };
      }
    }
  });
}

// MoreButton is the » that opens the actions that no longer fit. It is the
// last button of the actions, and only shows when at least one is hidden.
function MoreButton({
  bar,
  expanded,
  onClick,
}: {
  bar?: boolean;
  expanded: boolean;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}) {
  return (
    <ToolbarButton
      label="More Toolbar Items"
      className={bar ? BAR_CLASS : undefined}
      icon={<ChevronsRight size={bar ? 20 : 16} />}
      hasMenu
      expanded={expanded}
      onClick={onClick}
    />
  );
}

// MeasuringRow holds one button of every action at its real size, and the »,
// so the toolbar knows what the actions would need. It is invisible and inert.
function MeasuringRow({ row }: { row: RefObject<HTMLDivElement | null> }) {
  return (
    <div aria-hidden="true" inert className="pointer-events-none invisible absolute inset-0 overflow-hidden">
      <div ref={row} className="flex w-max items-center gap-1">
        {MESSAGE_ACTION_ORDER.map((kind) => {
          const Icon = ACTION_ICONS[kind];
          return (
            <button key={kind} type="button" tabIndex={-1} className={BUTTON_CLASS}>
              <Icon size={16} />
            </button>
          );
        })}
        <button type="button" tabIndex={-1} className={BUTTON_CLASS}>
          <ChevronsRight size={16} />
        </button>
      </div>
    </div>
  );
}

interface ActionFit {
  available: number;
  widths: number[];
  more: number;
  gap: number;
}

// useActionFit measures the actions in a hidden row at their real size, and
// the room the actions' container has. An unmeasured fit (available 0, as in
// a test without layout) shows every action.
function useActionFit(measured: boolean) {
  const box = useRef<HTMLDivElement>(null);
  const row = useRef<HTMLDivElement>(null);
  const [fit, setFit] = useState<ActionFit | null>(null);
  useLayoutEffect(() => {
    if (!measured) return;
    const container = box.current;
    const measuring = row.current;
    if (!container || !measuring) return;
    const measure = () => {
      const style = getComputedStyle(container);
      const widths = [...measuring.querySelectorAll("button")].map((button) => button.getBoundingClientRect().width);
      setFit({
        available: Math.max(
          0,
          container.getBoundingClientRect().width -
            (Number.parseFloat(style.paddingLeft) || 0) -
            (Number.parseFloat(style.paddingRight) || 0) -
            (Number.parseFloat(style.borderLeftWidth) || 0) -
            (Number.parseFloat(style.borderRightWidth) || 0),
        ),
        widths: widths.slice(0, MESSAGE_ACTION_ORDER.length),
        more: widths[MESSAGE_ACTION_ORDER.length] ?? 0,
        gap: Number.parseFloat(style.columnGap) || 0,
      });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(container);
    // The row also changes when the font loads, without a container resize.
    observer.observe(measuring);
    return () => observer.disconnect();
  }, [measured]);
  return { box, row, fit };
}

// The segment over the reader: the message actions, then the search field
// and the window controls at the far right.
function ReaderSegment({
  mode,
  selection,
  canArchive,
  moveTargets,
  flagNames,
  maximized,
  search,
  onCommand,
  onOpenMenu,
  onSelect,
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
  flagNames?: readonly string[];
  maximized: boolean;
  search: ReactNode;
  onCommand: (cmd: ToolbarCommand) => void;
  onOpenMenu: (kind: MenuKind) => (event: MouseEvent<HTMLButtonElement>) => void;
  onSelect: (id: string) => void;
}) {
  const fixedPane = mode === "calendar" || mode === "tasks";
  const { box, row, fit } = useActionFit(mode === "mail");
  const [more, setMore] = useState<{ x: number; y: number } | null>(null);
  const count = fit ? fitItems(fit.available, fit.widths, fit.more, fit.gap) : MESSAGE_ACTION_ORDER.length;
  const hidden = MESSAGE_ACTION_ORDER.slice(count);
  // The actions' natural width, so the container never takes its width from
  // the buttons it happens to show: a fixed base keeps the fit stable.
  const basis = fit
    ? fit.widths.reduce((sum, width) => sum + width, 0) + fit.gap * Math.max(0, fit.widths.length - 1)
    : null;
  return (
    <div
      data-segment={fixedPane ? "pane" : "reader"}
      data-tauri-drag-region
      className={`flex h-full min-w-0 items-center gap-1 px-2 ${fixedPane ? "shrink-0" : "flex-1"}`}
      style={fixedPane ? { width: `${(paneWidth ?? 320) + 1}px` } : undefined}
    >
      {mode === "mail" && (
        // The message actions give way last: the search shrinks first. The
        // actions that no longer fit move into » from their end, Move first.
        <div
          ref={box}
          data-message-actions
          data-tauri-drag-region
          className="relative flex min-w-0 grow-0 shrink items-center gap-1 overflow-hidden"
          style={basis === null ? undefined : { flexBasis: `${basis}px` }}
        >
          <MeasuringRow row={row} />
          <MessageActions
            selection={selection}
            canArchive={canArchive}
            moveTargets={moveTargets}
            onCommand={onCommand}
            onOpenMenu={onOpenMenu}
            count={count}
          />
          {hidden.length > 0 && (
            <MoreButton
              expanded={more !== null}
              onClick={(event) => {
                const rect = event.currentTarget.getBoundingClientRect();
                setMore({ x: rect.left, y: rect.bottom });
              }}
            />
          )}
          {more && (
            <ContextMenu
              items={overflowItems(hidden, selection, canArchive, moveTargets, flagNames)}
              x={more.x}
              y={more.y}
              onSelect={onSelect}
              onClose={() => setMore(null)}
            />
          )}
        </div>
      )}
      <div data-tauri-drag-region className="flex-1" />
      {mode === "mail" && (
        <IconButton
          label="To-Do Bar"
          icon={PanelRight}
          pressed={todoBar ?? false}
          onClick={() => onCommand({ kind: "toggleTodoBar" })}
        />
      )}
      {!fixedPane && (
        // The search shrinks before the message actions (1000 to 1), from
        // 220 to 120. Below that, the room beside the To-Do Bar button,
        // Settings and the window controls, with the gaps (168), is all it
        // can give: no width pushes the window controls out of the window.
        <div data-search-slot className="w-[220px] min-w-[min(120px,calc(100%_-_168px))] shrink-[1000]">
          {search}
        </div>
      )}
      <div data-window-controls data-tauri-drag-region className="flex shrink-0 items-center gap-1">
        <IconButton label="Settings" icon={Settings} keys="Ctrl+," onClick={() => onCommand({ kind: "settings" })} />
        {windowControls && <WindowControls maximized={maximized} onWindow={onCommand} />}
      </div>
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
      <IconButton
        label="New Event"
        keys="Ctrl+N"
        icon={Plus}
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
      <IconButton
        label={`Previous ${labels[calendar.view]}`}
        icon={ChevronLeft}
        onClick={() => onCommand({ kind: "previousPeriod" })}
      />
      <IconButton
        label={`Next ${labels[calendar.view]}`}
        icon={ChevronRight}
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
      <IconButton
        label="New Task"
        keys="Ctrl+N"
        icon={Plus}
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
  onSelect,
}: {
  props: ToolbarProps;
  onOpenMenu: MessageActionsProps["onOpenMenu"];
  onSelect: (id: string) => void;
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
        flagNames={props.flagNames}
        maximized={props.maximized}
        search={props.search}
        onCommand={props.onCommand}
        onOpenMenu={onOpenMenu}
        onSelect={onSelect}
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

// BAR_WIDTHS are the bar's buttons at their minimum: each is a touch target.
const BAR_WIDTHS: readonly number[] = MESSAGE_ACTION_ORDER.map(() => BAR_WIDTH);

/** MessageBar is a narrow window's message actions, in a bar at the foot of Mail. */
export function MessageBar(props: MessageBarProps) {
  const bar = useRef<HTMLDivElement>(null);
  const [fit, setFit] = useState<{ available: number; gap: number } | null>(null);
  const [menu, setMenu] = useState<{ kind: MenuKind | "more"; x: number; y: number } | null>(null);
  // The bar's buttons are all 44 wide, so only its own width has to be
  // measured. available <= 0 (no layout yet) shows every action.
  useLayoutEffect(() => {
    const element = bar.current;
    if (!element) return;
    const measure = () => {
      const style = getComputedStyle(element);
      setFit({
        available: Math.max(
          0,
          element.getBoundingClientRect().width -
            (Number.parseFloat(style.paddingLeft) || 0) -
            (Number.parseFloat(style.paddingRight) || 0) -
            (Number.parseFloat(style.borderLeftWidth) || 0) -
            (Number.parseFloat(style.borderRightWidth) || 0),
        ),
        gap: Number.parseFloat(style.columnGap) || 0,
      });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  const count = fit ? fitItems(fit.available, BAR_WIDTHS, BAR_WIDTH, fit.gap) : MESSAGE_ACTION_ORDER.length;
  const hidden = MESSAGE_ACTION_ORDER.slice(count);
  const openMenu = (kind: MenuKind | "more") => (event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    setMenu({ kind, x: rect.left, y: rect.top });
  };
  const choose = (id: string) => {
    const command = commandFor(id);
    if (command) props.onCommand(command);
  };
  const items = menu
    ? menu.kind === "flag"
      ? flagItems(props.selection, props.flagNames)
      : menu.kind === "move"
        ? moveItems(props.moveTargets)
        : overflowItems(hidden, props.selection, props.canArchive, props.moveTargets, props.flagNames)
    : [];
  return (
    <div
      ref={bar}
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
        count={count}
      />
      {hidden.length > 0 && <MoreButton bar expanded={menu?.kind === "more"} onClick={openMenu("more")} />}
      {menu && <ContextMenu items={items} x={menu.x} y={menu.y} onSelect={choose} onClose={() => setMenu(null)} />}
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
    const command = commandFor(id);
    if (command) props.onCommand(command);
  };

  return (
    <div
      role="toolbar"
      aria-label="Toolbar"
      data-tauri-drag-region
      className="flex h-[52px] shrink-0 items-center border-b border-separator bg-toolbar"
    >
      <ToolbarSegments props={props} onOpenMenu={openMenu} onSelect={choose} />
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
