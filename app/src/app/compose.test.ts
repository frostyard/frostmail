// A compose window learns which maild owns its draft, so it never opens that
// id on another one (docs/specs/settings-ui.md, Server pane).
import { beforeEach, describe, expect, it, vi } from "vitest";

const env = vi.hoisted(() => ({
  invoke: vi.fn(async () => null),
  maild: "local" as string | undefined,
  tauri: true,
}));

vi.mock("@tauri-apps/api/core", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tauri-apps/api/core")>()),
  invoke: env.invoke,
  isTauri: () => env.tauri,
}));
vi.mock("../rpc/transport/tauri", () => ({ maildKey: () => env.maild }));

const { openCompose } = await import("./compose");

beforeEach(() => {
  env.tauri = true;
  env.maild = "local";
  env.invoke.mockClear();
});

describe("openCompose", () => {
  it("passes the key of the maild that owns the draft", async () => {
    await openCompose(7, true);
    expect(env.invoke).toHaveBeenCalledWith("open_compose", { draftId: 7, fresh: true, maild: "local" });
  });

  it("passes no key before any connection", async () => {
    env.maild = undefined;
    await openCompose(7);
    expect(env.invoke).toHaveBeenCalledWith("open_compose", { draftId: 7, fresh: false, maild: null });
  });
});
