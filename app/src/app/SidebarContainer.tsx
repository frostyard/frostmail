// The sidebar, fed by the mail store (docs/specs/ui.md, Sidebar).
import { useMemo } from "react";

import { sourceFromKey, sourceKey, useMail, useUI } from "../data/stores";
import { ModuleBar } from "../features/sidebar/ModuleBar";
import { Sidebar, type SyncIndicator } from "../features/sidebar/Sidebar";
import { buildSidebar } from "../lib/mailboxTree";
import { OutboxSection } from "./OutboxContainer";

/** SidebarContainer connects Sidebar to the stores. */
export function SidebarContainer() {
  const { accounts, mailboxes, sync } = useMail();
  const { source, search, searchScope, focus, setSource, setFocus, module, setModule } = useUI();
  const sections = useMemo(() => buildSidebar(accounts, mailboxes), [accounts, mailboxes]);
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
          onSelect={(key) => {
            const s = sourceFromKey(key);
            if (s) setSource(s);
          }}
        />
      </div>
      <div className="max-h-[40%] shrink-0 overflow-y-auto">
        <OutboxSection />
      </div>
      <ModuleBar modules={["mail", "calendar", "people"]} current={module} onSelect={setModule} />
    </div>
  );
}
