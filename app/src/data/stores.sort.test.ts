// CONTRACT TEST for task card T-0122 (docs/tasks). Do not edit.
import { beforeEach, describe, expect, it } from "vitest";

import { listQuery, useUI } from "./stores";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

const base = { source: { kind: "allInboxes" } as const, search: "", searchScope: "all" as const, conversations: false };

describe("the list's sort", () => {
  it("starts by date, newest first, with no options on", () => {
    const ui = useUI.getState();
    expect([ui.sort, ui.ascending, ui.contactPhotos]).toEqual(["date", false, false]);
    expect(listQuery({ ...base, sort: "date", ascending: false })).toEqual({ role: "inbox" });
  });

  it("goes in the query unless it is the default", () => {
    expect(listQuery({ ...base, sort: "from", ascending: true })).toEqual({
      role: "inbox",
      sort: "from",
      ascending: true,
    });
    expect(listQuery({ ...base, sort: "size", ascending: false })).toEqual({ role: "inbox", sort: "size" });
    expect(listQuery({ ...base, sort: "date", ascending: true })).toEqual({
      role: "inbox",
      sort: "date",
      ascending: true,
    });
    expect(listQuery({ ...base, search: "plan", sort: "subject", ascending: true })).toEqual({
      text: "plan",
      sort: "subject",
      ascending: true,
    });
  });

  it("takes each field's usual direction", () => {
    const ui = useUI.getState();
    for (const [sort, ascending] of [
      ["from", true],
      ["to", true],
      ["subject", true],
      ["date", false],
      ["size", false],
      ["flags", false],
      ["unread", false],
      ["attachments", false],
    ] as const) {
      ui.setSort(sort);
      expect([useUI.getState().sort, useUI.getState().ascending]).toEqual([sort, ascending]);
    }
    ui.setAscending(true);
    ui.setConversations(false);
    ui.setContactPhotos(true);
    expect(useUI.getState()).toMatchObject({ ascending: true, conversations: false, contactPhotos: true });
  });
});
