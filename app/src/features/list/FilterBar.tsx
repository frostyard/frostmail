/** ListFilter is the choice that narrows the message list. */
export type ListFilter = "all" | "unread" | "flagged" | "attachments";

/** LIST_FILTERS are the filter bar's choices in display order. */
export const LIST_FILTERS: { key: ListFilter; label: string }[] = [
  { key: "all", label: "All" },
  { key: "unread", label: "Unread" },
  { key: "flagged", label: "Flagged" },
  { key: "attachments", label: "Attachments" },
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
  }
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
        const style = active
          ? "bg-selection-inactive text-primary"
          : "bg-transparent text-secondary hover:text-primary";
        return (
          <button
            key={key}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(key)}
            className={`h-[22px] rounded-md border-0 px-2 text-[12px] leading-4 ${style}`}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
