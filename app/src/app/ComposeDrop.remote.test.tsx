// The desktop app on a maild behind lodge attaches dropped files by their
// bytes, as a browser does: maild cannot read this machine's paths
// (docs/design/desktop.md, On a maild behind lodge).
import { createEvent, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { ComposeWindow } from "./ComposeWindow";

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
const onDragDropEvent = vi.fn(async () => () => {});
vi.mock("@tauri-apps/api/webview", () => ({ getCurrentWebview: () => ({ onDragDropEvent }) }));
vi.mock("../rpc/transport/tauri", () => ({
  remoteMaild: () => true,
  inFlatpak: () => false,
  filesAsBytes: () => true,
}));

describe("ComposeWindow on a maild behind lodge", () => {
  it("attaches a dropped file by its bytes, not its path", async () => {
    const mock = new MockTransport(mockData({ inbox: 1 }));
    const draft = await new Client(mock).draft.create({ kind: "new" });
    render(
      <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
        <ComposeWindow draftId={draft.id} fresh={false} />
      </Session>,
    );
    await screen.findByRole("toolbar", { name: "Compose" });
    expect(onDragDropEvent).not.toHaveBeenCalled();
    const file = new File(["hello"], "notes.txt", { type: "text/plain" });
    const drop = createEvent.drop(window, { dataTransfer: { types: ["Files"], files: [file] } });
    fireEvent(window, drop);
    await waitFor(() => expect(mock.calls.some((c) => c.method === "draft.attachData")).toBe(true));
    expect(mock.calls.some((c) => c.method === "draft.attach")).toBe(false);
  });
});
