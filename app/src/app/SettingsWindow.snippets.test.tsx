import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

async function setup() {
  const mock = new MockTransport(
    mockData({
      inbox: 1,
      now: new Date("2026-11-07T12:00:00Z"),
      snippets: [{ id: 1, name: "Greeting", html: "<p>Hello</p>", updatedAt: "2026-11-07T12:00:00Z" }],
    }),
  );
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <SettingsWindow />
    </Session>,
  );
  await screen.findByRole("button", { name: "Snippets" });
  fireEvent.click(screen.getByRole("button", { name: "Snippets" }));
  await screen.findByRole("list", { name: "Snippets" });
  return mock;
}

afterEach(() => vi.unstubAllGlobals());

describe("SettingsWindow snippets", () => {
  it("removes only after confirmation", async () => {
    const mock = await setup();
    vi.stubGlobal("confirm", () => false);
    fireEvent.click(screen.getByRole("button", { name: "Remove Snippet" }));
    await waitFor(() => expect(mock.calls.filter((call) => call.method === "snippet.delete")).toEqual([]));

    vi.stubGlobal("confirm", () => true);
    fireEvent.click(screen.getByRole("button", { name: "Remove Snippet" }));
    await waitFor(() =>
      expect(mock.calls.filter((call) => call.method === "snippet.delete")).toEqual([
        { method: "snippet.delete", params: { id: 1 } },
      ]),
    );
  });
});
