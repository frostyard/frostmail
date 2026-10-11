// CONTRACT TEST for task card T-0111 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Mailbox } from "../rpc/gen/api";
import { ACTIONS, actionsComplete, newAction, takesMailbox, withKind } from "./rules";

const mailbox = (id: number): Mailbox => ({
  id,
  accountId: 1,
  path: `M${id}`,
  name: `M${id}`,
  delimiter: "/",
  role: "none",
  total: 0,
  unread: 0,
  label: false,
});

describe("rule actions", () => {
  it("lists the actions in menu order with Mail.app's labels", () => {
    expect(ACTIONS.map((a) => [a.kind, a.label])).toEqual([
      ["move", "Move Message"],
      ["copy", "Copy Message"],
      ["read", "Mark as Read"],
      ["flag", "Mark as Flagged"],
      ["delete", "Delete Message"],
      ["notify", "Send Notification"],
      ["stop", "Stop Evaluating Rules"],
    ]);
    expect(ACTIONS.filter((a) => takesMailbox(a.kind)).map((a) => a.kind)).toEqual(["move", "copy"]);
    expect(newAction()).toEqual({ kind: "move" });
  });

  it("gives a row what its new kind takes", () => {
    expect(withKind({ kind: "move", mailboxId: 9 }, "copy")).toEqual({ kind: "copy", mailboxId: 9 });
    expect(withKind({ kind: "flag", color: 3 }, "move")).toEqual({ kind: "move" });
    expect(withKind({ kind: "read" }, "flag")).toEqual({ kind: "flag", color: 1 });
    expect(withKind({ kind: "flag", color: 3 }, "flag")).toEqual({ kind: "flag", color: 3 });
    expect(withKind({ kind: "move", mailboxId: 9 }, "stop")).toEqual({ kind: "stop" });
  });

  it("is complete when every move and copy names a mailbox that exists", () => {
    const mailboxes = [mailbox(9)];
    expect(actionsComplete([], mailboxes)).toBe(false);
    expect(actionsComplete([{ kind: "read" }, { kind: "move", mailboxId: 9 }], mailboxes)).toBe(true);
    expect(actionsComplete([{ kind: "copy" }], mailboxes)).toBe(false);
    expect(actionsComplete([{ kind: "move", mailboxId: 8 }], mailboxes)).toBe(false);
    expect(actionsComplete([{ kind: "stop" }], [])).toBe(true);
  });
});
