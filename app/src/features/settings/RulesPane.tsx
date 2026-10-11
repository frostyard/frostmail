import { ArrowDown, ArrowUp, TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";

import type { Rule } from "../../rpc/gen/api";
import { ALERT, BUTTON } from "./labels";

/** RulesPaneProps supplies maild's ordered rules and the pane's actions. */
export interface RulesPaneProps {
  rules: Rule[];
  error?: string;
  onToggle: (rule: Rule, enabled: boolean) => void;
  onAdd: () => void;
  onEdit: (rule: Rule) => void;
  onDuplicate: (rule: Rule) => void;
  onRemove: (rule: Rule) => void;
  onMove: (rule: Rule, position: number) => void;
}

/** RulesPane lists rules in execution order and keeps the selected rule. */
export function RulesPane(props: RulesPaneProps) {
  const [selected, setSelected] = useState<number | null>(null);
  const index = props.rules.findIndex((rule) => rule.id === selected);
  const rule = props.rules[index];
  useEffect(() => {
    if (index < 0) setSelected(null);
  }, [index]);
  return (
    <div className="flex h-full flex-col p-6">
      <ul aria-label="Rules" className="min-h-0 flex-1 overflow-y-auto rounded-md border border-separator">
        {props.rules.length === 0 && (
          <li className="flex h-7 items-center justify-center text-[13px] text-secondary">No Rules</li>
        )}
        {props.rules.map((item) => (
          <li
            key={item.id}
            className={`flex h-7 items-center gap-2 px-2 ${item.id === selected ? "bg-selection-inactive" : ""}`}
          >
            <input
              type="checkbox"
              aria-label={`Enable ${item.name}`}
              checked={item.enabled}
              onChange={(event) => props.onToggle(item, event.currentTarget.checked)}
            />
            <button
              type="button"
              aria-pressed={item.id === selected}
              className={`h-full min-w-0 flex-1 truncate text-left text-[13px] leading-[18px] ${item.enabled ? "text-primary" : "text-secondary"}`}
              onClick={() => setSelected(item.id)}
              onDoubleClick={() => props.onEdit(item)}
            >
              {item.name}
            </button>
            {item.problem && (
              <span role="img" aria-label={item.problem} title={item.problem} className="shrink-0 text-secondary">
                <TriangleAlert size={14} aria-hidden="true" />
              </span>
            )}
          </li>
        ))}
      </ul>
      <div className="mt-2 flex gap-2">
        <button type="button" className={BUTTON} onClick={props.onAdd}>
          Add Rule
        </button>
        <button type="button" className={BUTTON} disabled={!rule} onClick={() => rule && props.onEdit(rule)}>
          Edit
        </button>
        <button type="button" className={BUTTON} disabled={!rule} onClick={() => rule && props.onDuplicate(rule)}>
          Duplicate
        </button>
        <button type="button" className={BUTTON} disabled={!rule} onClick={() => rule && props.onRemove(rule)}>
          Remove
        </button>
        <div className="flex-1" />
        <button
          type="button"
          className={`${BUTTON} flex items-center gap-1`}
          disabled={!rule || index === 0}
          onClick={() => rule && props.onMove(rule, index - 1)}
        >
          <ArrowUp size={14} aria-hidden="true" />
          Move Up
        </button>
        <button
          type="button"
          className={`${BUTTON} flex items-center gap-1`}
          disabled={!rule || index === props.rules.length - 1}
          onClick={() => rule && props.onMove(rule, index + 1)}
        >
          <ArrowDown size={14} aria-hidden="true" />
          Move Down
        </button>
      </div>
      {props.error && (
        <p role="alert" className={ALERT}>
          {props.error}
        </p>
      )}
    </div>
  );
}
