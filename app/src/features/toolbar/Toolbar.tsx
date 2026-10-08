// The toolbar is the window's title bar: one 52px strip split into three
// segments aligned with the panes below it, carrying Mail.app's actions,
// the search field and our own window controls (docs/specs/ui.md, Layout:
// Toolbar). The strip and its empty space drag the window; double-clicking
// empty space toggles maximize. The planner's containers decide what each
// command does.
import {
  Archive,
  Copy,
  Flag,
  FolderInput,
  Forward,
  Mail,
  MailOpen,
  Minus,
  PanelLeft,
  RefreshCw,
  Reply,
  ReplyAll,
  Square,
  SquarePen,
  Trash2,
  X,
} from "lucide-react";
import { type MouseEvent, type ReactNode, useState } from "react";

import { FLAG_NAMES } from "../../lib/flags";
import { ContextMenu, type MenuItem } from "../menu/ContextMenu";

/** ToolbarCommand is what a toolbar control asks for. */
export type ToolbarCommand =
  | { kind: "toggleSidebar" }
  | { kind: "getMail" }
  | { kind: "delete" }
  | { kind: "archive" }
  /** color 0 clears the flag. */
  | { kind: "flag"; color: number }
  | { kind: "toggleRead" }
  | { kind: "move"; mailboxId: number }
  | { kind: "minimize" }
  | { kind: "toggleMaximize" }
  | { kind: "close" };

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

