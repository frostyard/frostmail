// A window whose reconnect reaches another maild forgets the old maild's ids
// (docs/specs/settings-ui.md, Server pane; frostmail#186): it opens nothing,
// and marks nothing read, by an id that only the old maild had. A reconnect to
// the same maild keeps them.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "../app/App";
import type { MessageSummary } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import type { Transport } from "../rpc/transport";
import { useUI } from "./stores";

const NOW = new Date("2026-10-07T12:00:00Z");
const LODGE = "https://lodge.example.ts.net";

beforeEach(() => {
  localStorage.clear();
  window.location.hash = "";
  useUI.setState(useUI.getInitialState());
});

/** maild is a mock of one maild, under the key its transport carries. */
function maild(key: string): MockTransport {
  const mock = new MockTransport(mockData({ inbox: 3, now: NOW }));
  mock.maild = key;
  return mock;
}

/** connect answers each transport in turn, then the last one again. */
function connect(...transports: MockTransport[]): () => Promise<Transport> {
  let next = 0;
  return () => {
    const transport = transports[Math.min(next, transports.length - 1)];
    next += 1;
    if (!transport) throw new Error("connect with no transport");
    return Promise.resolve(transport);
  };
}

/** setFlags are the params of every message.setFlags a maild saw. */
function setFlags(mock: MockTransport): unknown[] {
  return mock.calls.filter((c) => c.method === "message.setFlags").map((c) => c.params);
}

/** served waits until this maild answers the window's list. */
async function served(mock: MockTransport): Promise<void> {
  await waitFor(
    () => expect(mock.calls.some((c) => c.method === "view.open" || c.method === "view.range")).toBe(true),
    { timeout: 15_000 },
  );
}

/** unseenRow is the id of the first row the list shows whose message is
 * unseen: the reader marks that one read, which shows the reader ran. */
async function unseenRow(mock: MockTransport): Promise<number> {
  const list = await screen.findByRole("listbox", { name: "Messages" });
  const rows = await waitFor(() => {
    const found = within(list).queryAllByRole("option");
    if (found.length === 0) throw new Error("the list has no rows yet");
    return found;
  });
  for (const row of rows) {
    const id = Number(row.getAttribute("data-message-id"));
    const [summary] = await mock.call<MessageSummary[]>("message.summaries", { ids: [id] });
    if (summary && !summary.flags.seen) return id;
  }
  throw new Error("the list shows no unseen message");
}

describe("a window that changes maild", () => {
  it("forgets the message ids of the maild it left", async () => {
    const a = maild("local");
    const b = maild(`lodge:${LODGE}`);
    render(<App connect={connect(a, b)} />);

    const id = await unseenRow(a);
    // Positive control: in this setup the reader marks the message read.
    act(() => useUI.getState().select([id]));
    await waitFor(() => expect(setFlags(a)).toContainEqual({ ids: [id], changes: { seen: true } }));

    // The id means an unseen message on B too, so a kept selection would
    // mark it read there.
    const same = maild(`lodge:${LODGE}`);
    const [summary] = await same.call<MessageSummary[]>("message.summaries", { ids: [id] });
    expect(summary?.flags.seen).toBe(false);

    a.close("switch");
    await served(b);
    // The reader has nothing to open on B.
    const reader = await screen.findByRole("region", { name: "Message" });
    await waitFor(() => expect(reader.textContent).toContain("No Message Selected"));
    expect(useUI.getState().selected).toEqual([]);
    expect(setFlags(b)).toEqual([]);
  });

  it("keeps them when it reconnects to the same maild", async () => {
    const a = maild("local");
    const c = maild("local");
    render(<App connect={connect(a, c)} />);

    const id = await unseenRow(a);
    act(() => useUI.getState().select([id]));
    await waitFor(() => expect(setFlags(a)).toContainEqual({ ids: [id], changes: { seen: true } }));

    a.close("network drop");
    await served(c);
    await waitFor(() => expect(useUI.getState().selected).toEqual([id]));
    // The reader runs on C with the same message, as it should: same maild.
    await waitFor(() => expect(setFlags(c)).toContainEqual({ ids: [id], changes: { seen: true } }));
  });
});
