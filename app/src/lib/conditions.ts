import type { Account, Condition, ConditionField, ConditionOp, Mailbox } from "../rpc/gen/api";

export interface FieldInfo {
  field: ConditionField;
  label: string;
}

export const FIELDS: FieldInfo[] = [
  { field: "from", label: "From" },
  { field: "to", label: "To" },
  { field: "cc", label: "Cc" },
  { field: "recipient", label: "Any Recipient" },
  { field: "tome", label: "Sent to Me" },
  { field: "ccme", label: "Cc'd to Me" },
  { field: "subject", label: "Subject" },
  { field: "content", label: "Entire Message" },
  { field: "filename", label: "Attachment Name" },
  { field: "listid", label: "Mailing List" },
  { field: "account", label: "Account" },
  { field: "mailbox", label: "Mailbox" },
  { field: "role", label: "Mailbox Type" },
  { field: "received", label: "Date Received" },
  { field: "sent", label: "Date Sent" },
  { field: "unread", label: "Unread" },
  { field: "flagged", label: "Flagged" },
  { field: "attachments", label: "Has Attachments" },
  { field: "color", label: "Flag Color" },
  { field: "vip", label: "Sender Is a VIP" },
  { field: "contact", label: "Sender Is in Contacts" },
  { field: "reminder", label: "Has a Reminder" },
];

export const OP_LABEL: Record<ConditionOp, string> = {
  contains: "contains",
  notcontains: "does not contain",
  is: "is",
  isnot: "is not",
  begins: "begins with",
  ends: "ends with",
  anyof: "is any of",
  today: "is today",
  yesterday: "is yesterday",
  thisweek: "is this week",
  thismonth: "is this month",
  thisyear: "is this year",
  within: "is in the last",
  notwithin: "is not in the last",
  on: "is",
  since: "is on or after",
  before: "is before",
};

export function opsFor(field: ConditionField): ConditionOp[] {
  switch (field) {
    case "from":
    case "subject":
      return ["contains", "notcontains", "is", "begins", "ends"];
    case "to":
    case "cc":
      return ["contains", "is", "begins", "ends"];
    case "recipient":
    case "content":
    case "filename":
      return ["contains", "notcontains"];
    case "listid":
      return ["contains", "is"];
    case "account":
    case "mailbox":
    case "role":
    case "color":
      return ["is", "isnot", "anyof"];
    case "received":
    case "sent":
      return [
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
      ];
    default:
      return ["is"];
  }
}

export type ValueKind = "text" | "yesno" | "choice" | "choices" | "duration" | "date" | "none";

export function valueKind(field: ConditionField, op: ConditionOp): ValueKind {
  if (["tome", "ccme", "unread", "flagged", "attachments", "vip", "contact", "reminder"].includes(field)) {
    return "yesno";
  }
  if (["account", "mailbox", "role", "color"].includes(field)) {
    if (op === "anyof") return "choices";
    if (op === "is" || op === "isnot") return "choice";
  }
  if (op === "within" || op === "notwithin") return "duration";
  if (op === "on" || op === "since" || op === "before") return "date";
  if (["today", "yesterday", "thisweek", "thismonth", "thisyear"].includes(op)) return "none";
  return "text";
}

export interface ConditionContext {
  today: string;
  accounts: Account[];
  mailboxes: Mailbox[];
}

export function startingValue(field: ConditionField, op: ConditionOp, ctx: ConditionContext): string {
  switch (valueKind(field, op)) {
    case "yesno":
      return "true";
    case "duration":
      return "7d";
    case "date":
      return ctx.today;
    case "choice":
    case "choices":
      switch (field) {
        case "account":
          return ctx.accounts[0]?.id.toString() ?? "";
        case "mailbox":
          return ctx.mailboxes[0]?.id.toString() ?? "";
        case "role":
          return "inbox";
        case "color":
          return "1";
        default:
          return "";
      }
    default:
      return "";
  }
}

export const ROLES: readonly (readonly [string, string])[] = [
  ["inbox", "Inbox"],
  ["drafts", "Drafts"],
  ["sent", "Sent"],
  ["junk", "Junk"],
  ["trash", "Trash"],
  ["archive", "Archive"],
  ["all", "All Mail"],
];

export function newCondition(): Condition {
  return { field: "from", op: "contains", value: "" };
}

export function withField(_c: Condition, field: ConditionField, ctx: ConditionContext): Condition {
  const op = opsFor(field)[0] ?? "is";
  return { field, op, value: startingValue(field, op, ctx) };
}

export function withOp(c: Condition, op: ConditionOp, ctx: ConditionContext): Condition {
  const oldKind = valueKind(c.field, c.op);
  const newKind = valueKind(c.field, op);
  const choice = (kind: ValueKind) => kind === "choice" || kind === "choices";
  let value = startingValue(c.field, op, ctx);
  if (oldKind === newKind || (choice(oldKind) && choice(newKind))) {
    value = newKind === "choice" ? (c.value.split(",")[0] ?? "") : c.value;
  }
  return { ...c, op, value };
}
