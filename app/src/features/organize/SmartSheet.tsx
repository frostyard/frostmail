import { useEffect, useId, useRef, useState } from "react";

import { newCondition } from "../../lib/conditions";
import type { Account, Conditions, Mailbox } from "../../rpc/gen/api";
import { ALERT, BUTTON, FIELD, PRIMARY_BUTTON } from "../settings/labels";
import { ConditionEditor } from "./ConditionEditor";

/** SmartDraft holds the editable fields of a smart mailbox. */
export interface SmartDraft {
  name: string;
  conditions: Conditions;
  includeTrash: boolean;
  includeSent: boolean;
}

/** newSmartDraft supplies a new smart mailbox's starting values. */
export function newSmartDraft(): SmartDraft {
  return {
    name: "Smart Mailbox",
    conditions: { match: "all", conditions: [newCondition()] },
    includeTrash: false,
    includeSent: false,
  };
}

/** SmartSheetProps supplies the sheet's data and save callbacks. */
export interface SmartSheetProps {
  title: string;
  initial: SmartDraft;
  accounts: Account[];
  mailboxes: Mailbox[];
  flagNames?: readonly string[];
  today: string;
  busy?: boolean;
  error?: string;
  onSave: (draft: SmartDraft) => void;
  onCancel: () => void;
}

/** SmartSheet edits a smart mailbox's name, conditions and scope. */
export function SmartSheet(props: SmartSheetProps) {
  const [draft, setDraft] = useState(props.initial);
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
        className="w-[560px] max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
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
            if (draft.name.trim() && !props.busy) props.onSave({ ...draft, name: draft.name.trim() });
          }}
        >
          <label className="flex items-center gap-2 text-[13px]">
            Smart Mailbox Name:
            <input
              className={`${FIELD} min-w-0 flex-1`}
              maxLength={100}
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.currentTarget.value })}
            />
          </label>
          <ConditionEditor
            value={draft.conditions}
            accounts={props.accounts}
            mailboxes={props.mailboxes}
            flagNames={props.flagNames}
            today={props.today}
            onChange={(conditions) => setDraft({ ...draft, conditions })}
          />
          <label className="flex items-center gap-2 text-[13px]">
            <input
              type="checkbox"
              checked={draft.includeTrash}
              onChange={(event) => setDraft({ ...draft, includeTrash: event.currentTarget.checked })}
            />
            Include messages from Trash
          </label>
          <label className="flex items-center gap-2 text-[13px]">
            <input
              type="checkbox"
              checked={draft.includeSent}
              onChange={(event) => setDraft({ ...draft, includeSent: event.currentTarget.checked })}
            />
            Include messages from Sent
          </label>
          {props.error && (
            <p role="alert" className={ALERT}>
              {props.error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button type="button" className={BUTTON} onClick={props.onCancel}>
              Cancel
            </button>
            <button type="submit" className={PRIMARY_BUTTON} disabled={!draft.name.trim() || props.busy}>
              OK
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
