// Maild's general preferences (docs/specs/settings-ui.md).
import { Flag } from "lucide-react";
import { useEffect, useId, useState } from "react";

import { flagName } from "../../lib/flags";
import type {
  BlockAction,
  BlockedSender,
  NotifyScope,
  Settings,
  SettingsSetParams,
  SmartMailbox,
} from "../../rpc/gen/api";
import { ALERT, BUTTON, FIELD, GRID, LABEL } from "./labels";

/** GeneralPaneProps supplies preferences and saves changes through its container. */
export interface GeneralPaneProps {
  settings: Settings | null;
  smarts?: SmartMailbox[];
  blocked?: BlockedSender[];
  error?: string;
  onChange: (change: SettingsSetParams) => void;
  onUnblock?: (address: string) => void;
  /** In a browser: whether it may show notifications, and asking it to (docs/specs/lodge-app.md). */
  browser?: { permission: NotificationPermission; onAllow: () => void };
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

/** GeneralPane edits notifications, undo send, flag names and blocked senders. */
export function GeneralPane({
  settings,
  smarts = [],
  blocked = [],
  error,
  onChange,
  onUnblock,
  browser,
}: GeneralPaneProps) {
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
    <div className="h-full overflow-y-auto p-6 narrow:p-4">
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
        {browser && (
          <>
            <span className={LABEL}>In this browser:</span>
            {browser.permission === "default" ? (
              <button type="button" className={`${BUTTON} justify-self-start`} onClick={browser.onAllow}>
                Allow Notifications…
              </button>
            ) : (
              <span className="text-[13px] leading-[18px] text-secondary">
                {browser.permission === "granted"
                  ? "Notifications are on."
                  : "Notifications are blocked in the browser's settings."}
              </span>
            )}
          </>
        )}
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
        <label htmlFor={`${id}-block-action`} className={LABEL}>
          Blocked mail:
        </label>
        <select
          id={`${id}-block-action`}
          className={FIELD}
          disabled={!settings}
          value={settings?.blockAction ?? "mark"}
          onChange={(event) => onChange({ blockAction: event.target.value as BlockAction })}
        >
          <option value="mark">Mark it, and leave it where it arrives</option>
          <option value="trash">Move it to the Trash</option>
        </select>
        <span className={`${LABEL} self-start pt-1`}>Blocked senders:</span>
        {blocked.length > 0 ? (
          <ul aria-label="Blocked senders" className="flex flex-col gap-2">
            {blocked.map((sender) => (
              <li key={sender.address} className="flex items-center gap-2">
                <div className="min-w-0 flex-1">
                  {sender.name && <span className="text-[13px] leading-[18px]">{sender.name}</span>}
                  <span className="block text-[12px] leading-4 text-secondary">{sender.address}</span>
                </div>
                <button
                  type="button"
                  className={BUTTON}
                  aria-label={`Unblock ${sender.address}`}
                  onClick={() => onUnblock?.(sender.address)}
                >
                  Unblock
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <span className="text-secondary">None</span>
        )}
      </div>
      {error && (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      )}
    </div>
  );
}
