// A compose window and an event window belong to the maild they opened on
// (docs/specs/settings-ui.md, Server pane; frostmail#186). After a change of
// maild their window closes, and their draft or event is never loaded on the
// maild the app switched to, where the id names other data.
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { App } from "../app/App";
import type { CalendarEvent, Draft } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import type { Transport } from "../rpc/transport";
import { useUI } from "./stores";

const NOW = new Date("2026-10-07T12:00:00Z");
const LODGE = "https://lodge.example.ts.net";

// The window's shell: the app runs in Tauri, whose window it closes again
// when it left its maild.
const env = vi.hoisted(() => ({ close: vi.fn(async () => {}) }));
vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  isTauri: () => true,
}));
vi.mock("@tauri-apps/api/window", () => ({
  getCurrentWindow: () => ({
    close: env.close,
    isMaximized: async () => false,
    minimize: async () => {},
    onResized: async () => () => {},
    setTitle: async () => {},
    show: async () => {},
    toggleMaximize: async () => {},
  }),
}));
vi.mock("@tauri-apps/api/webview", () => ({
  getCurrentWebview: () => ({ onDragDropEvent: async () => () => {} }),
}));

beforeEach(() => {
  localStorage.clear();
  window.location.hash = "";
  useUI.setState(useUI.getInitialState());
  env.close.mockClear();
});

afterEach(() => {
  window.__frostmailCompose = undefined;
  window.__frostmailComposeFresh = undefined;
  window.__frostmailEvent = undefined;
  window.__frostmailMaild = undefined;
});

/** data is one maild's data; two calls make two mailds with the same ids. */
function data() {
  return mockData({ inbox: 2, now: NOW });
}

/** connect answers each transport in turn, then the last one again. */
function connect(...transports: MockTransport[]): () => Promise<Transport> {
  let next = 0;
  return () => {
    const transport = transports[Math.min(next, transports.length - 1)];
    next += 1;
    if (!transport) throw new Error("connect with no transport");
    return Promise.resolve(transport);
  };
}

/** failing is connect that fails its first failures times, then answers transport. */
function failing(failures: number, transport: MockTransport): () => Promise<Transport> {
  let attempts = 0;
  return () => {
    attempts += 1;
    if (attempts <= failures) return Promise.reject(new Error("the maild cannot be reached"));
    return Promise.resolve(transport);
  };
}

/** callsOf are the params of one method a maild saw. */
function callsOf(mock: MockTransport, method: string): unknown[] {
  return mock.calls.filter((c) => c.method === method).map((c) => c.params);
}

/** LostHandshake is a maild whose first rpc.hello never answers: the window's
 *  connection ends during the handshake, as a change of maild makes it. */
class LostHandshake extends MockTransport {
  override call<T>(method: string, params: unknown): Promise<T> {
    if (method !== "rpc.hello") return super.call<T>(method, params);
    this.calls.push({ method, params });
    this.close("switch");
    return Promise.reject(new Error("the connection ended during rpc.hello"));
  }
}

describe("a compose window after a change of maild", () => {
  it("closes instead of opening its draft on the other maild", async () => {
    const a = new MockTransport(data());
    const b = new MockTransport(data());
    a.maild = "local";
    b.maild = `lodge:${LODGE}`;
    // The same draft id on both mailds: without the fix, the window would
    // open B's draft, and closing it untouched would delete it.
    const draft = await a.call<Draft>("draft.create", { kind: "new" });
    expect((await b.call<Draft>("draft.create", { kind: "new" })).id).toBe(draft.id);
    window.__frostmailCompose = draft.id;
    window.__frostmailComposeFresh = true;
    render(<App connect={connect(a, b)} />);

    // The window shows its draft on A.
    await screen.findByRole("toolbar", { name: "Compose" });
    expect(callsOf(a, "draft.get")).toEqual([{ id: draft.id }]);

    a.close("switch");
    await screen.findByText("This draft is on the maild you left.");
    await waitFor(() => expect(env.close).toHaveBeenCalled());
    expect(callsOf(b, "draft.get")).toEqual([]);
    expect(callsOf(b, "draft.delete")).toEqual([]);
    expect(screen.queryByRole("toolbar", { name: "Compose" })).toBeNull();
  });

  it("closes when its first handshake ends on another maild", async () => {
    // A closes during rpc.hello, so no handshake ever answered: the key of
    // A must still count, or the retry on B looks like the same maild.
    const a = new LostHandshake(data());
    const b = new MockTransport(data());
    a.maild = "local";
    b.maild = `lodge:${LODGE}`;
    const draft = await a.call<Draft>("draft.create", { kind: "new" });
    expect((await b.call<Draft>("draft.create", { kind: "new" })).id).toBe(draft.id);
    window.__frostmailCompose = draft.id;
    window.__frostmailComposeFresh = true;
    render(<App connect={connect(a, b)} />);

    await screen.findByText("This draft is on the maild you left.");
    await waitFor(() => expect(env.close).toHaveBeenCalled());
    expect(callsOf(a, "rpc.hello")).toHaveLength(1);
    expect(callsOf(a, "draft.get")).toEqual([]);
    expect(callsOf(b, "draft.get")).toEqual([]);
    expect(callsOf(b, "draft.delete")).toEqual([]);
    expect(screen.queryByRole("toolbar", { name: "Compose" })).toBeNull();
  });

  it("keeps the window when it reconnects to the same maild", async () => {
    const a = new MockTransport(data());
    const c = new MockTransport(data());
    a.maild = "local";
    c.maild = "local";
    const draft = await a.call<Draft>("draft.create", { kind: "new" });
    await c.call<Draft>("draft.create", { kind: "new" });
    window.__frostmailCompose = draft.id;
    window.__frostmailComposeFresh = true;
    render(<App connect={connect(a, c)} />);
    await screen.findByRole("toolbar", { name: "Compose" });

    a.close("network drop");
    // C is the same maild, so the window opens the same draft again.
    await waitFor(() => expect(callsOf(c, "draft.get")).toEqual([{ id: draft.id }]));
    expect(screen.getByRole("toolbar", { name: "Compose" })).toBeTruthy();
    expect(env.close).not.toHaveBeenCalled();
  });
});

