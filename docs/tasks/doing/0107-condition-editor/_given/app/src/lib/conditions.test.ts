// CONTRACT TEST for task card T-0107 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import type { Account, Condition, Mailbox } from "../rpc/gen/api";
import {
  type ConditionContext,
  FIELDS,
  newCondition,
  OP_LABEL,
  opsFor,
  startingValue,
  valueKind,
  withField,
  withOp,
} from "./conditions";

const account = (id: number, email: string): Account => ({
  id,
  kind: "imap",
  email,
  displayName: "",
  auth: "password",
  imap: { host: "h", port: 993, tls: "tls", username: email },
  smtp: { host: "h", port: 465, tls: "tls", username: email },
  createdAt: "2026-10-01T00:00:00Z",
  readOnly: false,
  notify: true,
  syncDays: 0,
  signedIn: true,
});

const mailbox = (id: number, accountId: number, path: string): Mailbox => ({
  id,
  accountId,
  path,
  name: path,
  delimiter: "/",
  role: path === "INBOX" ? "inbox" : "none",
  total: 0,
  unread: 0,
  label: false,
});

const ctx: ConditionContext = {
  today: "2026-10-11",
  accounts: [account(3, "me@x.test"), account(4, "you@y.test")],
  mailboxes: [mailbox(10, 3, "INBOX"), mailbox(11, 3, "Lists"), mailbox(20, 4, "INBOX")],
};

describe("condition fields and operators", () => {
  it("lists the fields in menu order with their labels", () => {
    expect(FIELDS.map((f) => f.field)).toEqual([
      "from",
      "to",
      "cc",
      "recipient",
      "tome",
      "ccme",
      "subject",
      "content",
      "filename",
      "listid",
      "account",
      "mailbox",
      "role",
      "received",
      "sent",
      "unread",
      "flagged",
      "attachments",
      "color",
      "vip",
      "contact",
      "reminder",
    ]);
    expect(FIELDS.slice(0, 4).map((f) => f.label)).toEqual(["From", "To", "Cc", "Any Recipient"]);
    expect(FIELDS.find((f) => f.field === "received")?.label).toBe("Date Received");
    expect(FIELDS.find((f) => f.field === "vip")?.label).toBe("Sender Is a VIP");
  });

  it("gives each field its operators", () => {
    expect(opsFor("from")).toEqual(["contains", "notcontains", "is", "begins", "ends"]);
    expect(opsFor("to")).toEqual(["contains", "is", "begins", "ends"]);
    expect(opsFor("content")).toEqual(["contains", "notcontains"]);
    expect(opsFor("listid")).toEqual(["contains", "is"]);
    expect(opsFor("mailbox")).toEqual(["is", "isnot", "anyof"]);
    expect(opsFor("received")).toEqual([
      "today",
      "yesterday",
      "thisweek",
      "thismonth",
      "thisyear",
      "within",
      "notwithin",
      "on",
      "since",
      "before",
    ]);
    expect(opsFor("unread")).toEqual(["is"]);
    expect([OP_LABEL.notcontains, OP_LABEL.within, OP_LABEL.since, OP_LABEL.anyof]).toEqual([
      "does not contain",
      "is in the last",
      "is on or after",
      "is any of",
    ]);
  });

  it("knows what kind of value each takes", () => {
    expect(valueKind("from", "contains")).toBe("text");
    expect(valueKind("unread", "is")).toBe("yesno");
    expect(valueKind("account", "is")).toBe("choice");
    expect(valueKind("role", "anyof")).toBe("choices");
    expect(valueKind("received", "within")).toBe("duration");
    expect(valueKind("sent", "before")).toBe("date");
    expect(valueKind("received", "today")).toBe("none");
  });

  it("starts each kind of value", () => {
    expect(startingValue("subject", "is", ctx)).toBe("");
    expect(startingValue("unread", "is", ctx)).toBe("true");
    expect(startingValue("account", "is", ctx)).toBe("3");
    expect(startingValue("mailbox", "anyof", ctx)).toBe("10");
    expect(startingValue("role", "is", ctx)).toBe("inbox");
    expect(startingValue("color", "isnot", ctx)).toBe("1");
    expect(startingValue("received", "within", ctx)).toBe("7d");
    expect(startingValue("received", "on", ctx)).toBe("2026-10-11");
    expect(startingValue("received", "thisweek", ctx)).toBe("");
    expect(newCondition()).toEqual({ field: "from", op: "contains", value: "" });
  });

  it("changes a field or an operator", () => {
    const c: Condition = { field: "from", op: "contains", value: "ann" };
    expect(withField(c, "received", ctx)).toEqual({ field: "received", op: "today", value: "" });
    expect(withField(c, "flagged", ctx)).toEqual({ field: "flagged", op: "is", value: "true" });
    expect(withOp(c, "begins", ctx)).toEqual({ field: "from", op: "begins", value: "ann" });
    expect(withOp({ field: "received", op: "today", value: "" }, "within", ctx)).toEqual({
      field: "received",
      op: "within",
      value: "7d",
    });
    expect(withOp({ field: "received", op: "within", value: "3w" }, "notwithin", ctx).value).toBe("3w");
    expect(withOp({ field: "role", op: "is", value: "sent" }, "isnot", ctx).value).toBe("sent");
    expect(withOp({ field: "role", op: "is", value: "sent" }, "anyof", ctx).value).toBe("sent");
  });
});
