// Save As of a whole message in lodge's web build: a download from the
// content route (docs/specs/lodge-app.md, The web build).
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { type Lodge, setLodge } from "../lodge/lodge";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

const open = vi.fn();

// The shell's connection, from the click on: Save As asks it for no more.
function inLodge() {
  setLodge({ open, contentURL: (p: string) => `https://content.test/mail/${p}` } as unknown as Lodge);
}

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  open.mockReset();
});

afterEach(() => {
  setLodge(null);
  vi.restoreAllMocks();
});

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 3, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  const row = await waitFor(() => {
    const r = within(list).queryAllByRole("option")[0];
    if (!r) throw new Error("no rows yet");
    return r;
  });
  const id = Number(row.getAttribute("data-message-id"));
  fireEvent.click(row);
  const reader = screen.getByRole("region", { name: "Message" });
  await waitFor(() =>
    expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
  );
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  const more = () => {
    fireEvent.click(within(reader).getAllByRole("button", { name: "More Actions" })[0] as HTMLElement);
    return screen.getByRole("menu");
  };
  return { id, calls, more };
}

describe("Save As in lodge", () => {
  it("downloads the message from the content route", async () => {
    const { id, calls, more } = await setup();
    inLodge();
    fireEvent.click(within(more()).getByText("Save As…"));
    await waitFor(() =>
      expect(open).toHaveBeenCalledWith(`https://content.test/mail/m/mockmac/${id}/message.eml?download=1`),
    );
    expect(calls("message.source")).toEqual([{ id, content: true }]);
    expect(calls("message.save")).toEqual([]);
  });

  it("is in Raw Source's sheet too", async () => {
    const { id, more } = await setup();
    fireEvent.click(within(more()).getByText("Raw Source…"));
    const sheet = await screen.findByRole("dialog", { name: "Raw Source" });
    inLodge();
    fireEvent.click(within(sheet).getByRole("button", { name: "Save As…" }));
    await waitFor(() =>
      expect(open).toHaveBeenCalledWith(`https://content.test/mail/m/mockmac/${id}/message.eml?download=1`),
    );
  });
});
