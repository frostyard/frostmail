// The People module's sidebar (docs/specs/pim-ui.md, People sidebar).
// Sections collapse independently; navigation walks only their visible rows.
import { BookUser, ChevronDown, Lock, Users } from "lucide-react";
import { type KeyboardEvent, type MouseEvent, useState } from "react";

/** AddressBookRow is one address book in the sidebar. */
export interface AddressBookRow {
  id: number;
  name: string;
  readOnly: boolean;
}

/** AddressBookSection is an account and its address books. */
export interface AddressBookSection {
  accountId: number;
  title: string;
  books: AddressBookRow[];
}

/** PeopleSidebarProps are the People sidebar's inputs. */
export interface PeopleSidebarProps {
  sections: AddressBookSection[];
  /** "all" for All Contacts, or the selected address book's ID. */
  selected: "all" | number;
  /** The sidebar has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  onSelect: (s: "all" | number) => void;
}

/** PeopleSidebar shows All Contacts and each account's address books. */
export function PeopleSidebar({ sections, selected, focused, onSelect }: PeopleSidebarProps) {
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set());
  const groups: {
    key: string;
    title: string;
    books: { id: "all" | number; name: string; readOnly: boolean }[];
  }[] = [
    { key: "people", title: "People", books: [{ id: "all", name: "All Contacts", readOnly: false }] },
    ...sections
      .filter((section) => section.books.length > 0)
      .map((section) => ({
        key: `account:${section.accountId}`,
        title: section.title,
        books: section.books,
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
    const book = groups
      .flatMap((group) => group.books)
      .find((book) => (book.id === "all" ? "all" : `book:${book.id}`) === key);
    if (book) onSelect(book.id);
  };
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    const ids = groups
      .filter((group) => !collapsed.has(group.key))
      .flatMap((group) => group.books.map((book) => book.id));
    const at = ids.indexOf(selected);
    let target: "all" | number | undefined;
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
      aria-label="Address Books"
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
              group.books.map((book) => {
                const Icon = book.id === "all" ? Users : BookUser;
                const active = book.id === selected;
                const contrast = active && focused;
                return (
                  <div
                    key={book.id}
                    role="treeitem"
                    tabIndex={-1}
                    aria-level={1}
                    aria-selected={active}
                    data-key={book.id === "all" ? "all" : `book:${book.id}`}
                    className={`mx-2 flex h-7 items-center gap-2 rounded-md pl-1 pr-1 ${active ? (focused ? "bg-accent text-accent-contrast" : "bg-selection-sidebar") : ""}`}
                  >
                    <Icon
                      size={16}
                      aria-hidden="true"
                      className={`shrink-0 ${contrast ? "text-accent-contrast" : "text-accent"}`}
                    />
                    <span className="truncate text-sidebar-row">{book.name}</span>
                    {book.readOnly && (
                      <Lock
                        size={12}
                        aria-label="Read-only"
                        className={`shrink-0 ${contrast ? "text-accent-contrast" : "text-secondary"}`}
                      />
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
