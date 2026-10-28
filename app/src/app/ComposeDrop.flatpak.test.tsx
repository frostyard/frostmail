// The Flatpak attaches a file by its bytes, in both modes: its maild cannot
// read the paths the window sees outside the sandbox, so files dropped on the
// window reach the webview (open_compose leaves them to it), and Attach Files
// opens the webview's own chooser, which goes through the file chooser portal
// (ADR-0046, Files in the sandbox; docs/specs/compose-ui.md, Attachments).
import { createEvent, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ComposeWindow } from "./ComposeWindow";

/** shell is the webview as the app sees it: its drops, and the dialog
 * plugin's file chooser, which the Flatpak must not use. */
const shell = vi.hoisted(() => ({
  onDragDropEvent: vi.fn(async () => () => {}),
  openDialog: vi.fn(async () => null),
}));

vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  isTauri: () => true,
}));
vi.mock("@tauri-apps/api/window", () => ({
  getCurrentWindow: () => ({
    isMaximized: async () => false,
    onResized: async () => () => {},
    setTitle: async () => {},
  }),
}));
vi.mock("@tauri-apps/api/webview", () => ({ getCurrentWebview: () => ({ onDragDropEvent: shell.onDragDropEvent }) }));
vi.mock("@tauri-apps/plugin-dialog", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/plugin-dialog")>()),
  open: shell.openDialog,
}));
// The Flatpak on a maild of its own: no lodge, and every file goes by bytes.
vi.mock("../rpc/transport/tauri", () => ({
  remoteMaild: () => false,
  inFlatpak: () => true,
  filesAsBytes: () => true,
}));

beforeEach(() => {
  shell.onDragDropEvent.mockClear();
  shell.openDialog.mockClear();
});

/** setup renders a new draft's compose window on the mock transport. */
async function setup(): Promise<MockTransport> {
  const mock = new MockTransport(mockData({ inbox: 1 }));
  const draft = await new Client(mock).draft.create({ kind: "new" });
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <ComposeWindow draftId={draft.id} fresh={false} />
    </Session>,
  );
  await screen.findByRole("toolbar", { name: "Compose" });
  return mock;
}

describe("ComposeWindow in the Flatpak", () => {
  it("attaches a dropped file by its bytes, and leaves drops to the webview", async () => {
    const mock = await setup();
    expect(shell.onDragDropEvent).not.toHaveBeenCalled();
    const file = new File(["hello"], "notes.txt", { type: "text/plain" });
    const drop = createEvent.drop(window, { dataTransfer: { types: ["Files"], files: [file] } });
    fireEvent(window, drop);
    await waitFor(() => expect(mock.calls.some((c) => c.method === "draft.attachData")).toBe(true));
    expect(mock.calls.some((c) => c.method === "draft.attach")).toBe(false);
  });

  it("uses the webview's own chooser for Attach Files, and sends its file by bytes", async () => {
    const mock = await setup();
    const input = screen.getByLabelText("Files to attach") as HTMLInputElement;
    const click = vi.spyOn(input, "click").mockImplementation(() => {});
    fireEvent.click(screen.getByRole("button", { name: "Attach Files" }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(shell.openDialog).not.toHaveBeenCalled();

    fireEvent.change(input, { target: { files: [new File(["hello"], "notes.txt", { type: "text/plain" })] } });
    await waitFor(() => expect(mock.calls.some((c) => c.method === "draft.attachData")).toBe(true));
    expect(mock.calls.some((c) => c.method === "draft.attach")).toBe(false);
  });
});
