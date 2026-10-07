// The sidebar (docs/specs/ui.md, Sidebar). Task T-0030 implements Sidebar;
// the stub lists row labels only.
import type { SidebarSection } from "../../lib/mailboxTree";

/** SyncIndicator is what the sidebar shows next to an account title. */
export interface SyncIndicator {
  state: "idle" | "syncing" | "error";
  /** The error, shown as a tooltip. */
  message?: string;
}

/** SidebarProps are the sidebar's inputs. */
export interface SidebarProps {
  sections: SidebarSection[];
  selectedKey: string | null;
  /** The sidebar has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  /** Sync state by account ID; missing means idle. */
  sync: Record<number, SyncIndicator>;
  onSelect: (key: string) => void;
}

/** Sidebar shows Favorites and each account's mailboxes. */
export function Sidebar(props: SidebarProps) {
  return (
    <nav aria-label="Mailboxes">
      {props.sections.flatMap((s) =>
        s.items.map((i) => (
          <button key={i.key} type="button" onClick={() => props.onSelect(i.key)}>
            {i.label}
          </button>
        )),
      )}
    </nav>
  );
}
