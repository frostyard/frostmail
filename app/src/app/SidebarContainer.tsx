// The sidebar, fed by the mail store (docs/specs/ui.md, Sidebar).
import { isTauri } from "@tauri-apps/api/core";
import { ask } from "@tauri-apps/plugin-dialog";
import { useMemo, useState } from "react";

import { useClient } from "../data/session";
import { sourceFromKey, sourceKey, useMail, useUI } from "../data/stores";
import { ContextMenu, type MenuItem } from "../features/menu/ContextMenu";
import { MailboxSheet, type MailboxSheetMode } from "../features/sidebar/MailboxSheet";
import { ModuleBar } from "../features/sidebar/ModuleBar";
import { Sidebar, type SyncIndicator } from "../features/sidebar/Sidebar";
import { buildSidebar, vipGroups } from "../lib/mailboxTree";
import type { Mailbox, MailboxRole } from "../rpc/gen/api";
import { OutboxSection, SendLaterSection } from "./OutboxContainer";
import { useSidebarCounts } from "./useSidebarCounts";
import { useSmartCounts } from "./useSmartCounts";

const ROLES: [MailboxRole, string][] = [
  ["drafts", "Drafts"],
  ["sent", "Sent"],
  ["junk", "Junk"],
  ["trash", "Trash"],
  ["archive", "Archive"],
];

function mailboxMenu(mailbox: Mailbox, readOnly: boolean, favorite: boolean): MenuItem[] {
  const locked = readOnly || mailbox.role !== "none";
  const items: MenuItem[] = [
    { kind: "item", id: "new", label: "New Mailbox…", disabled: readOnly },
    { kind: "item", id: "rename", label: "Rename Mailbox…", disabled: locked },
    { kind: "item", id: "move", label: "Move Mailbox…", disabled: locked },
    { kind: "item", id: "delete", label: "Delete Mailbox…", disabled: locked },
  ];
  if (mailbox.role !== "inbox" && !mailbox.label) {
    items.push(
      { kind: "separator" },
      {
        kind: "submenu",
        id: "role",
        label: "Use This Mailbox For",
        disabled: readOnly,
        items: ROLES.map(([role, label]) => ({
          kind: "item",
          id: `role:${role}`,
          label,
          checked: mailbox.role === role,
          disabled: readOnly,
        })),
      },
    );
  }
  if (mailbox.role === "trash" || mailbox.role === "junk") {
    if (items.at(-1)?.kind !== "submenu") items.push({ kind: "separator" });
    items.push({
      kind: "item",
      id: "erase",
      label: mailbox.role === "trash" ? "Erase Deleted Items…" : "Erase Junk Mail…",
      disabled: readOnly,
    });
  }
  items.push(
    { kind: "separator" },
    { kind: "item", id: "favorite", label: favorite ? "Remove from Favorites" : "Add to Favorites" },
  );
  return items;
}

async function confirmMailbox(mailbox: Mailbox, erase: boolean): Promise<boolean> {
  const text = erase
    ? `Erase the ${mailbox.total} messages in "${mailbox.name}"? They cannot be recovered.`
    : `Delete the mailbox "${mailbox.name}" and the messages in it?`;
  const title = erase ? (mailbox.role === "trash" ? "Erase Deleted Items?" : "Erase Junk Mail?") : "Delete Mailbox?";
  return isTauri() ? ask(text, { title, kind: "warning", okLabel: erase ? "Erase" : "Delete" }) : window.confirm(text);
}

interface MailboxEdit {
  mode: MailboxSheetMode;
  accountId: number;
  id?: number;
  name: string;
  parentId?: number;
}

