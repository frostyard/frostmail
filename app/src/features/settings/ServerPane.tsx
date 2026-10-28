// The Server pane of the desktop app's settings window
// (docs/design/desktop.md, On a maild behind lodge): the maild on this
// computer, or the one behind lodge at an address. The choice is the app's
// own, kept beside it, never maild's. In the Flatpak it is a mode
// (ADR-0046): saving it stops or starts this computer's maild, and the pane
// says what that does before the user saves, from local facts alone.
import { useEffect, useState } from "react";

import { ALERT, FIELD, GRID, LABEL, PRIMARY_BUTTON } from "./labels";

/** NOTE_TO_LODGE is what saving lodge as the mode does. */
const NOTE_TO_LODGE =
  "This computer's maild stops. Its accounts and mail stay on this computer, not synced, until you switch back. Nothing moves to lodge.";

/** NOTE_TO_LOCAL_DATA is what saving this computer's maild does when it holds data. */
const NOTE_TO_LOCAL_DATA =
  "This computer's maild starts again with the accounts it had. It runs their rules, Send Later and reminders here. If lodge's maild has the same accounts, both run them.";

/** NOTE_TO_LOCAL_EMPTY is what saving this computer's maild does without data. */
const NOTE_TO_LOCAL_EMPTY =
  "Mail on lodge keeps running there. If you add the same accounts here, both run their rules, Send Later and reminders.";

/** STOPPING is the status while a save moves the mode to lodge. */
export const STOPPING = "Stopping this computer's maild…";

/** STARTING is the status while a save moves the mode to this computer. */
export const STARTING = "Starting this computer's maild…";

/**
 * saveStatus is the status a save shows while it runs, fixed when it starts:
 * the pane remounts during a change of mode, with the new choice already
 * stored, so it cannot tell the change from its props then.
 */
export function saveStatus(flatpak: boolean, from: string | null, to: string | null): string | null {
  if (!flatpak) return null;
  if (from === null && to !== null) return STOPPING;
  if (from !== null && to === null) return STARTING;
  return null;
}

/** ServerPaneProps are the Server pane's inputs. */
export interface ServerPaneProps {
  /** lodge's address, or null for the maild on this computer. */
  lodge: string | null;
  /** The Flatpak: a change of mode stops or starts its maild. */
  flatpak: boolean;
  /** This computer's data directory holds a database. */
  localData: boolean;
  busy: boolean;
  /** What a running save does (saveStatus), shown while busy. */
  status: string | null;
  error: string | null;
  /** Saves the choice; the app's windows then reconnect. */
  onSave: (lodge: string | null) => void;
}

/** ServerPane chooses which maild the app asks. */
export function ServerPane({ lodge, flatpak, localData, busy, status, error, onSave }: ServerPaneProps) {
  const [remote, setRemote] = useState(lodge !== null);
  const [address, setAddress] = useState(lodge ?? "");
  useEffect(() => {
    setRemote(lodge !== null);
    setAddress(lodge ?? "");
  }, [lodge]);

  const chosen = remote ? address.trim() : null;
  // What saving changes, from these props alone, so the note says the same
  // while lodge is unreachable. A new lodge address keeps the mode.
  const toLodge = remote && chosen !== null && lodge === null;
  const toLocal = !remote && lodge !== null;
  const note = !flatpak
    ? null
    : toLodge
      ? NOTE_TO_LODGE
      : toLocal
        ? localData
          ? NOTE_TO_LOCAL_DATA
          : NOTE_TO_LOCAL_EMPTY
        : null;
  const disabled = busy || (remote && chosen === "") || chosen === lodge;

  function submit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    onSave(chosen);
  }

  return (
    <form aria-label="Server" className="flex max-w-[560px] flex-col px-6 py-5" onSubmit={submit}>
      <h2 className="mb-4 text-[15px] leading-5 font-semibold">Server</h2>
      <p className="mb-4 text-[12px] leading-4 text-secondary">
        Frostmail reads your mail through maild: the one on this computer, or the one behind lodge on your tailnet,
        which your phone and browsers use too.
      </p>
      <fieldset className="flex flex-col gap-2">
        <label className="flex items-center gap-2 text-[13px]">
          <input type="radio" name="server" checked={!remote} onChange={() => setRemote(false)} />
          maild on this computer
        </label>
        <label className="flex items-center gap-2 text-[13px]">
          <input type="radio" name="server" checked={remote} onChange={() => setRemote(true)} />
          maild behind lodge
        </label>
      </fieldset>
      {remote ? (
        <div className={`${GRID} mt-3`}>
          <label className={LABEL} htmlFor="server-lodge">
            lodge:
          </label>
          <input
            id="server-lodge"
            className={FIELD}
            type="url"
            autoComplete="off"
            spellCheck={false}
            placeholder="https://lodge.example.ts.net"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
          />
        </div>
      ) : null}
      {note ? (
        <p role="note" className="mt-3 text-[12px] leading-4 text-secondary">
          {note}
        </p>
      ) : null}
      {busy && status ? (
        <p role="status" className="mt-3 text-[12px] leading-4 text-secondary">
          {status}
        </p>
      ) : null}
      {error ? (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      ) : null}
      <button type="submit" className={`${PRIMARY_BUTTON} mt-5 self-end`} disabled={disabled}>
        Save
      </button>
    </form>
  );
}
