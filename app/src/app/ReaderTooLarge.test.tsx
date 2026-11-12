// In lodge, a message's HTML shows only from lodge-content; one too large
// for maild to keep says so, rather than framing it on lodge's own origin
// (docs/specs/lodge-app.md, /content/).
import { render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { type Lodge, setLodge } from "../lodge/lodge";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

// A shell that answers every call, as lodge-client would.
const shell = new Proxy({ contentURL: (p: string) => `https://content.test/mail/${p}` } as Record<string, unknown>, {
  get: (target, key: string) => (key in target ? target[key] : () => () => {}),
}) as unknown as Lodge;

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  setLodge(shell);
});

afterEach(() => setLodge(null));

describe("the reader in lodge", () => {
  it("says a message is too large to show when maild kept no page for it", async () => {
    const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
    const html = data.messages.find((m) => m.html !== "");
    if (!html) throw new Error("no HTML message in the fixture");
    // The mock renders no contentPath, as maild does for a page past 64 MiB.
    render(
      <Session connect={() => Promise.resolve(new MockTransport(data))} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await screen.findByRole("listbox", { name: "Messages" });
    useUI.getState().select([html.summary.id], html.summary.id);
    const reader = screen.getByRole("region", { name: "Message" });
    await waitFor(() => expect(within(reader).getByText(/too large to show here/)).toBeTruthy());
    expect(reader.querySelector("iframe")).toBeNull();
  });

  it("asks maild for the content page and frames only lodge-content's URL", async () => {
    const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
    const html = data.messages.find((m) => m.html !== "");
    if (!html) throw new Error("no HTML message in the fixture");
    const mock = new MockTransport(data);
    const call = mock.call.bind(mock);
    // maild keeps the page and names it, as it does on its lodge socket.
    mock.call = (async (method: string, params: unknown) => {
      const result = await call(method, params);
      return method === "message.render" ? { ...(result as object), contentPath: "r/page.html" } : result;
    }) as typeof mock.call;
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await screen.findByRole("listbox", { name: "Messages" });
    useUI.getState().select([html.summary.id], html.summary.id);
    const reader = screen.getByRole("region", { name: "Message" });
    const frame = await waitFor(() => {
      const f = reader.querySelector("iframe");
      if (!f) throw new Error("no frame yet");
      return f;
    });
    expect(frame.getAttribute("src")).toBe("https://content.test/mail/r/page.html");
    expect(frame.hasAttribute("srcdoc")).toBe(false);
    const renders = mock.calls.filter((c) => c.method === "message.render");
    expect(renders.length).toBeGreaterThan(0);
    for (const c of renders) expect((c.params as { content?: boolean }).content).toBe(true);
  });
});

describe("the reader with no lodge shell", () => {
  beforeEach(() => setLodge(null));

  it("never frames a message's HTML on the app's own origin", async () => {
    const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
    const html = data.messages.find((m) => m.html !== "");
    if (!html) throw new Error("no HTML message in the fixture");
    render(
      <Session connect={() => Promise.resolve(new MockTransport(data))} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await screen.findByRole("listbox", { name: "Messages" });
    useUI.getState().select([html.summary.id], html.summary.id);
    const reader = screen.getByRole("region", { name: "Message" });
    await waitFor(() => expect(within(reader).getByText(/too large to show here/)).toBeTruthy());
    expect(reader.querySelector("iframe")).toBeNull();
  });
});
