// The message list: a virtualized view with selection, keyboard commands
// and the row context menu (docs/specs/ui.md, Message list and Behavior).
import { useVirtualizer } from "@tanstack/react-virtual";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";

import { useClient } from "../data/session";
import { useMail, useUI } from "../data/stores";
import type { ViewModel } from "../data/view";
import { MessageRow, ROW_HEIGHT, type SelectMode } from "../features/list/MessageRow";
import { ContextMenu, type MenuItem } from "../features/menu/ContextMenu";
import { FLAG_NAMES } from "../lib/flags";
import type { Command } from "../lib/keymap";
import {
  archiveMailbox,
  compose,
  moveMessages,
  rangeIds,
  selectedSummaries,
  setFlagColor,
  step,
  toggleRead,
} from "./commands";
import { openDraftMessage } from "./compose";

/** ListHandle lets the window send keyboard commands to the list. */
export interface ListHandle {
  command: (c: Command) => boolean;
  focus: () => void;
}

interface Menu {
  x: number;
  y: number;
  ids: number[];
}

/** ListContainer shows the list's view model. */
export const ListContainer = forwardRef<ListHandle, { model: ViewModel | null; onDelete: (ids: number[]) => void }>(
  function ListContainer({ model, onDelete }, ref) {
    const client = useClient();
    const mailboxes = useMail((s) => s.mailboxes);
    const { selected, anchor, focus, conversations, source, select, setFocus } = useUI();
    const scroller = useRef<HTMLDivElement>(null);
    const [menu, setMenu] = useState<Menu | null>(null);
    const [now, setNow] = useState(() => new Date());
    const lastIndex = useRef(0);
    const count = model?.count ?? 0;

    useEffect(() => {
      const t = setInterval(() => setNow(new Date()), 60_000);
      return () => clearInterval(t);
    }, []);

    const virtualizer = useVirtualizer({
      count,
      getScrollElement: () => scroller.current,
      estimateSize: () => ROW_HEIGHT,
      overscan: 8,
    });
    const items = virtualizer.getVirtualItems();
    const first = items[0]?.index ?? 0;
    const last = items[items.length - 1]?.index ?? 0;
    useEffect(() => {
      model?.ensure(Math.max(0, first - 50), last + 50);
    }, [model, first, last]);

    // Keep the selection on its rows; if they all vanished (deleted, moved
    // out), select the row that took their place.
    const version = model?.getVersion() ?? 0;
    // biome-ignore lint/correctness/useExhaustiveDependencies: version re-runs this after every view change.
    useEffect(() => {
      if (!model || selected.length === 0) return;
      const found = selected.map((id) => model.indexOf(id)).filter((i) => i >= 0);
      if (found.length > 0) {
        lastIndex.current = Math.min(...found);
        return;
      }
      if (model.count === 0) {
        select([], null);
        return;
      }
      const row = model.row(Math.min(lastIndex.current, model.count - 1));
      if (row) select([row.id], row.id);
    }, [model, version, selected, select]);

    const selectIndex = useCallback(
      (i: number, extend: boolean) => {
        if (!model) return;
        const row = model.row(i);
        if (!row) return;
        if (extend && anchor !== null) {
          const a = model.indexOf(anchor);
          select(a >= 0 ? rangeIds(model, a, i) : [row.id]);
        } else {
          select([row.id], row.id);
        }
        virtualizer.scrollToIndex(i, { align: "auto" });
      },
      [model, anchor, select, virtualizer],
    );

    const onSelect = useCallback(
      (id: number, mode: SelectMode) => {
        if (!model) return;
        setFocus("list");
        if (mode === "toggle") {
          select(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id], id);
        } else if (mode === "range" && anchor !== null) {
          const a = model.indexOf(anchor);
          const b = model.indexOf(id);
          select(a >= 0 && b >= 0 ? rangeIds(model, a, b) : [id]);
        } else {
          select([id], id);
        }
      },
      [model, selected, anchor, select, setFocus],
    );

    // A message in a Drafts mailbox opens in a compose window.
    const openIfDraft = useCallback(
      (id: number) => {
        const row = model?.row(model.indexOf(id));
        const drafts = new Set(mailboxes.filter((mb) => mb.role === "drafts").map((mb) => mb.id));
        if (!row?.mailboxIds.some((mb) => drafts.has(mb))) return;
        void openDraftMessage(client, id).catch((err: unknown) => console.warn("open draft", err));
      },
      [model, mailboxes, client],
    );

    const openMenu = useCallback(
      (id: number, x: number, y: number) => {
        const ids = selected.includes(id) ? selected : [id];
        if (!selected.includes(id)) select([id], id);
        setMenu({ x, y, ids });
      },
      [selected, select],
    );

    useImperativeHandle(
      ref,
      () => ({
        focus: () => scroller.current?.focus(),
        command: (c) => {
          if (!model) return false;
          const current = selected.length > 0 ? model.indexOf(selected[selected.length - 1] ?? -1) : -1;
          switch (c) {
            case "previous":
            case "next": {
              const i = step(model, selected, c === "next" ? 1 : -1);
              if (i !== null) selectIndex(i, false);
              return true;
            }
            case "extendPrevious":
            case "extendNext": {
              if (current < 0) return true;
              selectIndex(Math.max(0, Math.min(model.count - 1, current + (c === "extendNext" ? 1 : -1))), true);
              return true;
            }
            case "first":
              selectIndex(0, false);
              return true;
            case "last":
              selectIndex(model.count - 1, false);
              return true;
            case "selectAll":
              select(rangeIds(model, 0, model.count - 1));
              return true;
            case "delete":
              if (selected.length > 0) onDelete(selected);
              return true;
            case "open": {
              const id = selected[selected.length - 1];
              if (id !== undefined) openIfDraft(id);
              return true;
            }
            case "contextMenu": {
              const id = selected[0];
              const rect = scroller.current?.getBoundingClientRect();
              if (id !== undefined && rect) openMenu(id, rect.left + 40, rect.top + 40);
              return true;
            }
            default:
              return false;
          }
        },
      }),
      [model, selected, selectIndex, select, onDelete, openMenu, openIfDraft],
    );

    const menuItems = useMemo((): MenuItem[] => {
      if (!menu || !model) return [];
      const rows = selectedSummaries(model, menu.ids);
      const allSeen = rows.length > 0 && rows.every((r) => r.flags.seen);
      const accountId = rows[0]?.accountId;
      const targets = mailboxes.filter((mb) => mb.accountId === accountId && !rows[0]?.mailboxIds.includes(mb.id));
      const items: MenuItem[] = [
        { kind: "item", id: "reply", label: "Reply", shortcut: "Ctrl+R" },
        { kind: "item", id: "replyAll", label: "Reply All", shortcut: "Ctrl+Shift+R" },
        { kind: "item", id: "forward", label: "Forward", shortcut: "Ctrl+Shift+F" },
        { kind: "separator" },
        { kind: "item", id: "read", label: allSeen ? "Mark as Unread" : "Mark as Read", shortcut: "Ctrl+Shift+U" },
        {
          kind: "submenu",
          id: "flag",
          label: "Flag",
          items: [
            ...FLAG_NAMES.map(
              (name, i): MenuItem => ({
                kind: "item",
                id: `flag:${i + 1}`,
                label: name,
                checked: rows[0]?.flags.flagColor === i + 1,
              }),
            ),
            { kind: "separator" },
            { kind: "item", id: "flag:0", label: "Clear Flag", disabled: !rows.some((r) => r.flags.flagged) },
          ],
        },
        {
          kind: "submenu",
          id: "move",
          label: "Move to",
          disabled: targets.length === 0,
          items: targets.map((mb): MenuItem => ({ kind: "item", id: `move:${mb.id}`, label: mb.path })),
        },
        { kind: "separator" },
      ];
      if (archiveMailbox(model, menu.ids, mailboxes)) {
        items.push({ kind: "item", id: "archive", label: "Archive", shortcut: "Ctrl+Alt+A" });
      }
      items.push({ kind: "item", id: "delete", label: "Delete", shortcut: "Delete" });
      return items;
    }, [menu, model, mailboxes]);

    const onMenuSelect = useCallback(
      (id: string) => {
        if (!menu) return;
        const ids = menu.ids;
        if (id === "reply" || id === "replyAll" || id === "forward") {
          void compose(client, id, ids).catch((err: unknown) => console.warn("compose", err));
        } else if (id === "read") void toggleRead(client, model, ids);
        else if (id === "delete") onDelete(ids);
        else if (id === "archive")
          void moveMessages(client, ids, archiveMailbox(model, ids, mailboxes), source, mailboxes);
        else if (id.startsWith("flag:")) void setFlagColor(client, ids, Number(id.slice(5)));
        else if (id.startsWith("move:")) {
          const to = mailboxes.find((mb) => mb.id === Number(id.slice(5)));
          void moveMessages(client, ids, to, source, mailboxes);
        }
      },
      [menu, client, model, mailboxes, onDelete, source],
    );

    const focused = focus === "list";
    return (
      <div
        ref={scroller}
        role="listbox"
        aria-label="Messages"
        aria-multiselectable="true"
        tabIndex={0}
        onFocus={() => setFocus("list")}
        className="h-full overflow-y-auto bg-window outline-none"
      >
        {model?.error ? (
          <div className="flex h-full flex-col items-center justify-center gap-1 px-6 text-center text-secondary">
            <span className="text-empty">Messages could not be loaded</span>
            <span className="text-[12px]">{model.error}</span>
          </div>
        ) : model?.ready && count === 0 ? (
          <div className="flex h-full items-center justify-center text-empty text-secondary">No Messages</div>
        ) : (
          <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
            {items.map((v) => {
              const row = model?.row(v.index);
              return (
                // biome-ignore lint/a11y/noStaticElementInteractions: double-click opens a draft; Enter does the same from the keyboard (keymap "open").
                <div
                  key={v.key}
                  style={{ position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${v.start}px)` }}
                  onDoubleClick={() => row && openIfDraft(row.id)}
                >
                  {row ? (
                    <MessageRow
                      message={row}
                      selected={selected.includes(row.id)}
                      focused={focused}
                      showThreadCount={conversations}
                      now={now}
                      onSelect={onSelect}
                      onContextMenu={openMenu}
                    />
                  ) : (
                    <div style={{ height: ROW_HEIGHT }} />
                  )}
                </div>
              );
            })}
          </div>
        )}
        {menu && (
          <ContextMenu
            items={menuItems}
            x={menu.x}
            y={menu.y}
            onSelect={onMenuSelect}
            onClose={() => {
              setMenu(null);
              scroller.current?.focus();
            }}
          />
        )}
      </div>
    );
  },
);
