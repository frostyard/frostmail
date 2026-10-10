// CONTRACT TEST for task card T-0101 (docs/tasks). Do not edit.
import { beforeEach, describe, expect, it } from "vitest";

import { listQuery, useUI } from "./stores";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

const ui = { source: { kind: "allInboxes" } as const, search: "", searchScope: "all" as const, conversations: true };

describe("listQuery with a filter", () => {
  it("adds nothing for All", () => {
    expect(listQuery({ ...ui, listFilter: "all" })).toEqual({ role: "inbox", threads: true });
    expect(listQuery(ui)).toEqual({ role: "inbox", threads: true });
  });

  it("narrows the source", () => {
    expect(listQuery({ ...ui, listFilter: "unread" })).toEqual({ role: "inbox", threads: true, unread: true });
    expect(
      listQuery({ ...ui, source: { kind: "mailbox", mailboxId: 4 }, conversations: false, listFilter: "flagged" }),
    ).toEqual({ mailboxId: 4, flagged: true });
    expect(listQuery({ ...ui, listFilter: "attachments" })).toEqual({
      role: "inbox",
      threads: true,
      hasAttachments: true,
    });
  });

  it("narrows a search in either scope", () => {
    expect(listQuery({ ...ui, search: "lunch", listFilter: "unread" })).toEqual({
      text: "lunch",
      threads: true,
      unread: true,
    });
    expect(
      listQuery({ ...ui, search: "lunch", searchScope: "source", conversations: false, listFilter: "attachments" }),
    ).toEqual({ role: "inbox", text: "lunch", hasAttachments: true });
  });
});

describe("setListFilter", () => {
  it("starts at All, clears the selection and is not remembered", () => {
    expect(useUI.getState().listFilter).toBe("all");
    useUI.getState().select([3, 4], 3);
    useUI.getState().setListFilter("unread");
    const s = useUI.getState();
    expect(s.listFilter).toBe("unread");
    expect(s.selected).toEqual([]);
    expect(s.anchor).toBeNull();
    expect(localStorage.getItem("frostmail.ui") ?? "").not.toContain("listFilter");
  });

  it("keeps the filter when the source changes", () => {
    useUI.getState().setListFilter("flagged");
    useUI.getState().setSource({ kind: "mailbox", mailboxId: 2 });
    expect(useUI.getState().listFilter).toBe("flagged");
  });
});
