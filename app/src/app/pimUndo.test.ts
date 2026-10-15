import { beforeEach, describe, expect, it, vi } from "vitest";

import { Client } from "../rpc/gen/api";
import { mockData } from "../rpc/mock/fixture";
import { MockTransport } from "../rpc/mock/mock";
import { announceRestore, rememberRestore, undoPim, usePimUndo, watchRestores } from "./pimUndo";
import { UNDO_LIMIT } from "./undo";

beforeEach(() => {
  usePimUndo.setState(usePimUndo.getInitialState());
});

describe("pimUndo", () => {
  it("keeps each module's restores, at most the undo limit", () => {
    for (let i = 0; i < UNDO_LIMIT + 2; i++) rememberRestore({ module: "calendar", restoreId: `r${i}` });
    rememberRestore({ module: "people", restoreId: "p1" });
    rememberRestore({ module: "people", restoreId: "" });
    const state = usePimUndo.getState();
    expect(state.calendar).toHaveLength(UNDO_LIMIT);
    expect(state.calendar.at(0)).toBe("r2");
    expect(state.people).toEqual(["p1"]);
  });

  it("restores the module's newest write, once", async () => {
    const mock = new MockTransport(mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") }));
    const client = new Client(mock);
    const w = await client.calendar.update({ id: 302, fields: { summary: "Review" } });
    rememberRestore({ module: "calendar", restoreId: w.restoreId });
    expect(await undoPim(client, "people")).toBe(false);
    expect(await undoPim(client, "calendar")).toBe(true);
    expect((await client.calendar.event({ id: 302 })).summary).toBe("Design review");
    expect(await undoPim(client, "calendar")).toBe(false);
    expect(mock.calls.filter((c) => c.method === "calendar.restore")).toHaveLength(1);
  });

  it("takes restores another window announces", async () => {
    const stop = watchRestores();
    await announceRestore({ module: "calendar", restoreId: "r-from-editor" });
    await vi.waitFor(() => expect(usePimUndo.getState().calendar).toEqual(["r-from-editor"]));
    stop();
  });
});
