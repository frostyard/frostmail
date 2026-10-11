// Maild's general preferences (docs/specs/settings-ui.md).
import { Flag } from "lucide-react";
import { useEffect, useId, useState } from "react";

import { flagName } from "../../lib/flags";
import type { NotifyScope, Settings, SettingsSetParams, SmartMailbox } from "../../rpc/gen/api";
import { ALERT, FIELD, GRID, LABEL } from "./labels";

/** GeneralPaneProps supplies preferences and saves changes through its container. */
export interface GeneralPaneProps {
  settings: Settings | null;
  smarts?: SmartMailbox[];
  error?: string;
  onChange: (change: SettingsSetParams) => void;
}

const FLAG_CLASSES = [
  "text-flag-1",
  "text-flag-2",
  "text-flag-3",
  "text-flag-4",
  "text-flag-5",
  "text-flag-6",
  "text-flag-7",
] as const;

/** GeneralPane edits notifications, undo send and the seven flag names. */
export function GeneralPane({ settings, smarts = [], error, onChange }: GeneralPaneProps) {
  const id = useId();
  const flagNames = settings?.flagNames;
  const [names, setNames] = useState(() => FLAG_CLASSES.map((_, i) => flagNames?.[i] ?? ""));
  useEffect(() => {
    setNames(FLAG_CLASSES.map((_, i) => flagNames?.[i] ?? ""));
  }, [flagNames]);
  const saveNames = () => {
    const trimmed = names.map((name) => name.trim());
    if (settings && trimmed.some((name, i) => name !== settings.flagNames[i])) onChange({ flagNames: trimmed });
  };
  return (
    <div className="h-full overflow-y-auto p-6">
      <div className={GRID}>
        <label htmlFor={`${id}-scope`} className={LABEL}>
          New message notifications:
        </label>
        <select
          id={`${id}-scope`}
          className={FIELD}
          disabled={!settings}
          value={
            settings?.notifyScope === "smart" ? `smart:${settings.notifySmartId}` : (settings?.notifyScope ?? "inbox")
          }
          onChange={(event) => {
            const value = event.target.value;
            onChange(
              value.startsWith("smart:")
                ? { notifyScope: "smart", notifySmartId: Number(value.slice(6)) }
                : { notifyScope: value as NotifyScope },
            );
          }}
        >
          <option value="inbox">Inbox Only</option>
          <option value="vips">VIPs</option>
          <option value="contacts">Contacts</option>
          <option value="all">All Mailboxes</option>
          {smarts.length > 0 && (
            <optgroup label="Smart Mailboxes">
              {smarts.map((smart) => (
                <option key={smart.id} value={`smart:${smart.id}`}>
                  {smart.name}
                </option>
              ))}
            </optgroup>
          )}
        </select>
        <label htmlFor={`${id}-delay`} className={LABEL}>
          Undo send delay:
        </label>
        <select
          id={`${id}-delay`}
          className={FIELD}
          disabled={!settings}
          value={settings?.undoDelay ?? 10}
          onChange={(event) => onChange({ undoDelay: Number(event.target.value) })}
        >
          <option value="0">Off</option>
          <option value="10">10 Seconds</option>
          <option value="20">20 Seconds</option>
          <option value="30">30 Seconds</option>
        </select>
        <span className={`${LABEL} self-start pt-1`}>Flag names:</span>
        <div className="flex flex-col gap-2">
          {FLAG_CLASSES.map((className, i) => (
            <div key={className} className="flex items-center gap-2">
              <Flag size={12} fill="currentColor" className={className} aria-hidden="true" />
              <input
                className={`${FIELD} min-w-0 flex-1`}
                aria-label={`Flag ${i + 1} name`}
                disabled={!settings}
                maxLength={40}
                placeholder={flagName(i + 1)}
                value={names[i] ?? ""}
                onChange={(event) => setNames(names.map((name, index) => (index === i ? event.target.value : name)))}
                onBlur={saveNames}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    saveNames();
                  }
                }}
              />
            </div>
          ))}
        </div>
      </div>
      {error && (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      )}
    </div>
  );
}
