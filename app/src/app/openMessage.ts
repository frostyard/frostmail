// Opening a message from outside the window: a notification runs
// frostmail --open-message <id> (docs/design/desktop.md, One app instance).
// The first launch asks for its request once; later launches arrive as the
// open-message event.
import { CodeNotConnected } from "@frostyard/lodge-client/rpc";
import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

import { useMail, useUI } from "../data/stores";
import type { Client } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";

type Source = ReturnType<typeof useUI.getState>["source"];
interface Target {
  source: Source;
  id: number;
}

/** lookup finds what to show for a message: its account's inbox (or its first mailbox), or null. */
async function lookup(client: Client, id: number): Promise<Target | null> {
  const [s] = await client.message.summaries({ ids: [id] });
  if (!s) return null;
  const inbox = useMail.getState().mailboxes.find((mb) => mb.accountId === s.accountId && mb.role === "inbox");
  const mailboxId = inbox && s.mailboxIds.includes(inbox.id) ? inbox.id : s.mailboxIds[0];
  const source: Source = mailboxId === undefined ? { kind: "allInboxes" } : { kind: "mailbox", mailboxId };
  return { source, id };
}

function apply(t: Target): void {
  const ui = useUI.getState();
  ui.setSource(t.source);
  ui.select([t.id], t.id);
}

/** revealMessage shows a message in its account's inbox (or its first mailbox) and selects it. */
export async function revealMessage(client: Client, id: number): Promise<void> {
  const t = await lookup(client, id);
  if (t) apply(t);
}

// One request waits here until a connected window can show it. The window
// loses its client whenever maild stops (an update replaces it), and the
// notification click that follows can arrive before the next client exists
// (issue #149). So the listeners live as long as the webview, not as long as
// one client. Each request is an object: a late answer for an older request
// finds that pending is another object, and changes nothing.
interface Request {
  id: number;
}
let pending: Request | null = null;
let current: Client | null = null;
let inFlight: { client: Client; req: Request } | null = null;
let listening = false;

/** lostConnection tells a lost connection to maild from maild's own refusal. */
function lostConnection(err: unknown): boolean {
  if (!(err instanceof RPCError)) return true; // not a maild answer: keep the request
  return err.code === CodeNotConnected; // JsonRpcSession's error for a closed connection
}

function deliver(): void {
  const client = current;
  const req = pending;
  if (client === null || req === null) return;
  if (inFlight?.client === client && inFlight.req === req) return;
  const mine = { client, req };
  inFlight = mine;
  lookup(client, req.id)
    .then((t) => {
      if (pending !== req) return; // replaced, or delivered by another client
      if (t) apply(t);
      pending = null;
    })
    .catch((err: unknown) => {
      if (lostConnection(err)) return; // the next client delivers it
      console.warn("open message", err);
      if (pending === req) pending = null;
    })
    .finally(() => {
      if (inFlight === mine) inFlight = null;
    });
}

function request(id: number): void {
  pending = { id };
  deliver();
}

function startListening(): void {
  if (listening) return;
  listening = true;
  void invoke<number | null>("startup_message").then((id) => {
    if (id !== null) request(id);
  });
  void listen<number>("open-message", (e) => request(e.payload));
}

/** watchOpenRequests shows the launch's message and every later one in this client's window; it returns the cleanup. */
export function watchOpenRequests(client: Client): () => void {
  if (!isTauri()) return () => {};
  startListening();
  current = client;
  deliver();
  return () => {
    if (current === client) current = null;
  };
}
