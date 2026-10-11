// CONTRACT TEST for task card T-0108 (docs/tasks). Do not edit.
import { beforeEach, describe, expect, it } from "vitest";

import { sourceFromKey, sourceKey, sourceQuery, useUI } from "./stores";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("smart mailboxes as sources", () => {
  it("parse from and print to smart:<id>, and list by smartMailboxId", () => {
    expect(sourceFromKey("smart:12")).toEqual({ kind: "smart", id: 12 });
    expect(sourceFromKey("smart:x")).toBeNull();
    expect(sourceKey({ kind: "smart", id: 12 })).toBe("smart:12");
    expect(sourceQuery({ kind: "smart", id: 12 }, true)).toEqual({ smartMailboxId: 12, threads: true });
    expect(sourceQuery({ kind: "smart", id: 12 }, false)).toEqual({ smartMailboxId: 12 });
  });

  it("open and close the sheet", () => {
    expect(useUI.getState().smartSheet).toBeNull();
    useUI.getState().openSmartSheet({ mode: "edit", id: 3 });
    expect(useUI.getState().smartSheet).toEqual({ mode: "edit", id: 3 });
    useUI.getState().closeSmartSheet();
    expect(useUI.getState().smartSheet).toBeNull();
  });
});
