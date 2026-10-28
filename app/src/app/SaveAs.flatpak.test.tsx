// Save As in the Flatpak: the file chooser portal's dialog picks the path, in
// both modes, and maild writes the message there (message.save) or the app
// downloads it from lodge's content node there (save_content)
// (ADR-0046, Files in the sandbox; docs/specs/ui.md, More Actions).
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import type { MessageSummary } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { MainWindow } from "./MainWindow";

/** SAVED is the path the document portal answers; the app then writes there. */
const SAVED = "/run/user/1000/doc/5c1e/Lunch.eml";

/** shell is the webview as the app sees it: the plugin's dialog, the commands
 * it invokes, and what remoteMaild and the portal answer. */
const shell = vi.hoisted(() => {
  const state: { remote: boolean; portal: string | null } = { remote: false, portal: "" };
  return {
    state,
    save: vi.fn(async () => "/home/u/Lunch.eml"),
    invoke: vi.fn(
      async (cmd: string, _args?: unknown): Promise<unknown> => (cmd === "portal_save_file" ? state.portal : null),
    ),
  };
});

vi.mock("@tauri-apps/plugin-dialog", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/plugin-dialog")>()),
  save: shell.save,
}));
vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  invoke: shell.invoke,
  isTauri: () => true,
}));
vi.mock("../rpc/transport/tauri", () => ({
  remoteMaild: () => shell.state.remote,
  inFlatpak: () => true,
  filesAsBytes: () => true,
}));

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
  shell.state.remote = false;
  shell.state.portal = SAVED;
  shell.save.mockClear();
  shell.invoke.mockClear();
});

/** setup shows the first message and opens its More Actions menu. */
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
  const [summary] = await mock.call<MessageSummary[]>("message.summaries", { ids: [id] });
  fireEvent.click(row);
  const reader = screen.getByRole("region", { name: "Message" });
  await waitFor(() =>
    expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
  );
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  const saveAs = () => {
    fireEvent.click(within(reader).getAllByRole("button", { name: "More Actions" })[0] as HTMLElement);
    fireEvent.click(within(screen.getByRole("menu")).getByText("Save As…"));
  };
  const name = (summary?.subject.trim() || "(no subject)").replace(/[/\\]/g, "-");
  return { id, calls, saveAs, name };
}

/** asked is what the portal command was asked, once it has been asked. */
async function asked(): Promise<unknown> {
  await waitFor(() => expect(shell.invoke.mock.calls.some(([cmd]) => cmd === "portal_save_file")).toBe(true));
  return shell.invoke.mock.calls.find(([cmd]) => cmd === "portal_save_file")?.[1];
}

describe("Save As in the Flatpak", () => {
  it("asks the portal, then has maild write the message there", async () => {
    const { id, calls, saveAs, name } = await setup();
    saveAs();
    expect(await asked()).toEqual({
      title: "Save Message",
      currentName: `${name}.eml`,
      filterName: "Email Message",
      extension: "eml",
    });
    await waitFor(() => expect(calls("message.save")).toEqual([{ id, path: SAVED }]));
    expect(shell.save).not.toHaveBeenCalled();
  });

  it("downloads the message there on a maild behind lodge", async () => {
    const { id, calls, saveAs } = await setup();
    shell.state.remote = true;
    saveAs();
    await waitFor(() => expect(calls("message.source")).toEqual([{ id, content: true }]));
    const download = shell.invoke.mock.calls.find(([cmd]) => cmd === "save_content");
    expect(download?.[1]).toEqual({ contentPath: `m/mockmac/${id}/message.eml`, path: SAVED });
    expect(calls("message.save")).toEqual([]);
    expect(shell.save).not.toHaveBeenCalled();
  });

  it("writes nothing when the portal's dialog is cancelled", async () => {
    const { calls, saveAs } = await setup();
    shell.state.portal = null;
    saveAs();
    await asked();
    expect(calls("message.save")).toEqual([]);
    expect(calls("message.source")).toEqual([]);
    expect(shell.invoke.mock.calls.some(([cmd]) => cmd === "save_content")).toBe(false);
    expect(shell.save).not.toHaveBeenCalled();
  });
});
