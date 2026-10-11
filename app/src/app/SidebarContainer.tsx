// The sidebar, fed by the mail store (docs/specs/ui.md, Sidebar).
import { useMemo, useState } from "react";

import { useClient } from "../data/session";
import { sourceFromKey, sourceKey, useMail, useUI } from "../data/stores";
import { ContextMenu } from "../features/menu/ContextMenu";
import { ModuleBar } from "../features/sidebar/ModuleBar";
import { Sidebar, type SyncIndicator } from "../features/sidebar/Sidebar";
import { buildSidebar, vipGroups } from "../lib/mailboxTree";
import { OutboxSection, SendLaterSection } from "./OutboxContainer";
import { useSidebarCounts } from "./useSidebarCounts";
import { useSmartCounts } from "./useSmartCounts";

/** SidebarContainer connects Sidebar to the stores. */
export function SidebarContainer() {
  const client = useClient();
  const [menu, setMenu] = useState<{ id: number; x: number; y: number } | null>(null);
  const { accounts, mailboxes, sync, vips, settings, smarts } = useMail();
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
          }}
          onContextMenu={(key, x, y) => {
            const source = sourceFromKey(key);
            if (source?.kind === "smart") setMenu({ id: source.id, x, y });
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
      {menu && (
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
      <ModuleBar modules={["mail", "calendar", "people", "tasks"]} current={module} onSelect={setModule} />
    </div>
  );
}
