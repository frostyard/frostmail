// The sidebar's sections and rows, built from accounts and mailboxes
// (docs/specs/ui.md, Sidebar). Task T-0027 implements buildSidebar; the
// stub returns no sections.
import type { Account, Mailbox } from "../rpc/gen/api";

/** SidebarIcon names a Lucide icon for a sidebar row. */
export type SidebarIcon = "inbox" | "file" | "send" | "shield-alert" | "trash-2" | "archive" | "flag" | "folder";

/** SidebarItem is one row of a sidebar section. */
export interface SidebarItem {
  /** "all-inboxes", "flagged", "mailbox:<id>", or "path:<accountId>:<path>" for a parent not in the list. */
  key: string;
  label: string;
  icon: SidebarIcon;
  /** 0 for top-level rows; one more per ancestor. */
  depth: number;
  unread: number;
  /** False for parents that are not in the mailbox list. */
  selectable: boolean;
  /** The mailbox's ID; absent for Favorites rows and synthesized parents. */
  mailboxId?: number;
}

/** SidebarSection is "Favorites" or one account. */
export interface SidebarSection {
  /** "favorites" or "account:<id>". */
  key: string;
  title: string;
  /** Absent for Favorites. */
  accountId?: number;
  /** Rows in display order, parents before their children. */
  items: SidebarItem[];
}

/** buildSidebar builds the sidebar from accounts and their mailboxes. */
export function buildSidebar(_accounts: Account[], _mailboxes: Mailbox[]): SidebarSection[] {
  return [];
}