describe("a window that could not reach its own maild", () => {
  it("closes a compose window before its first handshake with the other maild", async () => {
    // A made the draft and opened this window, but the window cannot reach A;
    // the first connection that answers is B, which has a draft of that id.
    const b = new MockTransport(data());
    b.maild = `lodge:${LODGE}`;
    const draft = await b.call<Draft>("draft.create", { kind: "new" });
    window.__frostmailCompose = draft.id;
    window.__frostmailComposeFresh = true;
    window.__frostmailMaild = "local";
    render(<App connect={failing(2, b)} />);

    await screen.findByText("This draft is on the maild you left.", undefined, { timeout: 15_000 });
    await waitFor(() => expect(env.close).toHaveBeenCalled());
    expect(callsOf(b, "draft.get")).toEqual([]);
    expect(callsOf(b, "draft.update")).toEqual([]);
    expect(callsOf(b, "draft.delete")).toEqual([]);
    expect(screen.queryByRole("toolbar", { name: "Compose" })).toBeNull();
  });

  it("closes an event window before its first handshake with the other maild", async () => {
    const b = new MockTransport(data());
    b.maild = `lodge:${LODGE}`;
    const probe = new MockTransport(data());
    const event = await probe.call<CalendarEvent>("calendar.event", { id: 302 });
    window.__frostmailEvent = { kind: "edit", id: event.id, recurrenceId: "" };
    window.__frostmailMaild = "local";
    render(<App connect={failing(1, b)} />);

    await screen.findByText("This event is on the maild you left.", undefined, { timeout: 15_000 });
    await waitFor(() => expect(env.close).toHaveBeenCalled());
    expect(callsOf(b, "calendar.event")).toEqual([]);
    expect(callsOf(b, "calendar.update")).toEqual([]);
    expect(callsOf(b, "calendar.delete")).toEqual([]);
    expect(screen.queryByLabelText("Title")).toBeNull();
  });

  it("opens a compose window whose owner is the maild it reaches", async () => {
    const a = new MockTransport(data());
    a.maild = "local";
    const draft = await a.call<Draft>("draft.create", { kind: "new" });
    window.__frostmailCompose = draft.id;
    window.__frostmailComposeFresh = true;
    window.__frostmailMaild = "local";
    render(<App connect={connect(a)} />);

    await screen.findByRole("toolbar", { name: "Compose" });
    expect(callsOf(a, "draft.get")).toEqual([{ id: draft.id }]);
    expect(env.close).not.toHaveBeenCalled();
    expect(screen.queryByText("This draft is on the maild you left.")).toBeNull();
  });
});

describe("an event window after a change of maild", () => {
  it("closes instead of opening its event on the other maild", async () => {
    const a = new MockTransport(data());
    const b = new MockTransport(data());
    a.maild = "local";
    b.maild = `lodge:${LODGE}`;
    // The fixture's event 302 is on both mailds.
    const probe = new MockTransport(data());
    const event = await probe.call<CalendarEvent>("calendar.event", { id: 302 });
    expect(event.id).toBe(302);
    window.__frostmailEvent = { kind: "edit", id: event.id, recurrenceId: "" };
    render(<App connect={connect(a, b)} />);

    await screen.findByLabelText("Title");
    expect(callsOf(a, "calendar.event")).toEqual([{ id: event.id }]);

    a.close("switch");
    await screen.findByText("This event is on the maild you left.");
    await waitFor(() => expect(env.close).toHaveBeenCalled());
    expect(callsOf(b, "calendar.event")).toEqual([]);
    expect(screen.queryByLabelText("Title")).toBeNull();
  });
});
