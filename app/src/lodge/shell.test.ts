import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { NARROW } from "../app/narrow";
import { useUI } from "../data/stores";
import { useFrameWindow } from "./frameWindow";
import type { Lodge } from "./lodge";
import { backToList, wireShell } from "./shell";

type Handler = (m: never) => void;

function fakeLodge() {
  const routes: { path: string; replace: boolean }[] = [];
  const handlers: Record<string, Handler[]> = {};
  const l: Lodge = {
    app: "mail",
    contract: 1,
    theme: "dark",
    shortcut: "Ctrl+Backquote",
    role: "app",
    companion: null,
    contentOrigin: "https://content.test",
    visible: true,
    history: {
      location: { pathname: "/", search: "", hash: "" },
      push: () => {},
      replace: () => {},
      listen: () => () => {},
    },
    apiURL: () => "wss://lodge.test/app/mail/api",
    contentURL: (p) => `https://content.test/mail/${p}`,
    route: (path, _title, options) => routes.push({ path, replace: options?.replace === true }),
    context: () => {},
    ask: () => {},
    open: () => {},
    openSwitcher: () => {},
    storageKey: (k) => `mail:${k}`,
    close: () => {},
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
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  beforeEach(() => {
    useUI.setState({ source: { kind: "allInboxes" }, selected: [] });
    useFrameWindow.setState({ open: null });
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

  it("pushes a window, pushes the mail back when it closes, and follows Back out of it", () => {
    const { l, routes, emit } = fakeLodge();
    wireShell(l, "/mailbox/2/message/5");
    useFrameWindow.setState({ open: { kind: "compose", draftId: 12, fresh: true } });
    useFrameWindow.setState({ open: null });
    expect(routes.slice(1)).toEqual([
      { path: "/compose/12?fresh", replace: false },
      { path: "/mailbox/2/message/5", replace: false },
    ]);
    emit("navigate", { route: "/settings" });
    expect(useFrameWindow.getState().open).toEqual({ kind: "settings" });
    emit("navigate", { route: "/mailbox/2/message/5" });
    expect(useFrameWindow.getState().open).toBeNull();
    expect(useUI.getState().selected).toEqual([5]);
    expect(routes).toHaveLength(3);
  });

  it("in a narrow window, pushes a message opened from its list, so Back is history's", () => {
    vi.stubGlobal("matchMedia", (media: string) => ({ matches: media === NARROW, media }));
    const back = vi.spyOn(window.history, "back").mockImplementation(() => {});
    const { l, routes, emit } = fakeLodge();
    wireShell(l, "/mailbox/2");
    useUI.getState().select([5], 5);
    useUI.getState().select([6], 6);
    expect(routes.slice(1)).toEqual([
      { path: "/mailbox/2/message/5", replace: false },
      { path: "/mailbox/2/message/6", replace: true },
    ]);
    expect(backToList()).toBe(true);
    expect(back).toHaveBeenCalledOnce();
    emit("navigate", { route: "/mailbox/2" });
    expect(useUI.getState().selected).toEqual([]);
    expect(backToList()).toBe(false);
  });

  it("in a narrow window, does not go back by history from a message it was opened at", () => {
    vi.stubGlobal("matchMedia", (media: string) => ({ matches: media === NARROW, media }));
    const back = vi.spyOn(window.history, "back").mockImplementation(() => {});
    const { l, routes, emit } = fakeLodge();
    wireShell(l, "/mailbox/2/message/5");
    expect(backToList()).toBe(false);
    useUI.getState().select([], null);
    expect(routes.slice(1)).toEqual([{ path: "/mailbox/2", replace: true }]);
    useUI.getState().select([6], 6);
    emit("navigate", { route: "/mailbox/2/message/6" });
    expect(backToList()).toBe(false);
    expect(back).not.toHaveBeenCalled();
  });

  it("opens the window a frame's route names", () => {
    const { l, routes } = fakeLodge();
    wireShell(l, "/compose/7");
    expect(useFrameWindow.getState().open).toEqual({ kind: "compose", draftId: 7, fresh: false });
    expect(routes).toEqual([{ path: "/compose/7", replace: true }]);
  });
});
