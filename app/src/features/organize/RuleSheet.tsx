import { Sheet } from "@frostyard/lodge-ui/react";
import { useEffect, useId, useRef, useState } from "react";
import { newCondition } from "../../lib/conditions";
import { actionsComplete, newAction } from "../../lib/rules";
import type { Account, Conditions, Mailbox, RuleAction } from "../../rpc/gen/api";
import { ALERT, BUTTON, FIELD, PRIMARY_BUTTON } from "../settings/labels";
import { ActionEditor } from "./ActionEditor";
import { ConditionEditor } from "./ConditionEditor";

/** RuleDraft holds the editable fields of a rule. */
export interface RuleDraft {
  name: string;
  conditions: Conditions;
  actions: RuleAction[];
}

/** newRuleDraft supplies a new rule's starting values. */
export function newRuleDraft(n: number): RuleDraft {
  return {
    name: `Rule ${n}`,
    conditions: { match: "any", conditions: [newCondition()] },
    actions: [newAction()],
  };
}

/** RuleSheetProps supplies the sheet's data and save callbacks. */
export interface RuleSheetProps {
  title: string;
  initial: RuleDraft;
  accounts: Account[];
  mailboxes: Mailbox[];
  flagNames?: readonly string[];
  today: string;
  busy?: boolean;
  error?: string;
  onSave: (draft: RuleDraft) => void;
  onCancel: () => void;
}

/** RuleSheet edits a rule's description, conditions and actions. */
export function RuleSheet(props: RuleSheetProps) {
  const [draft, setDraft] = useState(props.initial);
  const canSave = Boolean(draft.name.trim()) && actionsComplete(draft.actions, props.mailboxes) && !props.busy;
  return (
    <Sheet
      title={props.title}
      width={640}
      className="rounded-[10px]"
      titleClassName="text-[13px] font-semibold"
      showClose={false}
      closeOnOverlay={false}
      initialFocus={"first"}
      onClose={props.onCancel}
    >
      {/* biome-ignore lint/a11y/noStaticElementInteractions: the Sheet's frame is the dialog; this wrapper only keeps Tab and the keys inside it */}
      <div
        onKeyDown={(event) => {
          if (event.key !== "Tab") return;
          const controls = Array.from(
            event.currentTarget.querySelectorAll<HTMLElement>("input, select, button"),
          ).filter((control) => !control.hasAttribute("disabled"));
          const first = controls[0];
          const last = controls.at(-1);
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last?.focus();
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first?.focus();
          }
        }}
      >
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (canSave) props.onSave({ ...draft, name: draft.name.trim() });
          }}
        >
          <label className="flex items-center gap-2 text-[13px]">
            Description:
            <input
              className={`${FIELD} min-w-0 flex-1`}
              maxLength={100}
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })}
            />
          </label>
          <ConditionEditor
            lead="If"
            trail="of the following conditions are met:"
            value={draft.conditions}
            accounts={props.accounts}
            mailboxes={props.mailboxes}
            flagNames={props.flagNames}
            today={props.today}
            onChange={(conditions) => setDraft({ ...draft, conditions })}
          />
          <ActionEditor
            value={draft.actions}
            accounts={props.accounts}
            mailboxes={props.mailboxes}
            flagNames={props.flagNames}
            onChange={(actions) => setDraft({ ...draft, actions })}
          />
          {props.error && (
            <p role="alert" className={ALERT}>
              {props.error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button type="button" className={BUTTON} onClick={props.onCancel}>
              Cancel
            </button>
            <button type="submit" className={PRIMARY_BUTTON} disabled={!canSave}>
              OK
            </button>
          </div>
        </form>
      </div>
    </Sheet>
  );
}
