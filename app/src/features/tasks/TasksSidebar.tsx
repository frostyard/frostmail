// The Tasks sidebar, with independently collapsible sections.
import { ChevronDown, Flag, List, ListChecks, Lock, Sun } from "lucide-react";
import { type KeyboardEvent, type MouseEvent, useState } from "react";
import type { TasksSource } from "../../lib/taskText";

/** TaskListRow is one task list in the sidebar. */
export interface TaskListRow {
  id: number;
  name: string;
  readOnly: boolean;
  /** Its open tasks. */
  count: number;
}

/** TaskListSection is an account and its task lists. */
export interface TaskListSection {
  accountId: number;
  title: string;
  lists: TaskListRow[];
}

/** TasksSidebarProps are the Tasks sidebar's inputs. */
export interface TasksSidebarProps {
  sections: TaskListSection[];
  /** The smart lists' counts: open tasks due by today, every open task, flagged messages. */
  counts: { today: number; all: number; flagged: number };
  selected: TasksSource;
  /** The sidebar has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  onSelect: (source: TasksSource) => void;
}

/** TasksSidebar shows the smart lists and each account's task lists. */
export function TasksSidebar({ sections, counts, selected, focused, onSelect }: TasksSidebarProps) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const groups: {
    key: string;
    title: string;
    lists: { id: TasksSource; name: string; readOnly: boolean; count: number }[];
  }[] = [
    {
      key: "tasks",
      title: "Tasks",
      lists: [
        { id: "today", name: "Today", readOnly: false, count: counts.today },
        { id: "all", name: "All Tasks", readOnly: false, count: counts.all },
        { id: "flagged", name: "Flagged Mail", readOnly: false, count: counts.flagged },
      ],
    },
    ...sections
      .filter((section) => section.lists.length > 0)
      .map((section) => ({
        key: `account:${section.accountId}`,
        title: section.title,
        lists: section.lists,
      })),
  ];
  const toggle = (key: string) =>
    setCollapsed((previous) => {
      const next = new Set(previous);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  const handleClick = (event: MouseEvent<HTMLDivElement>) => {
    const key = (event.target as Element | null)?.closest("[data-key]")?.getAttribute("data-key");
    const list = groups
      .flatMap((group) => group.lists)
      .find((list) => (typeof list.id === "number" ? `list:${list.id}` : list.id) === key);
    if (list) onSelect(list.id);
  };
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    const ids = groups
      .filter((group) => !collapsed.has(group.key))
      .flatMap((group) => group.lists.map((list) => list.id));
    const at = ids.indexOf(selected);
    let target: TasksSource | undefined;
    if (event.key === "ArrowDown") target = at < 0 ? ids[0] : ids[at + 1];
    else if (event.key === "ArrowUp") target = ids[at - 1];
    else if (event.key === "Home") target = ids[0];
    else target = ids[ids.length - 1];
    event.preventDefault();
    event.stopPropagation();
    if (target !== undefined && target !== selected) onSelect(target);
  };
  return (
    <div
      role="tree"
      aria-label="Task Lists"
      tabIndex={0}
      className="bg-sidebar pt-2 outline-none"
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      {groups.map((group) => {
        const open = !collapsed.has(group.key);
        return (
          <div key={group.key}>
            <button
              type="button"
              aria-expanded={open}
              onClick={() => toggle(group.key)}
              className="group flex h-[26px] w-full items-center gap-1 border-0 bg-transparent pl-3 text-left text-sidebar-section text-secondary"
            >
              <span className="truncate">{group.title}</span>
              <ChevronDown
                size={12}
                aria-hidden="true"
                className={`shrink-0 opacity-0 group-hover:opacity-100 ${open ? "" : "-rotate-90"}`}
              />
            </button>
            {open &&
              group.lists.map((list) => {
                const Icon =
                  list.id === "today" ? Sun : list.id === "all" ? ListChecks : list.id === "flagged" ? Flag : List;
                const active = list.id === selected;
                const contrast = active && focused;
                return (
                  <div
                    key={list.id}
                    role="treeitem"
                    tabIndex={-1}
                    aria-level={1}
                    aria-selected={active}
                    data-key={typeof list.id === "number" ? `list:${list.id}` : list.id}
                    className={`mx-2 flex h-7 items-center gap-2 rounded-md pl-1 pr-1 ${active ? (focused ? "bg-accent text-accent-contrast" : "bg-selection-sidebar") : ""}`}
                  >
                    <Icon
                      size={16}
                      aria-hidden="true"
                      className={`shrink-0 ${contrast ? "text-accent-contrast" : "text-accent"}`}
                    />
                    <span className="truncate flex-1 text-sidebar-row">{list.name}</span>
                    {list.readOnly && (
                      <Lock
                        size={12}
                        aria-label="Read-only"
                        className={`shrink-0 ${contrast ? "text-accent-contrast" : "text-secondary"}`}
                      />
                    )}
                    {list.count > 0 && (
                      <span
                        data-count
                        className={`text-list-date ${contrast ? "text-accent-contrast" : "text-secondary"}`}
                      >
                        {list.count}
                      </span>
                    )}
                  </div>
                );
              })}
          </div>
        );
      })}
    </div>
  );
}
