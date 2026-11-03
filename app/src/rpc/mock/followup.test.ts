// The mock's thread.awaitingOthers, as maild's rules make it
// (docs/specs/rpc-api.md, thread.awaitingOthers).
import { describe, expect, it } from "vitest";

import { type Address, Client, ErrorCode } from "../gen/api";
import { RPCError } from "../transport";
import { FIXTURE, mockData } from "./fixture";
import { type MockData, type MockMessage, MockTransport } from "./mock";

const ann: Address = { name: "Ann Smith", address: "ann.smith@northwind.test" };
const bob: Address = { name: "Bob Okafor", address: "bob.okafor@acme.test" };
const carol: Address = { name: "Carol Lindqvist", address: "carol@vertex.test" };
const ME: Address = { name: "Test One", address: "test1@mailtest.test" };

const DAY = 86_400_000;

interface SentSpec {
  id: number;
  daysAgo: number;
  to: Address[];
  cc?: Address[];
  threadId?: number;
  subject?: string;
  from?: Address;
  mailboxIds?: number[];
  muted?: boolean;
  draft?: boolean;
}

/** sent is a message, a sent one unless from or mailboxIds say otherwise. */
function sent(o: SentSpec): MockMessage {
  return {
    summary: {
      id: o.id,
      accountId: FIXTURE.accountId,
      mailboxIds: o.mailboxIds ?? [FIXTURE.sent],
      threadId: o.threadId ?? o.id,
      subject: o.subject ?? `Subject ${o.id}`,
      from: o.from ?? ME,
      date: new Date(Date.now() - o.daysAgo * DAY).toISOString(),
      preview: "",
      flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: o.draft ?? false, flagColor: 0 },
      hasAttachments: false,
      size: 2000,
      threadCount: 1,
      ...(o.muted ? { muted: true } : {}),
    },
    to: o.to,
    cc: o.cc ?? [],
    parts: [],
    text: "",
    html: "",
    remote: 0,
    trackers: 0,
  };
}

/** mock is a MockTransport over the fixture's account and mailboxes, holding only messages. */
function mock(messages: MockMessage[]): MockTransport {
  const base = mockData({ inbox: 0, now: new Date() });
  const data: MockData = { ...base, messages };
  return new MockTransport(data);
}

function client(messages: MockMessage[]): Client {
  return new Client(mock(messages));
}

describe("thread.awaitingOthers", () => {
  it("lists sent mail with no reply, with its recipients and days waited", async () => {
    const c = client([sent({ id: 1, daysAgo: 4, to: [ann, bob], subject: "Offsite plan", threadId: 11, cc: [carol] })]);
    const got = await c.thread.awaitingOthers({ limit: 25, waitingDays: 3 });
    expect(got.total).toBe(1);
    expect(got.threads).toHaveLength(1);
    const [thread] = got.threads;
    expect(thread?.threadId).toBe(11);
    expect(thread?.accountId).toBe(FIXTURE.accountId);
    expect(thread?.message.id).toBe(1);
    expect(thread?.message.subject).toBe("Offsite plan");
    expect(thread?.recipients.map((a) => a.address)).toEqual([ann.address, bob.address, carol.address]);
    expect(thread?.recipientCount).toBe(3);
    expect(thread?.vip).toBe(false);
    expect(thread?.waitingDays).toBe(4);
  });

  it("leaves out a thread whose newest message is a reply from someone else", async () => {
    const c = client([
      sent({ id: 1, daysAgo: 6, to: [ann], threadId: 11 }),
      sent({ id: 2, daysAgo: 5, to: [ME], threadId: 11, from: ann }),
    ]);
    expect((await c.thread.awaitingOthers({})).threads).toHaveLength(0);
    // The oldest message is not the one that decides.
    const only = client([sent({ id: 1, daysAgo: 6, to: [ann], threadId: 11 })]);
    expect((await only.thread.awaitingOthers({})).threads).toHaveLength(1);
  });

  it("leaves out drafts, muted threads and mail only in Trash or Junk", async () => {
    const c = client([
      sent({ id: 1, daysAgo: 6, to: [ann], draft: true, mailboxIds: [FIXTURE.drafts] }),
      sent({ id: 2, daysAgo: 6, to: [ann], muted: true }),
      sent({ id: 3, daysAgo: 6, to: [ann], mailboxIds: [FIXTURE.trash] }),
      sent({ id: 4, daysAgo: 6, to: [ann], mailboxIds: [FIXTURE.junk] }),
      sent({ id: 5, daysAgo: 6, to: [ME] }),
    ]);
    expect((await c.thread.awaitingOthers({})).threads).toHaveLength(0);
  });

  it("keeps mail dated between withinDays and waitingDays ago", async () => {
    const c = client([sent({ id: 1, daysAgo: 2, to: [ann] }), sent({ id: 2, daysAgo: 10, to: [bob] })]);
    const older = await c.thread.awaitingOthers({ waitingDays: 3 });
    expect(older.threads.map((t) => t.message.id)).toEqual([2]);
    const both = await c.thread.awaitingOthers({ waitingDays: 0, withinDays: 90 });
    expect(both.threads.map((t) => t.message.id)).toEqual([1, 2]);
    expect(both.threads.map((t) => t.waitingDays)).toEqual([2, 10]);
  });

  it("puts a thread with a VIP recipient first, then the newest", async () => {
    const made = mock([
      sent({ id: 1, daysAgo: 4, to: [ann] }),
      sent({ id: 2, daysAgo: 5, to: [bob] }),
      sent({ id: 3, daysAgo: 6, to: [carol] }),
    ]);
    await made.call("vip.add", { addresses: [carol.address] });
    const got = await new Client(made).thread.awaitingOthers({ waitingDays: 3 });
    expect(got.threads.map((t) => t.message.id)).toEqual([3, 1, 2]);
    expect(got.threads.map((t) => t.vip)).toEqual([true, false, false]);
  });

  it("counts every match but lists at most limit of them", async () => {
    const c = client([
      sent({ id: 1, daysAgo: 4, to: [ann] }),
      sent({ id: 2, daysAgo: 5, to: [ann] }),
      sent({ id: 3, daysAgo: 6, to: [ann] }),
      sent({ id: 4, daysAgo: 7, to: [ann] }),
    ]);
    const got = await c.thread.awaitingOthers({ limit: 2, waitingDays: 3 });
    expect(got.total).toBe(4);
    expect(got.threads.map((t) => t.message.id)).toEqual([1, 2]);
    const all = await c.thread.awaitingOthers({ waitingDays: 3 });
    expect(all.threads).toHaveLength(4);
  });

  it("refuses params out of range", async () => {
    const c = client([]);
    for (const params of [{ limit: 0 }, { limit: 26 }, { waitingDays: -1 }, { waitingDays: 31 }, { withinDays: 0 }]) {
      const err = await c.thread.awaitingOthers(params).catch((e: unknown) => e);
      expect(err).toBeInstanceOf(RPCError);
      expect((err as RPCError).code).toBe(ErrorCode.invalidParams);
    }
  });
});
