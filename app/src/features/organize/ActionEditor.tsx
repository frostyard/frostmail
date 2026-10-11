import { CircleMinus, CirclePlus } from "lucide-react";

import { flagLabel } from "../../lib/flags";
import { ACTIONS, newAction, takesMailbox, withKind } from "../../lib/rules";
import type { Account, Mailbox, RuleAction, RuleActionKind } from "../../rpc/gen/api";
import { FIELD } from "../settings/labels";

/** ActionEditorProps supplies the actions and their available destinations. */
export interface ActionEditorProps {
  value: RuleAction[];
  onChange: (value: RuleAction[]) => void;
  accounts: Account[];
  mailboxes: Mailbox[];
  flagNames?: readonly string[];
}

/** ActionEditor edits a rule's ordered actions as a controlled list. */
export function ActionEditor({ value, onChange, accounts, mailboxes, flagNames }: ActionEditorProps) {
  const update = (index: number, action: RuleAction) => onChange(value.map((old, i) => (i === index ? action : old)));
  const iconButton =
    "flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded text-secondary focus:outline-none focus:ring-2 focus:ring-focus disabled:opacity-40";
  return (
    <div className="space-y-2 text-[13px] leading-[18px]">
      <div>Perform the following actions:</div>
      {value.map((action, index) => {
        const name = `Action ${index + 1}`;
        const missingMailbox = !mailboxes.some((mailbox) => mailbox.id === action.mailboxId);
        return (
          <div key={name} className="flex items-center gap-2">
            <select
              className={FIELD}
              aria-label={name}
              value={action.kind}
              onChange={(event) => update(index, withKind(action, event.currentTarget.value as RuleActionKind))}
            >
              {ACTIONS.map(({ kind, label }) => (
                <option key={kind} value={kind}>
                  {label}
                </option>
              ))}
            </select>
            {takesMailbox(action.kind) && (
              <>
                <span>to mailbox:</span>
                <select
                  className={`${FIELD} min-w-0 flex-1`}
                  aria-label={`${name} mailbox`}
                  value={action.mailboxId ?? ""}
                  onChange={(event) =>
                    update(index, { kind: action.kind, mailboxId: Number(event.currentTarget.value) })
                  }
                >
                  {missingMailbox && (
                    <option value={action.mailboxId ?? ""} disabled>
                      {action.mailboxId === undefined ? "No Mailbox Selected" : "Missing Mailbox"}
                    </option>
                  )}
                  {accounts.map((account) => (
                    <optgroup key={account.id} label={account.email}>
                      {mailboxes
                        .filter((mailbox) => mailbox.accountId === account.id)
                        .map((mailbox) => (
                          <option key={mailbox.id} value={mailbox.id}>
                            {mailbox.path}
                          </option>
                        ))}
                    </optgroup>
                  ))}
                </select>
              </>
            )}
            {action.kind === "flag" && (
              <select
                className={FIELD}
                aria-label={`${name} color`}
                value={action.color ?? 1}
                onChange={(event) => update(index, { kind: "flag", color: Number(event.currentTarget.value) })}
              >
                {[1, 2, 3, 4, 5, 6, 7].map((color) => (
                  <option key={color} value={color}>
                    {flagLabel(color, flagNames)}
                  </option>
                ))}
              </select>
            )}
            <button
              type="button"
              className={iconButton}
              aria-label={`Remove action ${index + 1}`}
              disabled={value.length === 1}
              onClick={() => onChange(value.filter((_, i) => i !== index))}
            >
              <CircleMinus size={18} aria-hidden="true" />
            </button>
            <button
              type="button"
              className={iconButton}
              aria-label={`Add action after ${index + 1}`}
              onClick={() => onChange([...value.slice(0, index + 1), newAction(), ...value.slice(index + 1)])}
            >
              <CirclePlus size={18} aria-hidden="true" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
