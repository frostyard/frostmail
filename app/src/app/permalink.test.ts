// Mail's permanent route in lodge: /message/<Message-ID> opens the message
// maild resolves, the route it opens replaces the permalink's, and the
// companion learns the permanent form (docs/specs/lodge-app.md).
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useMail, useUI } from "../data/stores";
import { useFrameWindow } from "../lodge/frameWindow";
import { type Lodge, setLodge } from "../lodge/lodge";
import { wireShell } from "../lodge/shell";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { usePermalink, watchPermalinks } from "./permalink";
import { openLink } from "./ReaderContainer";

type Handler = (m: never) => void;

function fakeLodge() {
  const routes: { path: string; replace: boolean }[] = [];
  const handlers: Record<string, Handler[]> = {};
  const open = vi.fn();
  const l = {
    theme: "light",
    route: (path: string, _title: string, options?: { replace?: boolean }) =>
      routes.push({ path, replace: options?.replace === true }),
    open,
    on: ((type: string, fn: Handler) => {
      handlers[type] = [...(handlers[type] ?? []), fn];
      return () => {};
    }) as Lodge["on"],
  } as unknown as Lodge;
  const emit = (type: string, m: unknown) => {
    for (const h of handlers[type] ?? []) h(m as never);
  };
  return { l, routes, emit, open };
}

function client() {
  const data = mockData({ inbox: 3, now: new Date("2026-10-08T12:00:00Z") });
  return { data, client: new Client(new MockTransport(data)) };
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
  useFrameWindow.setState({ open: null });
  usePermalink.setState({ messageId: null });
});

afterEach(() => {
  setLodge(null);
  vi.restoreAllMocks();
});

describe("a permanent route", () => {
  it("opens the message maild resolves, replacing the permalink's entry", async () => {
    const { data, client: c } = client();
    const [first] = data.messages;
    if (!first) throw new Error("no messages");
    useMail.setState({ mailboxes: data.mailboxes.map((mb) => ({ ...mb, total: 0, unread: 0 })) });
    const { l, routes } = fakeLodge();
    const route = `/message/${encodeURIComponent(`mock-${first.summary.id}@mock.test`)}`;
    wireShell(l, route);
    expect(routes).toEqual([{ path: route, replace: true }]);
    const stop = watchPermalinks(c);
    await vi.waitFor(() => expect(useUI.getState().selected).toEqual([first.summary.id]));
    expect(usePermalink.getState().messageId).toBeNull();
    const inbox = data.mailboxes.find((mb) => mb.role === "inbox" && first.summary.mailboxIds.includes(mb.id));
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: inbox?.id });
    expect(routes.slice(1).every((r) => r.replace)).toBe(true);
    expect(routes.at(-1)?.path).toBe(`/mailbox/${inbox?.id}/message/${first.summary.id}`);
    stop();
  });

  it("leaves the window as it is when no message has the Message-ID", async () => {
    const { client: c } = client();
    const { l, emit } = fakeLodge();
    wireShell(l, "/mailbox/1");
    const stop = watchPermalinks(c);
    emit("navigate", { route: "/message/nobody%40x.test" });
    await vi.waitFor(() => expect(usePermalink.getState().messageId).toBeNull());
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: 1 });
    expect(useUI.getState().selected).toEqual([]);
    stop();
  });
});

describe("links in mail's own UI", () => {
  it("open through the shell in lodge, which opens a canonical lodge URL in place", () => {
    const { l, open } = fakeLodge();
    setLodge(l);
    openLink("https://lodge.example.ts.net/notes/1/Plan.md");
    expect(open).toHaveBeenCalledWith("https://lodge.example.ts.net/notes/1/Plan.md");
  });
});
