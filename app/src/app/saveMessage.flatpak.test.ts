// Save As in the Flatpak: the file chooser portal's dialog picks the path, in
// both modes, and maild writes the message there (message.save) or the app
// downloads it from lodge's content node there (save_content)
// (ADR-0046, Files in the sandbox; docs/specs/ui.md, More Actions).
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { saveMessage } from "./saveMessage";

/** SAVED is the path the document portal answers; the app then writes there. */
const SAVED = "/run/user/1000/doc/5c1e/Lunch.eml";

/** shell is the webview as the app sees it: the plugin's dialog, the commands
 * it invokes, and what remoteMaild and the portal answer. */
const shell = vi.hoisted(() => {
  const state: { remote: boolean; flatpak: boolean; portal: string | null } = {
    remote: false,
    flatpak: true,
    portal: "",
  };
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
  inFlatpak: () => shell.state.flatpak,
  filesAsBytes: () => shell.state.remote || shell.state.flatpak,
}));

beforeEach(() => {
  shell.state.remote = false;
  shell.state.flatpak = true;
  shell.state.portal = SAVED;
  shell.save.mockClear();
  shell.invoke.mockClear();
});

/** setup gives a client on mock data and the first message's id. */
function setup() {
  const data = mockData({ inbox: 3, now: new Date("2026-10-08T12:00:00Z") });
  const mock = new MockTransport(data);
  const client = new Client(mock);
  const first = data.messages[0]?.summary.id;
  if (first === undefined) throw new Error("the mock has no message");
  const calls = (method: string) => mock.calls.filter((c) => c.method === method).map((c) => c.params);
  const commands = (cmd: string) => shell.invoke.mock.calls.filter(([c]) => c === cmd).map(([, args]) => args);
  return { client, id: first, calls, commands };
}

describe("Save As in the Flatpak", () => {
  it("asks the portal, then has maild write the message there", async () => {
    const { client, id, calls, commands } = setup();
    await saveMessage(client, id, "Lunch / plans");
    expect(commands("portal_save_file")).toEqual([
      { title: "Save Message", currentName: "Lunch - plans.eml", filterName: "Email Message", extension: "eml" },
    ]);
    expect(calls("message.save")).toEqual([{ id, path: SAVED }]);
    expect(shell.save).not.toHaveBeenCalled();
  });

  it("downloads the message there on a maild behind lodge", async () => {
    const { client, id, calls, commands } = setup();
    shell.state.remote = true;
    await saveMessage(client, id, "Lunch");
    expect(calls("message.source")).toEqual([{ id, content: true }]);
    expect(commands("save_content")).toEqual([{ contentPath: `m/mockmac/${id}/message.eml`, path: SAVED }]);
    expect(calls("message.save")).toEqual([]);
    expect(shell.save).not.toHaveBeenCalled();
  });

  it("writes nothing when the portal's dialog is cancelled", async () => {
    const { client, id, calls, commands } = setup();
    shell.state.portal = null;
    await saveMessage(client, id, "Lunch");
    expect(commands("portal_save_file")).toHaveLength(1);
    expect(calls("message.save")).toEqual([]);
    expect(calls("message.source")).toEqual([]);
    expect(commands("save_content")).toEqual([]);
    expect(shell.save).not.toHaveBeenCalled();
  });

  it("keeps Tauri's dialog outside the Flatpak", async () => {
    const { client, id, calls, commands } = setup();
    shell.state.flatpak = false;
    await saveMessage(client, id, "Lunch");
    expect(shell.save).toHaveBeenCalledTimes(1);
    expect(commands("portal_save_file")).toEqual([]);
    expect(calls("message.save")).toEqual([{ id, path: "/home/u/Lunch.eml" }]);
  });
});