/** SidebarContainer connects Sidebar to the stores. */
export function SidebarContainer() {
  const client = useClient();
  const [menu, setMenu] = useState<{ kind: "smart" | "mailbox"; id: number; x: number; y: number } | null>(null);
  const { accounts, mailboxes, sync, vips, settings, smarts } = useMail();
  const [sheet, setSheet] = useState<MailboxEdit | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const menuMailbox = menu?.kind === "mailbox" ? mailboxes.find((m) => m.id === menu.id) : undefined;
  const openSheet = (edit: MailboxEdit) => {
    setError(undefined);
    setSheet(edit);
  };
  const editMailbox = (mailbox: Mailbox, mode: MailboxSheetMode) => {
    const index = mailbox.delimiter ? mailbox.path.lastIndexOf(mailbox.delimiter) : -1;
    const parent =
      mode === "new"
        ? mailbox
        : mailboxes.find(
            (m) => m.accountId === mailbox.accountId && index >= 0 && m.path === mailbox.path.slice(0, index),
          );
    openSheet({
      mode,
      accountId: mailbox.accountId,
      id: mailbox.id,
      name: mode === "new" ? "" : mailbox.name,
      ...(parent ? { parentId: parent.id } : {}),
    });
  };
  const saveMailbox = async (draft: { name: string; parentId?: number }) => {
    if (!sheet || busy) return;
    setBusy(true);
    setError(undefined);
    try {
      if (sheet.mode === "new") await client.mailbox.create({ accountId: sheet.accountId, ...draft });
      else if (sheet.id !== undefined && sheet.mode === "rename")
        await client.mailbox.rename({ id: sheet.id, name: draft.name });
      else if (sheet.id !== undefined)
        await client.mailbox.move({
          id: sheet.id,
          ...(draft.parentId === undefined ? {} : { parentId: draft.parentId }),
        });
      setSheet((current) => (current === sheet ? null : current));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };
  const mailboxAction = async (mailbox: Mailbox, action: string) => {
    if (action === "new" || action === "rename" || action === "move") editMailbox(mailbox, action);
    else if (action === "favorite") {
      const favorites = useMail.getState().settings?.favorites ?? [];
      await client.settings.set({
        favorites: favorites.includes(mailbox.id)
          ? favorites.filter((id) => id !== mailbox.id)
          : [...favorites, mailbox.id],
      });
    } else if (action.startsWith("role:")) {
      const role = ROLES.find(([role]) => action === `role:${role}`)?.[0];
      if (role) await client.mailbox.setRole({ id: mailbox.id, role });
    } else if ((action === "delete" || action === "erase") && (await confirmMailbox(mailbox, action === "erase"))) {
      if (action === "erase") await client.mailbox.erase({ id: mailbox.id });
      else {
        await client.mailbox.delete({ id: mailbox.id });
        const ui = useUI.getState();
        if (ui.source.kind === "mailbox" && ui.source.mailboxId === mailbox.id) ui.setSource({ kind: "allInboxes" });
      }
    }
  };
  const { source, search, searchScope, focus, setSource, setFocus, module, setModule } = useUI();
  const counts = useSidebarCounts(vipGroups(vips));
  const smartCounts = useSmartCounts(smarts);
  const sections = useMemo(
    () => buildSidebar(accounts, mailboxes, { vips, flagNames: settings?.flagNames, counts, smarts, smartCounts }),
    [accounts, mailboxes, vips, settings, counts, smarts, smartCounts],
  );
  const indicators = useMemo(() => {
    const out: Record<number, SyncIndicator> = {};
    for (const s of Object.values(sync)) {
      if (s.phase === "offline" || s.phase === "unauthorized" || s.phase === "failed") {
        out[s.accountId] = { state: "error", message: s.error ?? s.phase };
      } else if (s.phase !== "idle") {
        out[s.accountId] = { state: "syncing" };
      }
    }
    return out;
  }, [sync]);
  const selectedKey = search !== "" && searchScope === "all" ? null : sourceKey(source);
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: focus bubbling up from the sidebar's rows marks the pane focused.
    <div className="flex h-full flex-col bg-sidebar" onFocus={() => setFocus("sidebar")}>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <Sidebar
          sections={sections}
          selectedKey={selectedKey}
          focused={focus === "sidebar"}
          sync={indicators}
          onAdd={(key) => {
            if (key === "smart") useUI.getState().openSmartSheet({ mode: "new" });
            else {
              const account = accounts.find((a) => key === `account:${a.id}`);
              if (account && !account.readOnly) openSheet({ mode: "new", accountId: account.id, name: "" });
            }
          }}
          onContextMenu={(key, x, y) => {
            const source = sourceFromKey(key);
            if (source?.kind === "smart") setMenu({ kind: "smart", id: source.id, x, y });
            else if (source?.kind === "mailbox") setMenu({ kind: "mailbox", id: source.mailboxId, x, y });
          }}
          onSelect={(key) => {
            const s = sourceFromKey(key, vips);
            if (s) setSource(s);
          }}
        />
      </div>
      <div className="max-h-[40%] shrink-0 overflow-y-auto">
        <OutboxSection />
        <SendLaterSection />
      </div>
      {menu?.kind === "smart" && (
        <ContextMenu
          key={menu.id}
          x={menu.x}
          y={menu.y}
          items={[
            { kind: "item", id: "edit", label: "Edit Smart Mailbox…" },
            { kind: "item", id: "delete", label: "Delete Smart Mailbox" },
          ]}
          onClose={() => setMenu(null)}
          onSelect={(action) => {
            if (action === "edit") useUI.getState().openSmartSheet({ mode: "edit", id: menu.id });
            else
              void client.smart
                .delete({ id: menu.id })
                .then(() => {
                  const ui = useUI.getState();
                  if (ui.source.kind === "smart" && ui.source.id === menu.id) ui.setSource({ kind: "allInboxes" });
                })
                .catch((err: unknown) => console.warn("delete smart mailbox", err));
          }}
        />
      )}
      {menu && menuMailbox && (
        <ContextMenu
          key={`mailbox:${menu.id}`}
          x={menu.x}
          y={menu.y}
          items={mailboxMenu(
            menuMailbox,
            accounts.find((a) => a.id === menuMailbox.accountId)?.readOnly ?? true,
            settings?.favorites?.includes(menuMailbox.id) ?? false,
          )}
          onClose={() => setMenu(null)}
          onSelect={(action) => {
            void mailboxAction(menuMailbox, action).catch((err: unknown) => console.warn("mailbox action", err));
          }}
        />
      )}
      {sheet && (
        <MailboxSheet
          mode={sheet.mode}
          name={sheet.name}
          parentId={sheet.parentId}
          locations={mailboxes
            .filter((m) => m.accountId === sheet.accountId)
            .sort((a, b) => a.path.localeCompare(b.path))}
          busy={busy}
          error={error}
          onSave={(draft) => {
            void saveMailbox(draft);
          }}
          onCancel={() => setSheet(null)}
        />
      )}
      <ModuleBar modules={["mail", "calendar", "people", "tasks"]} current={module} onSelect={setModule} />
    </div>
  );
}
