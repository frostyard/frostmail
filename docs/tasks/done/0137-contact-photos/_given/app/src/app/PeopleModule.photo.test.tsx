// CONTRACT TEST for task card T-0137 (docs/tasks). Do not edit.
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

// The webview scales photos on a canvas, which the test DOM cannot draw.
vi.mock("../lib/photo", async (actual) => ({
  ...(await actual<typeof import("../lib/photo")>()),
  scalePhoto: vi.fn(async () => ({ contentType: "image/jpeg", data: "c2NhbGVk" })),
}));

async function setup() {
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  await screen.findByRole("tree", { name: "Mailboxes" });
  const calls = (method: string) => mock.calls.filter((c) => c.method === method);
  return { mock, calls };
}

const ctrl = (key: string) => fireEvent.keyDown(window, { key, ctrlKey: true });
const contacts = () => screen.getByRole("listbox", { name: "Contacts" });
const pane = () => screen.getByRole("region", { name: "Contact details" });

async function edit(name: string) {
  ctrl("3");
  await screen.findByRole("tree", { name: "Address Books" });
  fireEvent.click(await within(contacts()).findByRole("option", { name: new RegExp(name) }));
  await within(pane()).findByRole("heading", { name, level: 2 });
  fireEvent.click(within(pane()).getByRole("button", { name: "Edit Contact" }));
}

function pick() {
  const file = new File(["jpeg"], "photo.jpg", { type: "image/jpeg" });
  fireEvent.change(within(pane()).getByLabelText("Photo file"), { target: { files: [file] } });
}

beforeEach(() => {
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("contact photos, deletes and refusals", () => {
  it("sets a picked photo with Done", async () => {
    const { calls } = await setup();
    await edit("Ann Smith");
    pick();
    await within(pane()).findByRole("button", { name: "Remove Photo" });
    fireEvent.click(within(pane()).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(calls("people.setPhoto")).toHaveLength(1));
    expect(calls("people.setPhoto")[0]?.params).toEqual({
      id: 201,
      photo: { contentType: "image/jpeg", data: "c2NhbGVk" },
    });
    expect(calls("people.update")).toHaveLength(0);
    await waitFor(() => expect(within(pane()).queryByRole("button", { name: "Done" })).toBeNull());
  });

  it("gives a new contact its photo after making it", async () => {
    const { calls } = await setup();
    ctrl("3");
    await screen.findByRole("tree", { name: "Address Books" });
    fireEvent.click(screen.getByRole("button", { name: "New Contact" }));
    await within(pane()).findByRole("heading", { name: "New Contact", level: 2 });
    fireEvent.change(within(pane()).getByRole("textbox", { name: "First" }), { target: { value: "Kai" } });
    pick();
    await within(pane()).findByRole("button", { name: "Remove Photo" });
    fireEvent.click(within(pane()).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(calls("people.setPhoto")).toHaveLength(1));
    expect(calls("people.create")).toHaveLength(1);
    const params = calls("people.setPhoto")[0]?.params as { id: number } | undefined;
    expect(params?.id).toBeGreaterThan(1000);
  });

  it("removes a photo", async () => {
    const { mock, calls } = await setup();
    await new Client(mock).people.setPhoto({ id: 203, photo: { contentType: "image/png", data: "cG5n" } });
    await edit("Carmen Lindqvist");
    fireEvent.click(await within(pane()).findByRole("button", { name: "Remove Photo" }));
    fireEvent.click(within(pane()).getByRole("button", { name: "Done" }));
    await waitFor(() => expect(calls("people.setPhoto")).toHaveLength(2));
    expect(calls("people.setPhoto")[1]?.params).toEqual({ id: 203 });
  });

  it("deletes a contact after asking", async () => {
    const { calls } = await setup();
    const confirm = vi.fn((_message?: string) => false);
    vi.stubGlobal("confirm", confirm);
    await edit("Bob Okafor");
    fireEvent.click(within(pane()).getByRole("button", { name: "Delete Contact" }));
    await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
    expect(String(confirm.mock.calls[0]?.[0])).toContain('Delete "Bob Okafor" from Contacts');
    expect(calls("people.delete")).toHaveLength(0);
    confirm.mockReturnValue(true);
    fireEvent.click(within(pane()).getByRole("button", { name: "Delete Contact" }));
    await waitFor(() => expect(calls("people.delete")).toHaveLength(1));
    expect(calls("people.delete")[0]?.params).toEqual({ id: 202 });
    await waitFor(() => expect(within(contacts()).queryByRole("option", { name: /Bob Okafor/ })).toBeNull());
  });

  it("shows a refused change until dismissed", async () => {
    const { mock } = await setup();
    // The mock delivers events on a microtask: flush it inside act, so the
    // notice renders before the query, however busy the machine is.
    await act(async () => {
      mock.emit({
        event: "people.refused",
        data: { accountId: 1, contactId: 201, name: "Ann Smith", reason: "changed" },
      });
      await Promise.resolve();
    });
    const notice = await screen.findByText("“Ann Smith” was changed on another device, so your change wasn't saved.");
    fireEvent.click(within(notice.closest("output") as HTMLElement).getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText(/was changed on another device/)).toBeNull();
  });
});
