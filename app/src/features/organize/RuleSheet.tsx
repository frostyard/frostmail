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
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previous = document.activeElement;
    dialog.current?.querySelector<HTMLInputElement>("input")?.focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-window/60">
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="w-[640px] max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            props.onCancel();
          } else if (event.key === "Tab") {
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
          }
        }}
      >
        <h2 id={titleId} className="mb-4 text-[13px] font-semibold">
          {props.title}
        </h2>
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
    </div>
  );
}
