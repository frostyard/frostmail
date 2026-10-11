import { ChevronDown } from "lucide-react";
import { useState } from "react";

import type { ViewSort } from "../../rpc/gen/api";
import { ContextMenu, type MenuItem } from "../menu/ContextMenu";

/** SORTS are the list's sort fields in display order. */
export const SORTS: { key: ViewSort; label: string }[] = [
  { key: "date", label: "Date" },
  { key: "from", label: "From" },
  { key: "to", label: "To" },
  { key: "subject", label: "Subject" },
  { key: "size", label: "Size" },
  { key: "flags", label: "Flags" },
  { key: "unread", label: "Unread" },
  { key: "attachments", label: "Attachments" },
];

/** SortMenuProps are the list's sort and view options and their handlers. */
export interface SortMenuProps {
  sort: ViewSort;
  ascending: boolean;
  conversations: boolean;
  contactPhotos: boolean;
  onSort: (sort: ViewSort) => void;
  onAscending: (ascending: boolean) => void;
  onConversations: (conversations: boolean) => void;
  onContactPhotos: (contactPhotos: boolean) => void;
}

/** SortMenu offers the list's ordering and view options. */
export function SortMenu(props: SortMenuProps) {
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const items: MenuItem[] = [
    ...SORTS.map(({ key, label }): MenuItem => ({ kind: "item", id: key, label, checked: key === props.sort })),
    { kind: "separator" },
    { kind: "item", id: "ascending", label: "Ascending", checked: props.ascending },
    { kind: "item", id: "descending", label: "Descending", checked: !props.ascending },
    { kind: "separator" },
    { kind: "item", id: "conversations", label: "Conversations", checked: props.conversations },
    { kind: "item", id: "contactPhotos", label: "Contact Photos", checked: props.contactPhotos },
  ];
  const choose = (id: string) => {
    const field = SORTS.find(({ key }) => key === id);
    if (field) props.onSort(field.key);
    else if (id === "ascending" || id === "descending") props.onAscending(id === "ascending");
    else if (id === "conversations") props.onConversations(!props.conversations);
    else if (id === "contactPhotos") props.onContactPhotos(!props.contactPhotos);
  };
  return (
    <>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={menu !== null}
        className="flex h-[22px] shrink-0 items-center gap-1 rounded-md border-0 bg-transparent px-2 text-[12px] leading-4 text-secondary hover:text-primary"
        onClick={(event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          setMenu({ x: rect.left, y: rect.bottom });
        }}
      >
        Sort by {SORTS.find(({ key }) => key === props.sort)?.label}
        <ChevronDown size={12} aria-hidden="true" />
      </button>
      {menu ? (
        <ContextMenu items={items} x={menu.x} y={menu.y} onSelect={choose} onClose={() => setMenu(null)} />
      ) : null}
    </>
  );
}
