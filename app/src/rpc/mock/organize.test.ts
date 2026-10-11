import { describe, expect, it } from "vitest";

import type { Event, MessageSummary, PersonSummary, Settings, ViewCount, ViewInfo, Vip } from "../gen/api";
import { ErrorCode } from "../gen/api";
import { mockData } from "./fixture";
import { MockTransport } from "./mock";

function setup() {
  const data = mockData({ inbox: 10, now: new Date("2026-10-08T12:00:00Z") });
  const mock = new MockTransport(data);
  const events: Event[] = [];
  mock.onEvent((e) => events.push(e));
  return { data, mock, events };
}

async function rows(mock: MockTransport, view: ViewInfo): Promise<MessageSummary[]> {
  return mock.call<MessageSummary[]>("view.range", { id: view.id, start: 0, end: view.count + 10 });
}

describe("MockTransport settings", () => {
  it("starts with maild's defaults and keeps what is set", async () => {
    const { mock, events } = setup();
    expect(await mock.call<Settings>("settings.get", {})).toEqual({
      undoDelay: 10,
      notifyScope: "inbox",
      flagNames: ["", "", "", "", "", "", ""],
    });
    const s = await mock.call<Settings>("settings.set", {
      undoDelay: 30,
      flagNames: [" Urgent ", "", "", "", "", "", ""],
    });
    expect(s.undoDelay).toBe(30);
    expect(s.flagNames[0]).toBe("Urgent");
    expect(events.some((e) => e.event === "settings.changed")).toBe(true);
    expect((await mock.call<Settings>("settings.set", { notifyScope: "vips" })).undoDelay).toBe(30);
  });

  it("refuses what maild refuses", async () => {
    const { mock } = setup();
    await expect(mock.call("settings.set", { undoDelay: 15 })).rejects.toMatchObject({ code: ErrorCode.invalidParams });
    await expect(mock.call("settings.set", { flagNames: ["a"] })).rejects.toMatchObject({
      code: ErrorCode.invalidParams,
    });
  });
});

describe("MockTransport VIPs", () => {
  it("adds addresses and a person's addresses, named from People", async () => {
    const { mock, events } = setup();
    const people = await mock.call<PersonSummary[]>("people.list", { query: "ann" });
    const ann = people.find((p) => p.displayName === "Ann Smith");
    if (!ann) throw new Error("no Ann in the fixture");
    const vips = await mock.call<Vip[]>("vip.add", { personId: ann.id, addresses: ["Kofi.Okafor@acme.test"] });
    expect(vips.map((v) => v.address)).toEqual([
      "ann.smith@northwind.test",
      "ann@smith-family.test",
      "kofi.okafor@acme.test",
    ]);
    expect(vips[0]).toMatchObject({ name: "Ann Smith", personId: ann.id });
    expect(vips[2]).toMatchObject({ name: "Kofi Okafor" });
    expect(events.filter((e) => e.event === "vip.changed")).toHaveLength(1);
    await mock.call("vip.remove", { personId: ann.id });
    expect((await mock.call<Vip[]>("vip.list", {})).map((v) => v.address)).toEqual(["kofi.okafor@acme.test"]);
  });

  it("refuses an empty request and an unknown person", async () => {
    const { mock } = setup();
    await expect(mock.call("vip.add", {})).rejects.toMatchObject({ code: ErrorCode.invalidParams });
    await expect(mock.call("vip.add", { personId: 99999 })).rejects.toMatchObject({ code: ErrorCode.notFound });
  });
});

describe("MockTransport conditions and counts", () => {
  it("lists a flag color, VIP mail and one sender, live as VIPs change", async () => {
    const { data, mock } = setup();
    const red = await mock.call<ViewInfo>("view.open", {
      query: { conditions: { match: "all", conditions: [{ field: "color", op: "is", value: "1" }] } },
    });
    const wantRed = data.messages.filter((m) => m.summary.flags.flagged && m.summary.flags.flagColor === 1);
    expect(wantRed.length).toBeGreaterThan(0);
    expect(red.count).toBe(wantRed.length);

    const vipQuery = { conditions: { match: "all", conditions: [{ field: "vip", op: "is", value: "true" }] } };
    const vip = await mock.call<ViewInfo>("view.open", { query: vipQuery });
    expect(vip.count).toBe(0);
    await mock.call("vip.add", { addresses: ["ann.smith@northwind.test"] });
    const fromAnn = data.messages.filter((m) => m.summary.from.address === "ann.smith@northwind.test");
    expect((await rows(mock, vip)).map((r) => r.id).sort()).toEqual(fromAnn.map((m) => m.summary.id).sort());

    const any = await mock.call<ViewInfo>("view.open", {
      query: {
        conditions: {
          match: "any",
          conditions: [
            { field: "from", op: "is", value: "ANN.SMITH@northwind.test" },
            { field: "from", op: "is", value: "kofi.okafor@acme.test" },
          ],
        },
      },
    });
    const fromEither = data.messages.filter((m) =>
      ["ann.smith@northwind.test", "kofi.okafor@acme.test"].includes(m.summary.from.address),
    );
    expect(any.count).toBe(fromEither.length);
  });

  it("counts each query's messages and unread ones", async () => {
    const { data, mock } = setup();
    const counts = await mock.call<ViewCount[]>("view.count", { queries: [{ flagged: true }, { threads: true }] });
    const flagged = data.messages.filter((m) => m.summary.flags.flagged);
    expect(counts[0]).toEqual({
      total: flagged.length,
      unread: flagged.filter((m) => !m.summary.flags.seen).length,
    });
    expect(counts[1]?.total).toBe(data.messages.length);
  });
});
