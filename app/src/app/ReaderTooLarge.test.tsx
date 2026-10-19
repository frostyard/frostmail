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
});
