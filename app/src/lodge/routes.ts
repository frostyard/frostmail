// Mail's routes in lodge (docs/specs/lodge-app.md, The web build): the
// sidebar's source, then the message shown when exactly one is selected.
// The canonical URL is /mail followed by the route.

import type { Source } from "../data/stores";
import type { MailboxRole } from "../rpc/gen/api";

/** Where is what a route shows. */
export interface Where {
  source: Source;
  message?: number;
}

const ROLES: readonly MailboxRole[] = ["inbox", "drafts", "sent", "archive", "junk", "trash", "all", "flagged"];

function sourcePath(s: Source): string {
  switch (s.kind) {
    case "allInboxes":
      return "";
    case "mailbox":
      return `/mailbox/${s.mailboxId}`;
    case "smart":
      return `/smart/${s.id}`;
    case "role":
      return `/role/${s.role}`;
    case "flagged":
      return "/flagged";
    case "flagColor":
      return `/flag/${s.color}`;
    case "vips":
    case "vip":
      return "/vips";
  }
}

/** routeOf is the route of a source and its selection. */
export function routeOf(source: Source, selected: readonly number[]): string {
  const base = sourcePath(source);
  const message = selected.length === 1 ? `/message/${selected[0]}` : "";
  return base + message || "/";
}

const id = (s: string | undefined): number | undefined => (s && /^[1-9][0-9]{0,15}$/.test(s) ? Number(s) : undefined);

/** parseRoute is what a route shows; an unknown route shows All Inboxes. */
export function parseRoute(route: string): Where {
  const path = route.split(/[?#]/, 1)[0] ?? "";
  const segs = path.split("/").filter((s) => s !== "");
  let message: number | undefined;
  const at = segs.lastIndexOf("message");
  if (at >= 0 && at === segs.length - 2) {
    message = id(segs[at + 1]);
    segs.splice(at, 2);
  }
  const [kind, arg] = segs;
  let source: Source = { kind: "allInboxes" };
  if (segs.length === 2 && kind === "mailbox" && id(arg)) source = { kind: "mailbox", mailboxId: id(arg) as number };
  else if (segs.length === 2 && kind === "smart" && id(arg)) source = { kind: "smart", id: id(arg) as number };
  else if (segs.length === 2 && kind === "role" && ROLES.includes(arg as MailboxRole))
    source = { kind: "role", role: arg as MailboxRole };
  else if (segs.length === 2 && kind === "flag" && /^[1-7]$/.test(arg ?? ""))
    source = { kind: "flagColor", color: Number(arg) };
  else if (segs.length === 1 && kind === "flagged") source = { kind: "flagged" };
  else if (segs.length === 1 && kind === "vips") source = { kind: "vips" };
  return message === undefined ? { source } : { source, message };
}
