// P-211: the list's rows follow the preview-lines preference, as it changes.
import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

const rows = () => screen.getAllByRole("option").filter((row) => row.hasAttribute("data-message-id"));

describe("List preview lines", () => {
  it("shows 2 lines until the preference changes, then follows it without a reload", async () => {
    const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
    const client = new Client(mock);
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <MainWindow />
      </Session>,
    );
    await screen.findByRole("tree", { name: "Mailboxes" });
    await waitFor(() => expect(rows().length).toBeGreaterThan(0));
    const first = () => rows()[0] as HTMLElement;
    expect(first().style.height).toBe("");
    expect(first().querySelector(".text-list-preview")).not.toBeNull();

    await act(async () => {
      await client.settings.set({ previewLines: 0 });
    });
    await waitFor(() => expect(first().style.height).toBe("52px"));
    expect(first().querySelector(".text-list-preview")).toBeNull();
    for (const row of rows()) expect(row.style.height).toBe("52px");

    await act(async () => {
      await client.settings.set({ previewLines: 5 });
    });
    await waitFor(() => expect(first().style.height).toBe("132px"));
    const preview = first().querySelector<HTMLElement>(".text-list-preview");
    expect(preview?.className).toContain("line-clamp-5");

    await expect(client.settings.set({ previewLines: 6 })).rejects.toThrow();
  });
});
