// The sidebar (docs/specs/ui.md, Sidebar): Favorites and one section per
// account, each a collapsible group of mailbox rows with a role icon, an
// unread count and the selection, plus the account's sync activity. The rows
// come from buildSidebar; the tree takes the clicks and the arrow keys, which
// move the selection among the visible selectable rows.
import {
  Archive,
  ChevronDown,
  CircleAlert,
  File,
  Flag,
  Folder,
  FolderCog,
  Inbox,
  LoaderCircle,
  type LucideIcon,
  Plus,
  Send,
  ShieldAlert,
  Star,
  Trash2,
  User,
} from "lucide-react";
import { type DragEvent, type KeyboardEvent, type MouseEvent, useState } from "react";

import type { SidebarIcon, SidebarItem, SidebarSection } from "../../lib/mailboxTree";

/** SyncIndicator is what the sidebar shows next to an account title. */
export interface SyncIndicator {
  state: "idle" | "syncing" | "error";
  /** The error, shown as a tooltip. */
  message?: string;
}

/** SidebarProps are the sidebar's inputs. */
export interface SidebarProps {
  sections: SidebarSection[];
  selectedKey: string | null;
  /** The sidebar has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  /** Sync state by account ID; missing means idle. */
  sync: Record<number, SyncIndicator>;
  onSelect: (key: string) => void;
  onAdd?: (sectionKey: string) => void;
  onContextMenu?: (key: string, x: number, y: number) => void;
  canDrop?: (key: string) => boolean;
  onDrop?: (key: string, data: string, copy: boolean) => void;
  dragType?: string;
}

const ICONS: Record<SidebarIcon, LucideIcon> = {
  inbox: Inbox,
  file: File,
  send: Send,
  "shield-alert": ShieldAlert,
  "trash-2": Trash2,
  archive: Archive,
  flag: Flag,
  folder: Folder,
  "folder-cog": FolderCog,
  star: Star,
  user: User,
};

const FLAG_CLASSES: Record<number, string> = {
  1: "text-flag-1",
  2: "text-flag-2",
  3: "text-flag-3",
  4: "text-flag-4",
  5: "text-flag-5",
  6: "text-flag-6",
  7: "text-flag-7",
};

const NAVIGATION = new Set(["ArrowDown", "ArrowUp", "Home", "End"]);

// The selectable rows of the open sections, in display order: what the
// arrow keys walk. A collapsed section contributes nothing.
function visibleKeys(sections: SidebarSection[], collapsed: ReadonlySet<string>): string[] {
  const keys: string[] = [];
  for (const section of sections) {
    if (collapsed.has(section.key)) continue;
    for (const item of section.items) {
      if (item.selectable) keys.push(item.key);
    }
  }
  return keys;
}

function selectable(sections: SidebarSection[], key: string): boolean {
  return sections.some((section) => section.items.some((item) => item.key === key && item.selectable));
}

function SyncDot({ indicator }: { indicator: SyncIndicator | undefined }) {
  if (indicator?.state === "syncing") {
    return <LoaderCircle size={12} className="shrink-0 animate-spin" aria-label="Syncing" />;
  }
  if (indicator?.state === "error") {
    return (
      <span className="flex shrink-0" title={indicator.message}>
        <CircleAlert size={12} aria-label="Sync error" />
      </span>
    );
  }
  return null;
}

function Row({
  item,
  selected,
  focused,
  dropTarget,
}: {
  item: SidebarItem;
  selected: boolean;
  focused: boolean;
  dropTarget: boolean;
}) {
  const contrast = selected && focused;
  const Icon = ICONS[item.icon];
  const iconColor = item.flagColor === undefined ? "text-accent" : (FLAG_CLASSES[item.flagColor] ?? "text-accent");
  const classes = [
    "mx-2 flex h-7 items-center gap-2 rounded-md pr-1",
    selected ? (focused ? "bg-accent text-accent-contrast" : "bg-selection-sidebar") : "",
    dropTarget && !selected ? "bg-selection-inactive" : "",
    item.selectable ? "" : "text-secondary",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <div
      role="treeitem"
      tabIndex={-1}
      data-key={item.key}
      aria-level={item.depth + 1}
      aria-selected={selected}
      aria-disabled={item.selectable ? undefined : true}
      className={classes}
      style={{ paddingLeft: 4 + 16 * item.depth }}
    >
      <Icon size={16} data-icon={item.icon} className={`shrink-0 ${contrast ? "text-accent-contrast" : iconColor}`} />
      <span className="text-sidebar-row flex-1 truncate">{item.label}</span>
      {item.unread > 0 && (
        <span className={`text-[12px] tabular-nums ${contrast ? "text-accent-contrast" : "text-secondary"}`}>
          {item.unread}
        </span>
      )}
    </div>
  );
}

