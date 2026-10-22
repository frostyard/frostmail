import { Menu as ContextMenu, type MenuEntry as MenuItem } from "@frostyard/lodge-ui/react";
import { ChevronDown } from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";

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

/** emptyTextIn is emptyText naming the source the filter narrows: "No
 *  Flagged Messages in All Inboxes". All narrows nothing, so it names none. */
export function emptyTextIn(filter: ListFilter, scope: string): string {
  const text = emptyText(filter);
  return filter === "all" || scope === "" ? text : `${text} in ${scope}`;
}

function filterStyle(active: boolean): string {
  return active ? "bg-selection-inactive text-primary" : "bg-transparent text-secondary hover:text-primary";
}

const TAB_CLASS = "h-[22px] shrink-0 whitespace-nowrap rounded-md border-0 px-2 text-[12px] leading-4";
const MORE_CLASS = `flex items-center gap-1 ${TAB_CLASS}`;

function moreChoice(filter: ListFilter, count: number) {
  return [...LIST_FILTERS.slice(count), ...MORE_FILTERS].find(({ key }) => key === filter);
}

/** fitFilters keeps the longest leading run that fits with More's current label.
 * available excludes the toolbar's padding. An unmeasured bar shows all tabs. */
export function fitFilters(
  available: number,
  tabWidths: readonly number[],
  moreWidth: (label: string) => number,
  gap: number,
  filter: ListFilter = "all",
): number {
  if (available <= 0) return LIST_FILTERS.length;
  for (let count = LIST_FILTERS.length; count > 1; count--) {
    const tabs = tabWidths.slice(0, count).reduce((sum, width) => sum + width, 0);
    if (tabs + moreWidth(moreChoice(filter, count)?.label ?? "More") + gap * count <= available) return count;
  }
  return 1; // All and More always stay.
}

function MoreFilters({
  filter,
  count,
  onChange,
}: {
  filter: ListFilter;
  count: number;
  onChange: (filter: ListFilter) => void;
}) {
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const hidden = LIST_FILTERS.slice(count);
  const choices = [...hidden, ...MORE_FILTERS];
  const chosen = moreChoice(filter, count);
  const item = ({ key, label }: (typeof LIST_FILTERS)[number]): MenuItem => ({
    kind: "item",
    id: key,
    label,
    checked: key === filter,
  });
  const items: MenuItem[] = [
    ...hidden.map(item),
    ...(hidden.length ? [{ kind: "separator" } as const] : []),
    ...MORE_FILTERS.map(item),
  ];
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
        className={`${MORE_CLASS} ${filterStyle(chosen !== undefined)}`}
      >
        {chosen?.label ?? "More"}
        <ChevronDown size={12} aria-hidden="true" />
      </button>
      {menu ? (
        <ContextMenu
          items={items}
          x={menu.x}
          y={menu.y}
          onSelect={(id) => {
            const selected = choices.find(({ key }) => key === id);
            if (selected) onChange(selected.key);
          }}
          onClose={() => setMenu(null)}
        />
      ) : null}
    </>
  );
}

interface Measurements {
  available: number;
  tabs: number[];
  more: Map<string, number>;
  gap: number;
}

const MORE_LABELS = ["More", ...LIST_FILTERS.map(({ label }) => label), ...MORE_FILTERS.map(({ label }) => label)];

/** FilterBar offers the filters above the message list. */
export function FilterBar({ filter, onChange }: { filter: ListFilter; onChange: (filter: ListFilter) => void }) {
  const toolbar = useRef<HTMLDivElement>(null);
  const measuring = useRef<HTMLDivElement>(null);
  const [widths, setWidths] = useState<Measurements | null>(null);
  useLayoutEffect(() => {
    const bar = toolbar.current;
    const row = measuring.current;
    if (!bar || !row) return;
    const measure = () => {
      const style = getComputedStyle(bar);
      const buttons = [...row.querySelectorAll("button")].map((button) => button.getBoundingClientRect().width);
      setWidths({
        available: Math.max(
          0,
          bar.getBoundingClientRect().width -
            (Number.parseFloat(style.paddingLeft) || 0) -
            (Number.parseFloat(style.paddingRight) || 0) -
            (Number.parseFloat(style.borderLeftWidth) || 0) -
            (Number.parseFloat(style.borderRightWidth) || 0),
        ),
        tabs: buttons.slice(0, LIST_FILTERS.length),
        more: new Map(MORE_LABELS.map((label, index) => [label, buttons[LIST_FILTERS.length + index] ?? 0])),
        gap: Number.parseFloat(style.columnGap) || 0,
      });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(bar);
    // The natural row also changes when the font loads, without a bar resize.
    observer.observe(row);
    return () => observer.disconnect();
  }, []);
  const count = widths
    ? fitFilters(widths.available, widths.tabs, (label) => widths.more.get(label) ?? 0, widths.gap, filter)
    : LIST_FILTERS.length;
  return (
    <div
      ref={toolbar}
      role="toolbar"
      aria-label="Filter messages"
      className="relative flex h-[32px] min-w-0 flex-1 items-center gap-1 border-b border-separator bg-window px-3"
    >
      {LIST_FILTERS.slice(0, count).map(({ key, label }) => {
        const active = key === filter;
        return (
          <button
            key={key}
            type="button"
            aria-pressed={active}
            onClick={() => onChange(key)}
            className={`${TAB_CLASS} ${filterStyle(active)}`}
          >
            {label}
          </button>
        );
      })}
      <MoreFilters filter={filter} count={count} onChange={onChange} />
      <div aria-hidden="true" inert className="pointer-events-none invisible absolute inset-0 overflow-hidden">
        <div ref={measuring} className="flex w-max items-center gap-1">
          {LIST_FILTERS.map(({ key, label }) => (
            <button key={key} type="button" tabIndex={-1} className={`${TAB_CLASS} ${filterStyle(false)}`}>
              {label}
            </button>
          ))}
          {MORE_LABELS.map((label) => (
            <button key={label} type="button" tabIndex={-1} className={`${MORE_CLASS} ${filterStyle(false)}`}>
              {label}
              <ChevronDown size={12} aria-hidden="true" />
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
