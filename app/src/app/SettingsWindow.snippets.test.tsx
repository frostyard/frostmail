import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { SettingsWindow } from "./SettingsWindow";

async function setup(
  snippets = [{ id: 1, name: "Greeting", html: "<p>Hello</p>", updatedAt: "2026-11-07T12:00:00Z" }],
) {
  const mock = new MockTransport(
    mockData({
      inbox: 1,
      now: new Date("2026-11-07T12:00:00Z"),
      snippets,
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
  it("reloads the pane after snippet.changed", async () => {
    const mock = await setup([]);
    await waitFor(() => expect(screen.getByText(/Snippets are text you insert/)).toBeTruthy());
    const client = new Client(mock);
    await client.snippet.create({ name: "Greeting", html: "<p>Hello</p>" });
    await waitFor(() => expect(screen.getByRole("button", { name: "Greeting" })).toBeTruthy());
  });

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
