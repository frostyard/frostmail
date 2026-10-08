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
  Inbox,
  LoaderCircle,
  type LucideIcon,
  Send,
  ShieldAlert,
  Trash2,
} from "lucide-react";
import { type KeyboardEvent, type MouseEvent, useState } from "react";

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

function Row({ item, selected, focused }: { item: SidebarItem; selected: boolean; focused: boolean }) {
  const contrast = selected && focused;
  const Icon = ICONS[item.icon];
  const classes = [
    "mx-2 flex h-7 items-center gap-2 rounded-md pr-1",
    selected ? (focused ? "bg-accent text-accent-contrast" : "bg-selection-sidebar") : "",
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
      <Icon
        size={16}
        data-icon={item.icon}
        className={`shrink-0 ${contrast ? "text-accent-contrast" : "text-accent"}`}
      />
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
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      {props.sections.map((section) => {
        const open = !collapsed.has(section.key);
        return (
          <div key={section.key}>
            <button
              type="button"
              aria-expanded={open}
              onClick={() => toggle(section.key)}
              className="group flex h-[26px] w-full items-center gap-1 border-0 bg-transparent pl-3 text-left text-sidebar-section text-secondary"
            >
              <span className="truncate">{section.title}</span>
              {section.accountId !== undefined && <SyncDot indicator={props.sync[section.accountId]} />}
              <ChevronDown
                size={12}
                className={`shrink-0 opacity-0 group-hover:opacity-100 ${open ? "" : "-rotate-90"}`}
              />
            </button>
            {open && (
              // biome-ignore lint/a11y/useSemanticElements: a tree's rows are grouped by role="group"; a fieldset would not belong in a tree.
              <div role="group">
                {section.items.map((item) => (
                  <Row key={item.key} item={item} selected={item.key === props.selectedKey} focused={props.focused} />
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
