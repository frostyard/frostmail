// The main window: toolbar, sidebar, list and reader, the keyboard map and
// pane resizing (docs/specs/ui.md, Layout and Keyboard map).
import { useCallback, useEffect, useRef } from "react";

import { useClient } from "../data/session";
import { type Pane, useMail, useUI } from "../data/stores";
import { useView } from "../data/useView";
import { ScopeBar } from "../features/search/SearchField";
import { commandFor } from "../lib/keymap";
import { archiveMailbox, compose, getMail, toggleFlag, toggleRead } from "./commands";
import { ListContainer, type ListHandle } from "./ListContainer";
import { UndoToasts } from "./OutboxContainer";
import { ReaderContainer, type ReaderHandle } from "./ReaderContainer";
import { SidebarContainer } from "./SidebarContainer";
import { Splitter } from "./Splitter";
import { ToolbarContainer, useListQuery } from "./ToolbarContainer";

const SIDEBAR = { min: 160, max: 320 };
const LIST = { min: 280, max: 560 };
const READER_MIN = 360;

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));

function inTextField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA";
}

/** MainWindow lays out the panes and owns the list's view. */
export function MainWindow() {
  const client = useClient();
  const ui = useUI();
  const { accounts, mailboxes } = useMail();
  const query = useListQuery();
  const model = useView(query);
  const list = useRef<ListHandle>(null);
  const reader = useRef<ReaderHandle>(null);
  const search = useRef<HTMLInputElement>(null);
  const sidebarPane = useRef<HTMLDivElement>(null);
  const dragStart = useRef({ sidebar: ui.sidebarWidth, list: ui.listWidth });

  const onDelete = useCallback(
    (ids: number[]) => {
      void client.message.delete({ ids }).catch(() => {});
    },
    [client],
  );

  useEffect(() => {
    const focusPane = (dir: 1 | -1) => {
      const panes: Pane[] = ui.sidebarVisible ? ["sidebar", "list", "reader"] : ["list", "reader"];
      const i = panes.indexOf(ui.focus);
      const next = panes[(i + dir + panes.length) % panes.length] ?? "list";
      ui.setFocus(next);
      if (next === "list") list.current?.focus();
      else if (next === "reader") reader.current?.focus();
      else sidebarPane.current?.querySelector<HTMLElement>("[tabindex='0'],button")?.focus();
    };
    const onKey = (e: KeyboardEvent) => {
      const cmd = commandFor(e, inTextField(e.target));
      if (!cmd) return;
      const ids = ui.selected;
      let handled = true;
      switch (cmd) {
        case "focusSearch":
          search.current?.focus();
          search.current?.select();
          break;
        case "escape":
          if (ui.search !== "" || ui.searchDraft !== "") ui.clearSearch();
          else handled = false;
          break;
        case "getMail":
          void getMail(
            client,
            accounts.map((a) => a.id),
          );
          break;
        case "allInboxes":
          ui.setSource({ kind: "allInboxes" });
          break;
        case "toggleSidebar":
          ui.toggleSidebar();
          break;
        case "nextPane":
        case "previousPane":
          focusPane(cmd === "nextPane" ? 1 : -1);
          break;
        case "toggleRead":
          void toggleRead(client, model, ids);
          break;
        case "toggleFlag":
          void toggleFlag(client, model, ids);
          break;
        case "archive": {
          const mb = archiveMailbox(model, ids, mailboxes);
          if (mb && ids.length > 0) void client.message.move({ ids, mailboxId: mb.id });
          break;
        }
        case "pageDown":
        case "pageUp":
          if (ui.focus === "sidebar") handled = false;
          else reader.current?.page(cmd === "pageDown" ? 1 : -1);
          break;
        case "compose":
        case "reply":
        case "replyAll":
        case "forward":
          void compose(client, cmd, ids).catch((err: unknown) => console.warn("compose", err));
          break;
        default:
          handled = ui.focus === "list" && (list.current?.command(cmd) ?? false);
      }
      if (handled) e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [ui, client, accounts, mailboxes, model]);

  // No webview context menu anywhere; list rows open the app's own.
  useEffect(() => {
    const block = (e: MouseEvent) => {
      if (!inTextField(e.target)) e.preventDefault();
    };
    window.addEventListener("contextmenu", block);
    return () => window.removeEventListener("contextmenu", block);
  }, []);

  const width = typeof window === "undefined" ? 1200 : window.innerWidth;
  const resizeSidebar = (d: number) =>
    ui.setWidths({ sidebarWidth: clamp(dragStart.current.sidebar + d, SIDEBAR.min, SIDEBAR.max) });
  const resizeList = (d: number) =>
    ui.setWidths({
      listWidth: clamp(
        dragStart.current.list + d,
        LIST.min,
        Math.min(LIST.max, width - READER_MIN - (ui.sidebarVisible ? ui.sidebarWidth : 0)),
      ),
    });
  const endDrag = () => {
    dragStart.current = { sidebar: useUI.getState().sidebarWidth, list: useUI.getState().listWidth };
  };

  const source = ui.source;
  const sourceLabel =
    source.kind === "mailbox"
      ? (mailboxes.find((m) => m.id === source.mailboxId)?.name ?? "Mailbox")
      : source.kind === "allInboxes"
        ? "All Inboxes"
        : "Flagged";

  return (
    <div className="flex h-full flex-col">
      <ToolbarContainer model={model} onDelete={onDelete} searchRef={search} />
      {ui.search !== "" && (
        <ScopeBar
          scopes={[
            { key: "all", label: "All Mailboxes" },
            { key: "source", label: sourceLabel },
          ]}
          selected={ui.searchScope}
          onSelect={(k) => ui.setSearchScope(k === "source" ? "source" : "all")}
        />
      )}
      <div className="flex min-h-0 flex-1">
        {ui.sidebarVisible && (
          <>
            <div ref={sidebarPane} className="h-full shrink-0" style={{ width: ui.sidebarWidth }}>
              <SidebarContainer />
            </div>
            <Splitter label="Resize sidebar" onResize={resizeSidebar} onEnd={endDrag} />
          </>
        )}
        <div className="h-full shrink-0" style={{ width: ui.listWidth }}>
          <ListContainer ref={list} model={model} onDelete={onDelete} />
        </div>
        <Splitter label="Resize message list" onResize={resizeList} onEnd={endDrag} />
        <div className="h-full min-w-0 flex-1">
          <ReaderContainer ref={reader} />
        </div>
      </div>
      <UndoToasts />
    </div>
  );
}
