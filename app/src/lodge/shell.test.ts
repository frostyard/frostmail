import { beforeEach, describe, expect, it } from "vitest";

import { useUI } from "../data/stores";
import type { Lodge } from "./lodge";
import { wireShell } from "./shell";
import { frameRoute } from "./start";

type Handler = (m: never) => void;

function fakeLodge() {
  const routes: { path: string; replace: boolean }[] = [];
  const handlers: Record<string, Handler[]> = {};
  const l: Lodge = {
    theme: "dark",
    apiURL: () => "wss://lodge.test/app/mail/api",
    contentURL: (p) => `https://content.test/mail/${p}`,
    route: (path, _title, replace) => routes.push({ path, replace: replace === true }),
    context: () => {},
    open: () => {},
    on: ((type: string, fn: Handler) => {
      handlers[type] = [...(handlers[type] ?? []), fn];
      return () => {};
    }) as Lodge["on"],
  };
  const emit = (type: string, m: unknown) => {
    for (const h of handlers[type] ?? []) h(m as never);
  };
  return { l, routes, emit };
}

describe("wireShell", () => {
  beforeEach(() => {
    useUI.setState({ source: { kind: "allInboxes" }, selected: [] });
    delete document.documentElement.dataset.theme;
  });

  it("opens the frame's route and reports it without a history entry", () => {
    const { l, routes } = fakeLodge();
    wireShell(l, "/mailbox/4/message/9");
    expect(useUI.getState().source).toEqual({ kind: "mailbox", mailboxId: 4 });
    expect(useUI.getState().selected).toEqual([9]);
    expect(routes).toEqual([{ path: "/mailbox/4/message/9", replace: true }]);
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("pushes a new source and replaces a new message", () => {
    const { l, routes } = fakeLodge();
    wireShell(l, "/");
    useUI.getState().setSource({ kind: "mailbox", mailboxId: 2 });
    useUI.getState().select([5], 5);
    useUI.getState().select([6], 6);
    useUI.getState().select([6, 7], 7);
    expect(routes.slice(1)).toEqual([
      { path: "/mailbox/2", replace: false },
      { path: "/mailbox/2/message/5", replace: true },
      { path: "/mailbox/2/message/6", replace: true },
      { path: "/mailbox/2", replace: true },
    ]);
  });

  it("obeys navigate without reporting it back", () => {
    const { l, routes, emit } = fakeLodge();
    wireShell(l, "/");
    emit("navigate", { route: "/smart/3/message/8" });
    expect(useUI.getState().source).toEqual({ kind: "smart", id: 3 });
    expect(useUI.getState().selected).toEqual([8]);
    expect(routes).toHaveLength(1);
    emit("theme", { scheme: "light" });
    expect(document.documentElement.dataset.theme).toBe("light");
  });
});

describe("frameRoute", () => {
  it("is the path under the build's base", () => {
    expect(frameRoute("/app/mail/", "/app/mail/")).toBe("/");
    expect(frameRoute("/app/mail", "/app/mail/")).toBe("/");
    expect(frameRoute("/app/mail/mailbox/3", "/app/mail/")).toBe("/mailbox/3");
    expect(frameRoute("/app/mailx/1", "/app/mail/")).toBe("/");
    expect(frameRoute("/elsewhere", "/app/mail/")).toBe("/");
  });
});