/** Sidebar shows Favorites and each account's mailboxes as collapsible
 * sections, with the selection, the sync state and arrow-key navigation. */
export function Sidebar(props: SidebarProps) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const [dropTarget, setDropTarget] = useState<string | null>(null);

  const dropKey = (event: DragEvent<HTMLDivElement>) => {
    const key = (event.target as Element).closest("[data-key]")?.getAttribute("data-key");
    return key && props.canDrop?.(key) && props.dragType && event.dataTransfer.types.includes(props.dragType)
      ? key
      : null;
  };

  const toggle = (key: string) => {
    setCollapsed((previous) => {
      const next = new Set(previous);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  };

  const handleClick = (event: MouseEvent<HTMLDivElement>) => {
    const target = event.target as Element | null;
    const key = target?.closest("[data-key]")?.getAttribute("data-key");
    if (key !== undefined && key !== null && selectable(props.sections, key)) props.onSelect(key);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (
      (event.key === "ContextMenu" || (event.key === "F10" && event.shiftKey)) &&
      props.selectedKey &&
      props.onContextMenu
    ) {
      const row = event.currentTarget.querySelector(`[data-key="${props.selectedKey}"]`);
      const rect = row?.getBoundingClientRect();
      if (rect) {
        event.preventDefault();
        event.stopPropagation();
        props.onContextMenu(props.selectedKey, rect.left, rect.bottom);
      }
      return;
    }
    if (!NAVIGATION.has(event.key)) return;
    const keys = visibleKeys(props.sections, collapsed);
    const at = props.selectedKey === null ? -1 : keys.indexOf(props.selectedKey);
    let target: string | undefined;
    if (event.key === "ArrowDown") target = at < 0 ? keys[0] : keys[at + 1];
    else if (event.key === "ArrowUp") target = keys[at - 1];
    else if (event.key === "Home") target = keys[0];
    else target = keys[keys.length - 1];
    event.preventDefault();
    event.stopPropagation();
    if (target !== undefined && target !== props.selectedKey) props.onSelect(target);
  };

  return (
    <div
      role="tree"
      aria-label="Mailboxes"
      tabIndex={0}
      className="pt-2 outline-none"
      onContextMenu={(event) => {
        const key = (event.target as Element).closest("[data-key]")?.getAttribute("data-key");
        if (key && props.onContextMenu) {
          event.preventDefault();
          props.onContextMenu(key, event.clientX, event.clientY);
        }
      }}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      onDragOver={(event) => {
        const key = dropKey(event);
        setDropTarget(key);
        if (!key) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = event.altKey === true || event.ctrlKey === true ? "copy" : "move";
      }}
      onDragLeave={(event) => {
        const row = (event.target as Element).closest("[data-key]");
        if (event.relatedTarget instanceof Node && row?.contains(event.relatedTarget)) return;
        setDropTarget(null);
      }}
      onDrop={(event) => {
        setDropTarget(null);
        const key = dropKey(event);
        if (!key || !props.dragType) return;
        event.preventDefault();
        props.onDrop?.(
          key,
          event.dataTransfer.getData(props.dragType),
          event.altKey === true || event.ctrlKey === true,
        );
      }}
    >
      {props.sections.map((section) => {
        const open = !collapsed.has(section.key);
        return (
          <div key={section.key}>
            <div className="group flex items-center">
              <button
                type="button"
                aria-expanded={open}
                onClick={() => toggle(section.key)}
                className="flex h-[26px] flex-1 min-w-0 items-center gap-1 border-0 bg-transparent pl-3 text-left text-sidebar-section text-secondary"
              >
                <span className="truncate">{section.title}</span>
                {section.accountId !== undefined && <SyncDot indicator={props.sync[section.accountId]} />}
                <ChevronDown
                  size={12}
                  className={`shrink-0 opacity-0 group-hover:opacity-100 ${open ? "" : "-rotate-90"}`}
                />
              </button>
              {section.addLabel && props.onAdd && (
                <button
                  type="button"
                  aria-label={section.addLabel}
                  onClick={() => props.onAdd?.(section.key)}
                  className="mr-3 text-secondary opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100"
                >
                  <Plus size={14} />
                </button>
              )}
            </div>
            {open && (
              // biome-ignore lint/a11y/useSemanticElements: a tree's rows are grouped by role="group"; a fieldset would not belong in a tree.
              <div role="group">
                {section.items.map((item) => (
                  <Row
                    key={item.key}
                    item={item}
                    selected={item.key === props.selectedKey}
                    focused={props.focused}
                    dropTarget={item.key === dropTarget}
                  />
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
