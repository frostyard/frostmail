// useFollowUp refreshes the Follow Up section as maild changes
// (docs/specs/ui.md, Follow Up).
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Session } from "../data/session";
import { useUI } from "../data/stores";
import { type Address, ErrorCode, type MessageSummary } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { type MockMessage, MockTransport } from "../rpc/mock/mock";
import { RPCError } from "../rpc/transport";
import { useFollowUp } from "./useFollowUp";

const ANN: Address = { name: "Ann Smith", address: "ann.smith@northwind.test" };
const ME: Address = { name: "Test One", address: "test1@mailtest.test" };

/** sent is a sent message 4 days old, from the account to someone else. */
function sent(): MockMessage {
  const summary: MessageSummary = {
    id: 9001,
    accountId: FIXTURE.accountId,
    mailboxIds: [FIXTURE.sent],
    threadId: 9001,
    subject: "Offsite plan",
    from: ME,
    date: new Date(Date.now() - 4 * 86_400_000).toISOString(),
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 2000,
    threadCount: 1,
  };
  return { summary, to: [ANN], cc: [], parts: [], text: "", html: "", remote: 0, trackers: 0 };
}

/** Probe shows what the hook holds, so a test can read it. */
function Probe() {
  const awaiting = useFollowUp();
  return <p data-testid="list">{(awaiting?.threads ?? []).map((t) => t.message.subject).join(",") || "none"}</p>;
}

beforeEach(() => {
  vi.useFakeTimers();
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

/** settle lets the connection and the pending replies finish. */
async function settle(times = 4): Promise<void> {
  for (let i = 0; i < times; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
  }
}

async function setup() {
  const data = mockData({ inbox: 3 });
  data.messages.push(sent());
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <Probe />
    </Session>,
  );
  await settle();
  const calls = () => mock.calls.filter((c) => c.method === "thread.awaitingOthers").length;
  return { mock, calls };
}

describe("useFollowUp", () => {
  it("asks again 1000 ms after the last maild event", async () => {
    const { mock, calls } = await setup();
    expect(screen.getByTestId("list").textContent).toBe("Offsite plan");
    expect(calls()).toBe(1);

    // VIPs changing can change which threads wait: vip.changed follows.
    await mock.call("vip.add", { addresses: [ANN.address] });
    await settle(1);
    expect(calls()).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(999);
    });
    expect(calls()).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(calls()).toBe(2);
  });

  it("keeps the last list when a refresh fails", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const { mock, calls } = await setup();
    expect(screen.getByTestId("list").textContent).toBe("Offsite plan");
    expect(calls()).toBe(1);

    let failed = 0;
    const call = mock.call.bind(mock);
    vi.spyOn(mock, "call").mockImplementation((method: string, params: unknown) => {
      if (method !== "thread.awaitingOthers") return call(method, params);
      failed++;
      return Promise.reject(new RPCError(ErrorCode.unavailable, "maild is restarting"));
    });
    await mock.call("vip.add", { addresses: [ANN.address] });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(failed).toBe(1);
    expect(screen.getByTestId("list").textContent).toBe("Offsite plan");
    expect(warn).toHaveBeenCalledWith("follow up", expect.anything());
  });
});
