// The narrow window's Mail (docs/specs/ui.md, Narrow windows): one pane
// at a time, Back between them, and the message bar.
import { readFileSync } from "node:fs";

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";
import { NARROW, narrowPane } from "./narrow";

function stubWidth(narrow: boolean) {
  vi.stubGlobal("matchMedia", (media: string) => ({
    matches: narrow && media === NARROW,
    media,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function setup(narrow = true) {
  stubWidth(narrow);
  const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
  const { container } = render(
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
  const pane = () => container.querySelector("[data-narrow-pane]")?.getAttribute("data-narrow-pane");
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  return { list, row, pane, calls };
}

describe("narrowPane", () => {
  it("shows Mailboxes when asked, else the message when one is selected, else the list", () => {
    expect(narrowPane(true, [4])).toBe("mailboxes");
    expect(narrowPane(false, [4])).toBe("reader");
    expect(narrowPane(false, [])).toBe("list");
    expect(narrowPane(false, [4, 5])).toBe("list");
  });

  it("is the query app.css's narrow variant uses", () => {
    // Tests run in app/.
    const css = readFileSync("src/styles/app.css", "utf8");
    expect(css).toContain(`@custom-variant narrow (@media ${NARROW});`);
  });
});

describe("a narrow window's Mail", () => {
  it("shows a message in place of the list, with the message bar, and Back returns to the list", async () => {
    const { list, row, pane, calls } = await setup();
    expect(pane()).toBe("list");
    expect(screen.queryByRole("toolbar", { name: "Message Actions" })).toBeNull();
    expect(screen.queryByRole("separator", { name: "Resize message list" })).toBeNull();
    fireEvent.click(row);
    await waitFor(() => expect(pane()).toBe("reader"));
    expect(list.closest("[inert]")).not.toBeNull();
    const bar = screen.getByRole("toolbar", { name: "Message Actions" });
    const id = Number(row.getAttribute("data-message-id"));
    fireEvent.click(within(bar).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(calls("message.delete")).toEqual([{ ids: [id] }]));
    useUI.getState().select([id], id);
    await waitFor(() => expect(pane()).toBe("reader"));
    fireEvent.click(screen.getByRole("button", { name: "Back to All Inboxes" }));
    expect(useUI.getState().selected).toEqual([]);
    expect(pane()).toBe("list");
    expect(list.closest("[inert]")).toBeNull();
  });

  it("shows Mailboxes, without the module bar, until a mailbox is chosen", async () => {
    const { pane } = await setup();
    fireEvent.click(screen.getByRole("button", { name: "Back to Mailboxes" }));
    expect(pane()).toBe("mailboxes");
    const toolbar = screen.getByRole("toolbar", { name: "Toolbar" });
    expect(within(toolbar).getByText("Mailboxes")).toBeTruthy();
    expect(within(toolbar).getByRole("button", { name: "Get Mail" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Calendar" })).toBeNull();
    const inbox = await screen.findAllByRole("treeitem", { name: /^Inbox/ });
    fireEvent.click(inbox[0] as HTMLElement);
    expect(useUI.getState().mailboxesOpen).toBe(false);
    expect(pane()).toBe("list");
    expect(screen.getByRole("button", { name: "Back to Mailboxes" })).toBeTruthy();
  });

  it("goes back to Mail from another module", async () => {
    await setup();
    useUI.getState().setModule("calendar");
    fireEvent.click(await screen.findByRole("button", { name: "Back to Mail" }));
    expect(useUI.getState().module).toBe("mail");
  });

  it("keeps the panes side by side in a wide window", async () => {
    const { pane } = await setup(false);
    expect(pane()).toBeUndefined();
    expect(screen.getByRole("separator", { name: "Resize message list" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Back to Mailboxes" })).toBeNull();
  });
});
