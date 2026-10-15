import { invoke } from "@tauri-apps/api/core";
import { describe, expect, it, vi } from "vitest";

import { openLink } from "./ReaderContainer";

vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  invoke: vi.fn(),
  isTauri: () => true,
}));

describe("openLink", () => {
  it("asks the app shell to open the link", () => {
    vi.mocked(invoke).mockResolvedValue(undefined);
    openLink("https://example.test/a");
    expect(invoke).toHaveBeenCalledWith("open_link", { url: "https://example.test/a" });
  });

  it("logs a link the shell could not open", async () => {
    vi.mocked(invoke).mockRejectedValue("no launcher worked");
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    openLink("https://example.test/b");
    await vi.waitFor(() => expect(warn).toHaveBeenCalledWith("open link", "no launcher worked"));
    warn.mockRestore();
  });
});
