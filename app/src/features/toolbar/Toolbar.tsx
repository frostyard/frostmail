// The toolbar and title bar (docs/specs/ui.md, Layout: Toolbar). Task
// T-0032 implements Toolbar; the stub shows the title only.
import type { ReactNode } from "react";

/** ToolbarCommand is what a toolbar control asks for. */
export type ToolbarCommand =
  | { kind: "toggleSidebar" }
  | { kind: "getMail" }
  | { kind: "delete" }
  | { kind: "archive" }
  /** color 0 clears the flag. */
  | { kind: "flag"; color: number }
  | { kind: "toggleRead" }
  | { kind: "move"; mailboxId: number }
  | { kind: "minimize" }
  | { kind: "toggleMaximize" }
  | { kind: "close" };

/** MoveTarget is one entry of the Move menu. */
export interface MoveTarget {
  mailboxId: number;
  label: string;
  depth: number;
}

/** ToolbarSelection describes the selected messages. */
export interface ToolbarSelection {
  count: number;
  /** Every selected message is seen. */
  seen: boolean;
  /** The first selected message's flag color; 0 when not flagged. */
  flagColor: number;
}

/** ToolbarProps are the toolbar's inputs. */
export interface ToolbarProps {
  /** The sidebar's width, 0 while it is hidden. */
  sidebarWidth: number;
  listWidth: number;
  title: string;
  subtitle: string;
  syncing: boolean;
  selection: ToolbarSelection;
  canArchive: boolean;
  moveTargets: MoveTarget[];
  maximized: boolean;
  /** The search field, placed before the window controls. */
  search: ReactNode;
  onCommand: (cmd: ToolbarCommand) => void;
}

/** Toolbar is the window's top strip, aligned with the panes. */
export function Toolbar(props: ToolbarProps) {
  return (
    <div role="toolbar" aria-label="Toolbar" className="flex h-[52px] items-center gap-2 px-3">
      <span>{props.title}</span>
      {props.search}
    </div>
  );
}
