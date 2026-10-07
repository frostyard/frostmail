// The sidebar's sections and rows, built from accounts and mailboxes
// (docs/specs/ui.md, Sidebar). Favorites comes first, then one section per
// account: role mailboxes at depth 0 in role order, then folders by label,
// nested by the hierarchy delimiter. A folder whose parent mailbox is not in
// the list gets a synthesized, non-selectable parent row.
import type { Account, Mailbox, MailboxRole } from "../rpc/gen/api";

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

const ROLE_ORDER: MailboxRole[] = ["inbox", "drafts", "sent", "junk", "trash", "archive", "all", "flagged"];

const ROLE_LABEL: Record<MailboxRole, string> = {
  none: "",
  inbox: "Inbox",
  drafts: "Drafts",
  sent: "Sent",
  junk: "Junk",
  trash: "Trash",
  archive: "Archive",
  all: "All Mail",
  flagged: "Flagged",
};

const ROLE_ICON: Record<MailboxRole, SidebarIcon> = {
  none: "folder",
  inbox: "inbox",
  drafts: "file",
  sent: "send",
  junk: "shield-alert",
  trash: "trash-2",
  archive: "archive",
  all: "archive",
  flagged: "flag",
};

interface Node {
  item: SidebarItem;
  children: Node[];
}

function lastComponent(path: string, delimiter: string): string {
  if (!delimiter) return path;
  return path.split(delimiter).pop() ?? path;
}

function parentPath(path: string, delimiter: string): string | null {
  if (!delimiter) return null;
  const idx = path.lastIndexOf(delimiter);
  return idx < 0 ? null : path.slice(0, idx);
}

function byLabel(a: Node, b: Node): number {
  return a.item.label.localeCompare(b.item.label, undefined, { sensitivity: "base" });
}

function flatten(nodes: Node[], depth: number, out: SidebarItem[]): void {
  for (const node of nodes) {
    node.item.depth = depth;
    out.push(node.item);
    node.children.sort(byLabel);
    flatten(node.children, depth + 1, out);
  }
}

function accountRows(account: Account, mailboxes: Mailbox[]): SidebarItem[] {
  const own = mailboxes.filter((m) => m.accountId === account.id);
  const byPath = new Map<string, Mailbox>();
  for (const m of own) byPath.set(m.path, m);

  const roleNodes = new Map<string, Node>();
  const roots: Node[] = [];
  for (const role of ROLE_ORDER) {
    for (const m of own.filter((x) => x.role === role)) {
      const node: Node = {
        item: {
          key: `mailbox:${m.id}`,
          label: ROLE_LABEL[role] ?? m.name,
          icon: ROLE_ICON[role],
          depth: 0,
          unread: m.unread,
          selectable: true,
          mailboxId: m.id,
        },
        children: [],
      };
      roleNodes.set(m.path, node);
      roots.push(node);
    }
  }

  const nodes = new Map<string, Node>();
  const topFolders: Node[] = [];
  const folderNode = (path: string, delimiter: string): Node => {
    const existing = nodes.get(path);
    if (existing) return existing;
    const real = byPath.get(path);
    const node: Node = {
      item: {
        key: real ? `mailbox:${real.id}` : `path:${account.id}:${path}`,
        label: real ? real.name : lastComponent(path, delimiter),
        icon: "folder",
        depth: 0,
        unread: real?.unread ?? 0,
        selectable: real !== undefined,
        ...(real ? { mailboxId: real.id } : {}),
      },
      children: [],
    };
    nodes.set(path, node);
    const parent = parentPath(path, delimiter);
    if (parent === null) topFolders.push(node);
    else if (roleNodes.has(parent)) roleNodes.get(parent)?.children.push(node);
    else folderNode(parent, delimiter).children.push(node);
    return node;
  };
  for (const m of own) {
    if (m.role === "none") folderNode(m.path, m.delimiter);
  }

  topFolders.sort(byLabel);
  const out: SidebarItem[] = [];
  flatten(roots, 0, out);
  flatten(topFolders, 0, out);
  return out;
}

/** buildSidebar builds the sidebar from accounts and their mailboxes. */
export function buildSidebar(accounts: Account[], mailboxes: Mailbox[]): SidebarSection[] {
  const allInboxes = mailboxes.filter((m) => m.role === "inbox").reduce((sum, m) => sum + m.unread, 0);
  const favorites: SidebarSection = {
    key: "favorites",
    title: "Favorites",
    items: [
      { key: "all-inboxes", label: "All Inboxes", icon: "inbox", depth: 0, unread: allInboxes, selectable: true },
      { key: "flagged", label: "Flagged", icon: "flag", depth: 0, unread: 0, selectable: true },
    ],
  };
  const sections: SidebarSection[] = accounts.map((a) => ({
    key: `account:${a.id}`,
    title: a.email,
    accountId: a.id,
    items: accountRows(a, mailboxes),
  }));
  return [favorites, ...sections];
}
