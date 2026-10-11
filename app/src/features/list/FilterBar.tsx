import { ChevronDown } from "lucide-react";
import { useState } from "react";

import { ContextMenu } from "../menu/ContextMenu";

/** ListFilter is the choice that narrows the message list. */
export type ListFilter = "all" | "unread" | "flagged" | "attachments" | "toMe" | "ccMe" | "vips";

/** LIST_FILTERS are the filter bar's choices in display order. */
export const LIST_FILTERS: { key: ListFilter; label: string }[] = [
  { key: "all", label: "All" },
  { key: "unread", label: "Unread" },
  { key: "flagged", label: "Flagged" },
  { key: "attachments", label: "Attachments" },
];

/** MORE_FILTERS are the additional choices in the More menu. */
export const MORE_FILTERS: { key: ListFilter; label: string }[] = [
  { key: "toMe", label: "To Me" },
  { key: "ccMe", label: "Cc Me" },
  { key: "vips", label: "From VIPs" },
];

/** emptyText names an empty message list for its filter. */
export function emptyText(filter: ListFilter): string {
  switch (filter) {
    case "all":
      return "No Messages";
    case "unread":
      return "No Unread Messages";
    case "flagged":
      return "No Flagged Messages";
    case "attachments":
      return "No Messages with Attachments";
    case "toMe":
      return "No Messages to You";
    case "ccMe":
      return "No Messages Cc'd to You";
    case "vips":
      return "No Messages from VIPs";
  }
}

function filterStyle(active: boolean): string {
  return active ? "bg-selection-inactive text-primary" : "bg-transparent text-secondary hover:text-primary";
}

function MoreFilters({ filter, onChange }: { filter: ListFilter; onChange: (filter: ListFilter) => void }) {
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const chosen = MORE_FILTERS.find(({ key }) => key === filter);
  return (
    <>
      <button
        type="button"
        aria-label="More filters"
        aria-haspopup="menu"
        aria-expanded={menu !== null}
        aria-pressed={chosen !== undefined}
        onClick={(event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          setMenu({ x: rect.left, y: rect.bottom });
        }}
        className={`flex h-[22px] items-center gap-1 rounded-md border-0 px-2 text-[12px] leading-4 ${filterStyle(chosen !== undefined)}`}
      >
        {chosen?.label ?? "More"}
        <ChevronDown size={12} aria-hidden="true" />
      </button>
      {menu ? (
        <ContextMenu
          items={MORE_FILTERS.map(({ key, label }) => ({ kind: "item", id: key, label, checked: key === filter }))}
          x={menu.x}
          y={menu.y}
          onSelect={(id) => {
            const selected = MORE_FILTERS.find(({ key }) => key === id);
            if (selected) onChange(selected.key);
          }}
          onClose={() => setMenu(null)}
        />
      ) : null}
    </>
  );
}

/** FilterBar offers the filters above the message list. */
export function FilterBar({ filter, onChange }: { filter: ListFilter; onChange: (filter: ListFilter) => void }) {
  return (
    <div
      role="toolbar"
      aria-label="Filter messages"
      className="flex h-[32px] shrink-0 items-center gap-1 border-b border-separator bg-window px-3"
    >
      {LIST_FILTERS.map(({ key, label }) => {
        const active = key === filter;
        return (
          <button
            key={key}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(key)}
            className={`h-[22px] rounded-md border-0 px-2 text-[12px] leading-4 ${filterStyle(active)}`}
          >
            {label}
          </button>
        );
      })}
      <MoreFilters filter={filter} onChange={onChange} />
    </div>
  );
}