/** ToolbarProps are the toolbar's inputs. */
export interface ToolbarProps {
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

// The Flag menu: the seven colors as checkable items, then Clear Flag.
function flagItems(selection: ToolbarSelection): MenuItem[] {
  const items: MenuItem[] = FLAG_NAMES.map((name, i) => ({
    kind: "item",
    id: `flag:${i + 1}`,
    label: name,
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
  shortcut?: string;
  icon: ReactNode;
  disabled?: boolean;
  hasMenu?: boolean;
  onClick?: (event: MouseEvent<HTMLButtonElement>) => void;
}

function ToolbarButton(props: ToolbarButtonProps) {
  const { label, shortcut, icon, disabled, hasMenu, onClick } = props;
  return (
    <button
      type="button"
      aria-label={label}
      title={shortcut ? `${label} (${shortcut})` : label}
      aria-haspopup={hasMenu ? "menu" : undefined}
      disabled={disabled}
      onClick={onClick}
      className={BUTTON_CLASS}
    >
      {icon}
    </button>
  );
}

// The segment over the sidebar: the sidebar toggle and Get Mail. It keeps
// its buttons, and its width collapses, while the sidebar is hidden.
function SidebarSegment({
  sidebarWidth,
  syncing,
  onCommand,
}: {
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
      <ToolbarButton
        label="Get Mail"
        shortcut="Ctrl+Shift+N"
        icon={<RefreshCw size={16} className={syncing ? "animate-spin" : undefined} />}
        onClick={() => onCommand({ kind: "getMail" })}
      />
    </div>
  );
}

// The segment over the list: the source title above its subtitle, and
// Compose, disabled until M3.
function ListSegment({ listWidth, title, subtitle }: { listWidth: number; title: string; subtitle: string }) {
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
      <ToolbarButton label="New Message" icon={<SquarePen size={16} />} disabled />
    </div>
  );
}

// The segment over the reader: the message actions, then the search field
// and the window controls at the far right.
function ReaderSegment({
  selection,
  canArchive,
  moveTargets,
  maximized,
  search,
  onCommand,
  onOpenMenu,
}: {
  selection: ToolbarSelection;
  canArchive: boolean;
  moveTargets: MoveTarget[];
  maximized: boolean;
  search: ReactNode;
  onCommand: (cmd: ToolbarCommand) => void;
  onOpenMenu: (kind: MenuKind) => (event: MouseEvent<HTMLButtonElement>) => void;
}) {
  const none = selection.count === 0;
  const markLabel = selection.seen ? "Mark as Unread" : "Mark as Read";
  return (
    <div data-segment="reader" data-tauri-drag-region className="flex h-full min-w-0 flex-1 items-center gap-1 px-2">
      <ToolbarButton
        label="Delete"
        shortcut="Delete"
        icon={<Trash2 size={16} />}
        disabled={none}
        onClick={() => onCommand({ kind: "delete" })}
      />
      <ToolbarButton
        label="Archive"
        shortcut="Ctrl+Alt+A"
        icon={<Archive size={16} />}
        disabled={none || !canArchive}
        onClick={() => onCommand({ kind: "archive" })}
      />
      <ToolbarButton label="Reply" icon={<Reply size={16} />} disabled />
      <ToolbarButton label="Reply All" icon={<ReplyAll size={16} />} disabled />
      <ToolbarButton label="Forward" icon={<Forward size={16} />} disabled />
      <ToolbarButton
        label="Flag"
        shortcut="Ctrl+Shift+L"
        icon={<Flag size={16} />}
        disabled={none}
        hasMenu
        onClick={onOpenMenu("flag")}
      />
      <ToolbarButton
        label={markLabel}
        shortcut="Ctrl+Shift+U"
        icon={selection.seen ? <Mail size={16} /> : <MailOpen size={16} />}
        disabled={none}
        onClick={() => onCommand({ kind: "toggleRead" })}
      />
      <ToolbarButton
        label="Move"
        icon={<FolderInput size={16} />}
        disabled={none || moveTargets.length === 0}
        hasMenu
        onClick={onOpenMenu("move")}
      />
      <div data-tauri-drag-region className="flex-1" />
      {search}
      <ToolbarButton label="Minimize" icon={<Minus size={16} />} onClick={() => onCommand({ kind: "minimize" })} />
      <ToolbarButton
        label={maximized ? "Restore" : "Maximize"}
        icon={maximized ? <Copy size={16} /> : <Square size={16} />}
        onClick={() => onCommand({ kind: "toggleMaximize" })}
      />
      <ToolbarButton label="Close" icon={<X size={16} />} onClick={() => onCommand({ kind: "close" })} />
    </div>
  );
}

/** Toolbar is the window's top strip, aligned with the panes. */
export function Toolbar(props: ToolbarProps) {
  const [menu, setMenu] = useState<MenuState | null>(null);

  const openMenu = (kind: MenuKind) => (event: MouseEvent<HTMLButtonElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    setMenu({ kind, x: rect.left, y: rect.bottom });
  };

  const choose = (id: string) => {
    if (id.startsWith("flag:")) props.onCommand({ kind: "flag", color: Number(id.slice(5)) });
    else if (id.startsWith("move:")) props.onCommand({ kind: "move", mailboxId: Number(id.slice(5)) });
  };

  // Only the drag regions themselves toggle maximize: a double click on a
  // button must not reach this as a window gesture.
  const dragDoubleClick = (event: MouseEvent<HTMLDivElement>) => {
    if ((event.target as HTMLElement).hasAttribute("data-tauri-drag-region")) {
      props.onCommand({ kind: "toggleMaximize" });
    }
  };

  return (
    <div
      role="toolbar"
      aria-label="Toolbar"
      data-tauri-drag-region
      className="flex h-[52px] shrink-0 items-center border-b border-separator bg-toolbar"
      onDoubleClick={dragDoubleClick}
    >
      <SidebarSegment sidebarWidth={props.sidebarWidth} syncing={props.syncing} onCommand={props.onCommand} />
      <ListSegment listWidth={props.listWidth} title={props.title} subtitle={props.subtitle} />
      <ReaderSegment
        selection={props.selection}
        canArchive={props.canArchive}
        moveTargets={props.moveTargets}
        maximized={props.maximized}
        search={props.search}
        onCommand={props.onCommand}
        onOpenMenu={openMenu}
      />
      {menu ? (
        <ContextMenu
          items={menu.kind === "flag" ? flagItems(props.selection) : moveItems(props.moveTargets)}
          x={menu.x}
          y={menu.y}
          onSelect={choose}
          onClose={() => setMenu(null)}
        />
      ) : null}
    </div>
  );
}
