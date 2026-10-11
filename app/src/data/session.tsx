// The connection to maild and the stores it feeds (docs/design/app.md, Data).
import { createContext, type ReactNode, useContext, useEffect, useState } from "react";

import { Client, type Event, PROTOCOL } from "../rpc/gen/api";
import type { Transport } from "../rpc/transport";
import { useMail } from "./stores";

const ClientContext = createContext<Client | null>(null);

/** useClient returns the connected client; it must be used under Session. */
export function useClient(): Client {
  const c = useContext(ClientContext);
  if (!c) throw new Error("useClient outside <Session>");
  return c;
}

/** Connect opens a transport to maild. */
export type Connect = () => Promise<Transport>;

/** RETRY_MS is how long Session waits before reconnecting. */
export const RETRY_MS = 1000;

/**
 * Session connects with connect, completes rpc.hello, subscribes to events,
 * loads the mail store, and renders children with the client. When the
 * connection ends it reconnects every RETRY_MS, rendering fallback meanwhile.
 */
export function Session(props: { connect: Connect; children: ReactNode; fallback: ReactNode }) {
  const { connect } = props;
  const [client, setClient] = useState<Client | null>(null);

  useEffect(() => {
    let stopped = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let cleanup: (() => void) | undefined;

    const attempt = async () => {
      useMail.setState({ connection: { state: "connecting" } });
      try {
        const transport = await connect();
        if (stopped) return;
        const c = new Client(transport);
        await c.rpc.hello({ protocol: PROTOCOL, client: "frostmail-app" });
        cleanup = wire(c);
        await c.events.subscribe({});
        await reload(c);
        if (stopped) return;
        useMail.setState({ connection: { state: "ready" } });
        setClient(c);
        transport.onClose((reason) => {
          if (stopped) return;
          cleanup?.();
          setClient(null);
          useMail.setState({ connection: { state: "lost", reason } });
          retry = setTimeout(attempt, RETRY_MS);
        });
      } catch (err) {
        if (stopped) return;
        useMail.setState({ connection: { state: "lost", reason: err instanceof Error ? err.message : String(err) } });
        retry = setTimeout(attempt, RETRY_MS);
      }
    };
    void attempt();
    return () => {
      stopped = true;
      clearTimeout(retry);
      cleanup?.();
    };
  }, [connect]);

  if (!client) return <>{props.fallback}</>;
  return <ClientContext.Provider value={client}>{props.children}</ClientContext.Provider>;
}

async function reload(c: Client): Promise<void> {
  const [accounts, mailboxes, sync, outbox] = await Promise.all([
    c.account.list(),
    c.mailbox.list(),
    c.sync.status(),
    c.outbox.list({}),
  ]);
  useMail.setState({ accounts, mailboxes, sync: Object.fromEntries(sync.map((s) => [s.accountId, s])), outbox });
  // M5's preferences load on their own, so a maild without them still
  // shows mail.
  loadVips(c);
  loadSettings(c);
  loadSmarts(c);
}

function loadSmarts(c: Client): void {
  void c.smart
    .list()
    .then((smarts) => useMail.setState({ smarts }))
    .catch(() => {});
}

function loadVips(c: Client): void {
  void c.vip
    .list()
    .then((vips) => useMail.setState({ vips }))
    .catch(() => {});
}

function loadSettings(c: Client): void {
  void c.settings
    .get()
    .then((settings) => useMail.setState({ settings }))
    .catch(() => {});
}

// wire keeps the mail store current from events; it returns the unsubscribe.
function wire(c: Client): () => void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const reloadSoon = () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      void Promise.all([c.account.list(), c.mailbox.list()])
        .then(([accounts, mailboxes]) => useMail.setState({ accounts, mailboxes }))
        .catch(() => {});
    }, 100);
  };
  let outboxTimer: ReturnType<typeof setTimeout> | undefined;
  const reloadOutbox = () => {
    clearTimeout(outboxTimer);
    outboxTimer = setTimeout(() => {
      void c.outbox
        .list({})
        .then((outbox) => useMail.setState({ outbox }))
        .catch(() => {});
    }, 50);
  };
  const off = c.transport.onEvent((e: Event) => {
    switch (e.event) {
      case "mailbox.changed":
      case "account.changed":
        reloadSoon();
        break;
      case "outbox.changed":
        reloadOutbox();
        break;
      case "vip.changed":
        loadVips(c);
        break;
      case "settings.changed":
        loadSettings(c);
        break;
      case "smart.changed":
        loadSmarts(c);
        break;
      case "sync.progress":
        useMail.setState((s) => ({ sync: { ...s.sync, [e.data.status.accountId]: e.data.status } }));
        break;
    }
  });
  return () => {
    clearTimeout(timer);
    clearTimeout(outboxTimer);
    off();
  };
}
