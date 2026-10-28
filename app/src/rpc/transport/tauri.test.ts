import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The bridge as the webview sees it: maild_connect answers whether maild is
// behind lodge, in_flatpak whether the app runs in its Flatpak, and maild_send
// carries lines.
const sent: string[] = [];
let answer: { remote: boolean } | null = null;
let flatpakAnswer: boolean | null = false;
vi.mock("@tauri-apps/api/core", () => ({
  Channel: class {
    onmessage: ((m: unknown) => void) | null = null;
  },
  invoke: vi.fn(async (cmd: string, args: { line?: string }) => {
    if (cmd === "maild_send" && args.line) sent.push(args.line);
    if (cmd === "maild_connect") return answer;
    if (cmd === "in_flatpak") return flatpakAnswer;
    return null;
  }),
}));

const { connectTauri, filesAsBytes, inFlatpak, PING_MS, remoteMaild } = await import("./tauri");

describe("connectTauri", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    sent.length = 0;
  });
  afterEach(() => vi.useRealTimers());

  it("pings a maild behind lodge, and only that one", async () => {
    answer = { remote: true };
    await connectTauri();
    expect(remoteMaild()).toBe(true);
    await vi.advanceTimersByTimeAsync(PING_MS);
    expect(sent.filter((l) => l.includes('"rpc.ping"'))).toHaveLength(1);

    sent.length = 0;
    answer = null; // the bridge on maild's own socket
    await connectTauri();
    expect(remoteMaild()).toBe(false);
    await vi.advanceTimersByTimeAsync(PING_MS * 2);
    expect(sent.filter((l) => l.includes('"rpc.ping"'))).toHaveLength(2); // the first connection's, still open
  });

  it("attaches by bytes in the Flatpak, on a maild of its own", async () => {
    answer = null;
    flatpakAnswer = true;
    await connectTauri();
    expect(inFlatpak()).toBe(true);
    expect(remoteMaild()).toBe(false);
    expect(filesAsBytes(), "the Flatpak's maild cannot read the window's paths").toBe(true);

    // A shell that knows no in_flatpak is no Flatpak.
    flatpakAnswer = null;
    await connectTauri();
    expect(inFlatpak()).toBe(false);
    expect(filesAsBytes()).toBe(false);

    // A maild behind lodge takes bytes outside a Flatpak too.
    answer = { remote: true };
    flatpakAnswer = null;
    await connectTauri();
    expect(filesAsBytes()).toBe(true);
  });
});
