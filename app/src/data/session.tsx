// The connection to maild and the stores it feeds (docs/design/app.md, Data).
import { createContext, type ReactNode, useContext, useEffect, useState } from "react";

import { Client, type Event, PROTOCOL } from "../rpc/gen/api";
import type { Transport } from "../rpc/transport";
import { useMail, useUI } from "./stores";

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
 *
 * When the new connection reaches another maild, the window forgets the old
 * maild's ids (useUI.forgetMaild) before the children mount again. A window
 * whose contents belong to the old maild (a compose window's draft) says so
 * with onOtherMaild: Session then renders `left` instead and never mounts its
 * children on the new maild.
 */
export interface SessionProps {
  connect: Connect;
  children: ReactNode;
  fallback: ReactNode;
  /**
   * onOtherMaild runs when a new connection reaches another maild than the
   * last one, after the window forgot that maild's ids. When it returns
   * false, Session renders `left` in place of the children and never mounts
   * them on the new maild. Keep the callback stable: a new one reconnects.
   */
  onOtherMaild?: () => boolean;
  /** left renders in place of the children after onOtherMaild returned false. */
  left?: ReactNode;
}

export function Session(props: SessionProps) {
  const { connect, onOtherMaild } = props;
  const [client, setClient] = useState<Client | null>(null);
  // leftMaild is set when this window must not take the new maild, so it
  // shows `left` instead of its children, even for one render.
  const [leftMaild, setLeftMaild] = useState(false);

  useEffect(() => {
    let stopped = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let cleanup: (() => void) | undefined;
    // The maild the last connection reached, by its transport's key
    // (rpc/transport.ts): a different one means the old ids are not this
    // maild's, so the window forgets them before its children mount again.
    let lastMaild: string | undefined;
    // gone is set when onOtherMaild refused the new maild: the window keeps
    // no connection of its own then, and never reconnects.
    let gone = false;

    const attempt = async () => {
      useMail.setState({ connection: { state: "connecting" } });
      try {
        const transport = await connect();
        if (stopped || gone) return;
        const c = new Client(transport);
        await c.rpc.hello({ protocol: PROTOCOL, client: "frostmail-app" });
        const maild = transport.maild;
        if (lastMaild !== undefined && maild !== undefined && maild !== lastMaild) {
          useUI.getState().forgetMaild();
          if (onOtherMaild?.() === false) {
            gone = true;
            setLeftMaild(true);
            return;
          }
        }
        lastMaild = maild;
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
        if (stopped || gone) return;
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
  }, [connect, onOtherMaild]);

  if (leftMaild) return <>{props.left ?? props.fallback}</>;
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
  loadBlocked(c);
  loadSettings(c);
  loadSmarts(c);
  loadRules(c);
}

function loadRules(c: Client): void {
  void c.rule
    .list()
    .then((rules) => useMail.setState({ rules }))
    .catch(() => {});
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

function loadBlocked(c: Client): void {
  void c.block
    .list()
    .then((blocked) => useMail.setState({ blocked }))
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
      case "block.changed":
        loadBlocked(c);
        break;
      case "settings.changed":
        loadSettings(c);
        break;
      case "smart.changed":
        loadSmarts(c);
        break;
      case "rule.changed":
        loadRules(c);
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
