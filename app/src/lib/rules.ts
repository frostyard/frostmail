import type { Mailbox, RuleAction, RuleActionKind } from "../rpc/gen/api";

/** ACTIONS lists the rule actions in menu order. */
export const ACTIONS: { kind: RuleActionKind; label: string }[] = [
  { kind: "move", label: "Move Message" },
  { kind: "copy", label: "Copy Message" },
  { kind: "read", label: "Mark as Read" },
  { kind: "flag", label: "Mark as Flagged" },
  { kind: "delete", label: "Delete Message" },
  { kind: "notify", label: "Send Notification" },
  { kind: "stop", label: "Stop Evaluating Rules" },
];

/** takesMailbox reports whether an action needs a destination. */
export function takesMailbox(kind: RuleActionKind): boolean {
  return kind === "move" || kind === "copy";
}

/** newAction supplies a new row's starting action. */
export function newAction(): RuleAction {
  return { kind: "move" };
}

/** withKind retains only the parameters the new kind takes. */
export function withKind(action: RuleAction, kind: RuleActionKind): RuleAction {
  if (takesMailbox(kind) && takesMailbox(action.kind) && action.mailboxId !== undefined) {
    return { kind, mailboxId: action.mailboxId };
  }
  if (kind === "flag") {
    return { kind, color: action.kind === "flag" ? (action.color ?? 1) : 1 };
  }
  return { kind };
}

/** actionsComplete checks that every destination exists and the list is nonempty. */
export function actionsComplete(actions: RuleAction[], mailboxes: Mailbox[]): boolean {
  return (
    actions.length > 0 &&
    actions.every(
      (action) => !takesMailbox(action.kind) || mailboxes.some((mailbox) => mailbox.id === action.mailboxId),
    )
  );
}
