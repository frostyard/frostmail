import { describe, expect, it } from "vitest";

import { Client, type Event, type MessageSummary } from "../rpc/gen/api";
import { FIXTURE, mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import type { Transport } from "../rpc/transport";
import { ViewModel } from "./view";

const settle = () => new Promise((r) => setTimeout(r, 0));

function setup(inbox = 60) {
  const mock = new MockTransport(mockData({ inbox, now: new Date("2026-10-07T12:00:00Z") }));
  return { mock, client: new Client(mock) };
}

describe("ViewModel with MockTransport", () => {
  it("opens the view and loads the requested window", async () => {
    const { client } = setup(250);
    const vm = new ViewModel(client, { mailboxId: FIXTURE.inbox });
    vm.ensure(0, 30);
    await settle();
    await settle();
    expect(vm.ready).toBe(true);
    expect(vm.count).toBe(250);
    expect(vm.row(0)).toBeDefined();
    expect(vm.row(99)).toBeDefined();
    expect(vm.row(100)).toBeUndefined();
    vm.ensure(180, 220);
    await settle();
    expect(vm.row(150)).toBeDefined();
    expect(vm.row(249)).toBeDefined();
  });

  it("shifts rows on inserts and fetches the new row", async () => {
    const { mock, client } = setup();
    const vm = new ViewModel(client, { mailboxId: FIXTURE.inbox });
    vm.ensure(0, 20);
    await settle();
    await settle();
    const first = vm.row(0);
    const copy = mock.message(first?.id ?? 0);
    if (!first || !copy) throw new Error("no first row");
    const fresh: MessageSummary = { ...first, id: 9999, subject: "Brand new", date: "2026-10-07T12:00:00Z" };
    mock.add([{ ...copy, summary: fresh }]);
    await settle();
    await settle();
    expect(vm.count).toBe(61);
    expect(vm.row(0)?.subject).toBe("Brand new");
    expect(vm.row(1)?.id).toBe(first.id);
  });

  it("refreshes rows named by message.changed", async () => {
    const { client } = setup();
    const vm = new ViewModel(client, { mailboxId: FIXTURE.inbox });
    vm.ensure(0, 20);
    await settle();
    await settle();
    const row = vm.row(2);
    if (!row) throw new Error("no row");
    await client.message.setFlags({ ids: [row.id], changes: { flagColor: 4 } });
    await settle();
    await settle();
    expect(vm.row(vm.indexOf(row.id))?.flags.flagColor).toBe(4);
  });

  it("removes rows and closes the view on close", async () => {
    const { mock, client } = setup();
    const vm = new ViewModel(client, { mailboxId: FIXTURE.inbox });
    vm.ensure(0, 20);
    await settle();
    await settle();
    const gone = vm.row(0)?.id ?? 0;
    mock.remove([gone]);
    await settle();
    await settle();
    expect(vm.count).toBe(59);
    expect(vm.indexOf(gone)).toBe(-1);
    vm.close();
    await settle();
    expect(mock.calls.some((c) => c.method === "view.close")).toBe(true);
  });
});

// A transport whose replies the test releases by hand.
class ManualTransport implements Transport {
  readonly pending: { method: string; params: unknown; resolve: (v: unknown) => void }[] = [];
  private handler: ((e: Event) => void) | null = null;
  call<T>(method: string, params: unknown): Promise<T> {
    return new Promise<T>((resolve) => {
      this.pending.push({ method, params, resolve: resolve as (v: unknown) => void });
    });
  }
  onEvent(h: (e: Event) => void) {
    this.handler = h;
    return () => {};
  }
  onClose() {
    return () => {};
  }
  send(e: Event) {
    this.handler?.(e);
  }
  take(method: string) {
    const i = this.pending.findIndex((p) => p.method === method);
    const [p] = this.pending.splice(i, 1);
    if (!p) throw new Error(`no pending ${method}`);
    return p;
  }
}

const row = (id: number) => ({ id, subject: `m${id}` }) as MessageSummary;

describe("ViewModel ordering", () => {
  it("drops a range reply that a delta overtook, and asks again", async () => {
    const t = new ManualTransport();
    const vm = new ViewModel(new Client(t), {});
    vm.ensure(0, 10);
    t.take("view.open").resolve({ id: 7, count: 3 });
    await settle();
    const stale = t.take("view.range");
    t.send({ event: "view.delta", data: { id: 7, count: 4, ops: [{ op: "insert", at: 0, count: 1 }] } });
    stale.resolve([row(1), row(2), row(3)]);
    await settle();
    expect(vm.row(0)).toBeUndefined();
    t.take("view.range").resolve([row(4), row(1), row(2), row(3)]);
    await settle();
    expect([0, 1, 2, 3].map((i) => vm.row(i)?.id)).toEqual([4, 1, 2, 3]);
  });

  it("applies a delta that arrived before the view.open reply", async () => {
    const t = new ManualTransport();
    const vm = new ViewModel(new Client(t), {});
    vm.ensure(0, 10);
    const open = t.take("view.open");
    t.send({ event: "view.delta", data: { id: 3, count: 1, ops: [{ op: "remove", at: 1, count: 1 }] } });
    open.resolve({ id: 3, count: 2 });
    await settle();
    expect(vm.count).toBe(1);
  });
});
