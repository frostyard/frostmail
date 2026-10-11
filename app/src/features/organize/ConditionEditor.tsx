import { CircleMinus, CirclePlus } from "lucide-react";
import { FIELDS, newCondition, OP_LABEL, opsFor, ROLES, valueKind, withField, withOp } from "../../lib/conditions";
import { flagLabel } from "../../lib/flags";
import type { Account, Condition, ConditionField, ConditionOp, Conditions, Mailbox } from "../../rpc/gen/api";
import { FIELD } from "../settings/labels";

export interface ConditionEditorProps {
  value: Conditions;
  onChange: (value: Conditions) => void;
  accounts: Account[];
  mailboxes: Mailbox[];
  flagNames?: readonly string[];
  today: string;
}

type ChoiceContext = Pick<ConditionEditorProps, "accounts" | "mailboxes" | "flagNames">;

function ChoiceOptions({ field, accounts, mailboxes, flagNames }: ChoiceContext & { field: ConditionField }) {
  if (field === "mailbox") {
    return accounts.map((account) => (
      <optgroup key={account.id} label={account.email}>
        {mailboxes
          .filter((mailbox) => mailbox.accountId === account.id)
          .map((mailbox) => (
            <option key={mailbox.id} value={mailbox.id}>
              {mailbox.path}
            </option>
          ))}
      </optgroup>
    ));
  }
  const options: readonly (readonly [string, string])[] =
    field === "account"
      ? accounts.map((account) => [String(account.id), account.email] as const)
      : field === "role"
        ? ROLES
        : [1, 2, 3, 4, 5, 6, 7].map((color) => [String(color), flagLabel(color, flagNames)] as const);
  return options.map(([value, label]) => (
    <option key={value} value={value}>
      {label}
    </option>
  ));
}

function ConditionValue({
  condition,
  name,
  onChange,
  ...context
}: ChoiceContext & { condition: Condition; name: string; onChange: (value: string) => void }) {
  const kind = valueKind(condition.field, condition.op);
  if (kind === "none") return null;
  if (kind === "yesno") {
    return (
      <select
        className={FIELD}
        aria-label={`${name} value`}
        value={condition.value}
        onChange={(event) => onChange(event.currentTarget.value)}
      >
        <option value="true">Yes</option>
        <option value="false">No</option>
      </select>
    );
  }
  if (kind === "choice" || kind === "choices") {
    const multiple = kind === "choices";
    return (
      <select
        className={`${FIELD} min-w-0 flex-1 ${multiple ? "h-auto" : ""}`}
        aria-label={`${name} value`}
        multiple={multiple}
        size={multiple ? 4 : undefined}
        value={multiple ? condition.value.split(",") : condition.value}
        onChange={(event) =>
          onChange(
            multiple
              ? Array.from(event.currentTarget.selectedOptions, (option) => option.value).join(",")
              : event.currentTarget.value,
          )
        }
      >
        <ChoiceOptions field={condition.field} {...context} />
      </select>
    );
  }
  if (kind === "duration") {
    const parts = /^(.*?)([dwmy])$/.exec(condition.value);
    const number = parts?.[1] ?? condition.value;
    const unit = parts?.[2] ?? "d";
    return (
      <>
        <input
          className={`${FIELD} w-20`}
          aria-label={`${name} value`}
          type="number"
          min={1}
          max={999}
          value={number}
          onChange={(event) => onChange(`${event.currentTarget.value}${unit}`)}
        />
        <select
          className={FIELD}
          aria-label={`${name} unit`}
          value={unit}
          onChange={(event) => onChange(`${number}${event.currentTarget.value}`)}
        >
          <option value="d">Days</option>
          <option value="w">Weeks</option>
          <option value="m">Months</option>
          <option value="y">Years</option>
        </select>
      </>
    );
  }
  return (
    <input
      className={`${FIELD} min-w-0 flex-1`}
      aria-label={`${name} value`}
      type={kind === "date" ? "date" : "text"}
      value={condition.value}
      onChange={(event) => onChange(event.currentTarget.value)}
    />
  );
}

export function ConditionEditor({ value, onChange, accounts, mailboxes, flagNames, today }: ConditionEditorProps) {
  const context = { accounts, mailboxes, today };
  const update = (index: number, condition: Condition) =>
    onChange({
      ...value,
      conditions: value.conditions.map((old, i) => (i === index ? condition : old)),
    });
  const iconButton =
    "flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded text-secondary focus:outline-none focus:ring-2 focus:ring-focus disabled:opacity-40";
  return (
    <div className="space-y-2 text-[13px] leading-[18px]">
      <div className="flex flex-wrap items-center gap-1">
        Contains messages that match
        <select
          className={FIELD}
          aria-label="Match"
          value={value.match}
          onChange={(event) => onChange({ ...value, match: event.currentTarget.value as Conditions["match"] })}
        >
          <option value="all">all</option>
          <option value="any">any</option>
        </select>
        of the following conditions:
      </div>
      {value.conditions.map((condition, index) => {
        const name = `Condition ${index + 1}`;
        return (
          // biome-ignore lint/suspicious/noArrayIndexKey: Conditions have only positional identity in this controlled editor.
          <div key={index} className="flex items-center gap-2">
            <select
              className={`${FIELD} min-w-0`}
              aria-label={`${name} field`}
              value={condition.field}
              onChange={(event) =>
                update(index, withField(condition, event.currentTarget.value as ConditionField, context))
              }
            >
              {FIELDS.map(({ field, label }) => (
                <option key={field} value={field}>
                  {label}
                </option>
              ))}
            </select>
            {valueKind(condition.field, condition.op) !== "yesno" && (
              <select
                className={`${FIELD} min-w-0`}
                aria-label={`${name} operator`}
                value={condition.op}
                onChange={(event) =>
                  update(index, withOp(condition, event.currentTarget.value as ConditionOp, context))
                }
              >
                {opsFor(condition.field).map((op) => (
                  <option key={op} value={op}>
                    {OP_LABEL[op]}
                  </option>
                ))}
              </select>
            )}
            <ConditionValue
              condition={condition}
              name={name}
              accounts={accounts}
              mailboxes={mailboxes}
              flagNames={flagNames}
              onChange={(next) => update(index, { ...condition, value: next })}
            />
            <button
              type="button"
              className={iconButton}
              aria-label={`Remove condition ${index + 1}`}
              disabled={value.conditions.length === 1}
              onClick={() => onChange({ ...value, conditions: value.conditions.filter((_, i) => i !== index) })}
            >
              <CircleMinus size={18} aria-hidden="true" />
            </button>
            <button
              type="button"
              className={iconButton}
              aria-label={`Add condition after ${index + 1}`}
              onClick={() =>
                onChange({
                  ...value,
                  conditions: [
                    ...value.conditions.slice(0, index + 1),
                    newCondition(),
                    ...value.conditions.slice(index + 1),
                  ],
                })
              }
            >
              <CirclePlus size={18} aria-hidden="true" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
