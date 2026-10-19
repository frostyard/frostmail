// The desktop app hands a remote maild's announcements to its own notifier
// (docs/design/desktop.md, Notifications).
import { invoke } from "@tauri-apps/api/core";
import { describe, expect, it, vi } from "vitest";

import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { watchAnnouncements } from "./announce";

vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  invoke: vi.fn(() => Promise.resolve()),
  isTauri: () => true,
}));

describe("watchAnnouncements in the desktop app", () => {
  it("hands each announcement to the app's notifier", async () => {
    const mock = new MockTransport(mockData({ inbox: 1 }));
    const stop = watchAnnouncements(new Client(mock));
    const notes = [{ title: "Ann", body: "Plan", messageId: 7 }];
    mock.emit({ event: "message.announced", data: { accountId: 2, notes } });
    await vi.waitFor(() => expect(invoke).toHaveBeenCalledWith("notify_mail", { accountId: 2, notes }));
    stop();
  });
});
