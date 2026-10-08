// The toolbar, fed by the stores and the list (docs/specs/ui.md, Toolbar).
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { type Ref, useCallback, useEffect, useMemo, useState } from "react";

import { useClient } from "../data/session";
import { listQuery, useMail, useUI } from "../data/stores";
import type { ViewModel } from "../data/view";
import { SearchField } from "../features/search/SearchField";
import { type MoveTarget, Toolbar, type ToolbarCommand } from "../features/toolbar/Toolbar";
import { formatCount } from "../lib/format";
import { buildSidebar } from "../lib/mailboxTree";
import {
  archiveMailbox,
  compose,
  getMail,
  moveMessages,
  selectedSummaries,
  setFlagColor,
  toggleRead,
} from "./commands";
import { openSettings } from "./settings";

/** ToolbarContainer connects Toolbar to the stores, the list and the window. */
export function ToolbarContainer(props: {
  model: ViewModel | null;
  onDelete: (ids: number[]) => void;
  searchRef: Ref<HTMLInputElement>;
}) {
  const { model, onDelete } = props;
  const client = useClient();
  const { accounts, mailboxes, sync } = useMail();
  const ui = useUI();
  const [maximized, setMaximized] = useState(false);

  useEffect(() => {
    if (!isTauri()) return;
    const win = getCurrentWindow();
    const update = () => void win.isMaximized().then(setMaximized);
    update();
    const off = win.onResized(update);
    return () => void off.then((f) => f());
  }, []);

  const rows = selectedSummaries(model, ui.selected);
  const selection = {
    count: ui.selected.length,
    seen: rows.length > 0 && rows.every((r) => r.flags.seen),
    flagColor: rows[0]?.flags.flagColor ?? 0,
  };
  const archive = archiveMailbox(model, ui.selected, mailboxes);

  const moveTargets = useMemo((): MoveTarget[] => {
    const accountId = rows[0]?.accountId;
    const section = buildSidebar(accounts, mailboxes).find((s) => s.accountId === accountId);
    return (section?.items ?? []).flatMap((i) =>
      i.mailboxId !== undefined && i.selectable ? [{ mailboxId: i.mailboxId, label: i.label, depth: i.depth }] : [],
    );
  }, [accounts, mailboxes, rows[0]?.accountId]);

  const { title, subtitle } = useMemo(() => {
    const searching = ui.search !== "";
    const count = model?.ready ? model.count : null;
    if (searching) {
      return { title: "Search", subtitle: count === null ? "Searching…" : `${formatCount(count)} results` };
    }
    const src = ui.source;
    const inboxes = mailboxes.filter((mb) => mb.role === "inbox");
    const counts = (list: typeof mailboxes) => {
      const total = list.reduce((n, mb) => n + mb.total, 0);
      const unread = list.reduce((n, mb) => n + mb.unread, 0);
      return `${formatCount(total)} messages, ${formatCount(unread)} unread`;
    };
    if (src.kind === "allInboxes") return { title: "All Inboxes", subtitle: counts(inboxes) };
    if (src.kind === "flagged")
      return { title: "Flagged", subtitle: count === null ? "" : `${formatCount(count)} messages` };
    const mb = mailboxes.find((m) => m.id === src.mailboxId);
    const label = buildSidebar(accounts, mailboxes)
      .flatMap((s) => s.items)
      .find((i) => i.mailboxId === src.mailboxId)?.label;
    const readOnly = accounts.some((a) => a.id === mb?.accountId && a.readOnly);
    const subtitle = mb ? `${counts([mb])}${readOnly ? " · Read-only" : ""}` : "";
    return { title: label ?? mb?.name ?? "", subtitle };
  }, [ui.search, ui.source, model?.ready, model?.count, mailboxes, accounts]);

  const onCommand = useCallback(
    (cmd: ToolbarCommand) => {
      const ids = ui.selected;
      switch (cmd.kind) {
        case "toggleSidebar":
          ui.toggleSidebar();
          break;
        case "getMail":
          void getMail(
            client,
            accounts.map((a) => a.id),
          );
          break;
        case "delete":
          if (ids.length > 0) onDelete(ids);
          break;
        case "archive":
          void moveMessages(client, ids, archive, ui.source, mailboxes);
          break;
        case "flag":
          void setFlagColor(client, ids, cmd.color);
          break;
        case "toggleRead":
          void toggleRead(client, model, ids);
          break;
        case "move":
          void moveMessages(
            client,
            ids,
            mailboxes.find((mb) => mb.id === cmd.mailboxId),
            ui.source,
            mailboxes,
          );
          break;
        case "settings":
          void openSettings().catch((err: unknown) => console.warn("settings", err));
          break;
        case "compose":
        case "reply":
        case "replyAll":
        case "forward":
          void compose(client, cmd.kind, ids).catch((err: unknown) => console.warn("compose", err));
          break;
        case "minimize":
          if (isTauri()) void getCurrentWindow().minimize();
          break;
        case "toggleMaximize":
          if (isTauri()) void getCurrentWindow().toggleMaximize();
          break;
        case "close":
          if (isTauri()) void getCurrentWindow().close();
          break;
      }
    },
    [ui, client, accounts, archive, model, onDelete, mailboxes],
  );

  const syncing = Object.values(sync).some(
    (s) => s.phase !== "idle" && s.phase !== "offline" && s.phase !== "unauthorized" && s.phase !== "failed",
  );
  return (
    <Toolbar
      sidebarWidth={ui.sidebarVisible ? ui.sidebarWidth : 0}
      listWidth={ui.listWidth}
      title={title}
      subtitle={subtitle}
      syncing={syncing}
      selection={selection}
      canArchive={archive !== undefined}
      moveTargets={moveTargets}
      maximized={maximized}
      onCommand={onCommand}
      search={
        <SearchField
          value={ui.searchDraft}
          onChange={ui.setSearchDraft}
          onSearch={(text) => (text === "" ? ui.clearSearch() : ui.commitSearch(text))}
          onClear={ui.clearSearch}
          inputRef={props.searchRef}
        />
      }
    />
  );
}

/** useListQuery is the list's view query for the UI state. */
export function useListQuery() {
  const source = useUI((s) => s.source);
  const search = useUI((s) => s.search);
  const searchScope = useUI((s) => s.searchScope);
  const conversations = useUI((s) => s.conversations);
  return useMemo(
    () => listQuery({ source, search, searchScope, conversations }),
    [source, search, searchScope, conversations],
  );
}
