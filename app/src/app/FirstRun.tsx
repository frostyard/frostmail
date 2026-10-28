// The main window with no account (docs/specs/ui.md, First run): a welcome in
// place of the modules, which adds an account, and lodge as the secondary
// choice on this computer's maild (ADR-0046). In the Flatpak, below its
// buttons, it also shows the move note when a host install's mail is here.
import { invoke, isTauri } from "@tauri-apps/api/core";
import { useEffect, useRef, useState } from "react";

import { BUTTON, PRIMARY_BUTTON } from "../features/settings/labels";
import { inFlatpak, remoteMaild } from "../rpc/transport/tauri";
import { openSettings } from "./settings";
import type { SettingsPane } from "./settingsPane";

/** MOVE_NOTE is the window's own key for the move note's dismissal. */
const MOVE_NOTE = "frostmail.moveNote";

/** MOVE_NOTE_DISMISSED is the value that keeps the note away on this computer. */
const MOVE_NOTE_DISMISSED = "dismissed";

/** MOVE_STEPS are the commands that move a host install's mail here, in order
 * (ADR-0046, Moving from a host maild). They are the user's to run: the app
 * never reads, copies or stops the host's maild. */
const MOVE_STEPS = [
  "systemctl --user disable --now maild.service",
  "mailctl export ~/frostmail-export",
  "flatpak run --command=mailctl org.frostyard.Frostmail import ~/frostmail-export",
];

/** MOVE_FOUND is what the move note says about the host install's mail. */
const MOVE_FOUND =
  "This computer also has mail from a Frostmail installed outside Flatpak. Nothing moves by itself. To move it here, run these commands in a terminal:";

/** MOVE_SECRETS is what the move note says about the export folder. */
const MOVE_SECRETS =
  "Then open Frostmail again. The folder ~/frostmail-export holds your passwords in plain text: delete it once your accounts sync.";

/** onThisComputer reports whether the desktop app reads this computer's maild. */
function onThisComputer(): boolean {
  return isTauri() && !remoteMaild();
}

/** moveNoteDismissed reports whether this computer dismissed the move note. */
function moveNoteDismissed(): boolean {
  return localStorage.getItem(MOVE_NOTE) === MOVE_NOTE_DISMISSED;
}

function showSettings(pane: SettingsPane) {
  void openSettings(pane).catch((err: unknown) => console.warn("settings", err));
}

/**
 * useOpenSettingsOnFirstRun opens the settings window on Accounts itself when
 * the main window, as it mounts on a ready connection, finds no account. The
 * decision is made once, on that first render, whether the store is empty or
 * not: removing the last account later shows the welcome again without
 * opening the window. The main window mounts again on each new connection.
 */
export function useOpenSettingsOnFirstRun(empty: boolean): void {
  const decided = useRef(false);
  useEffect(() => {
    if (decided.current) return;
    decided.current = true;
    if (empty && isTauri()) showSettings("accounts");
  }, [empty]);
}

/**
 * MoveNote says how to move a host install's mail into the Flatpak
 * (docs/specs/ui.md, First run, the move note). It changes nothing: the move
 * is the three commands, run by the user. Dismiss keeps that choice in the
 * window's localStorage, so the note does not return on this computer.
 */
function MoveNote() {
  const [hidden, setHidden] = useState(false);
  if (hidden) return null;
  const dismiss = () => {
    localStorage.setItem(MOVE_NOTE, MOVE_NOTE_DISMISSED);
    setHidden(true);
  };
  return (
    <div
      role="note"
      aria-label="Mail from another install"
      className="mt-6 rounded-[6px] bg-banner p-3 text-[12px] leading-4"
    >
      <p>{MOVE_FOUND}</p>
      <ol className="mt-2 list-inside list-decimal">
        {MOVE_STEPS.map((step) => (
          <li key={step}>
            <code className="font-mono select-text">{step}</code>
          </li>
        ))}
      </ol>
      <p className="mt-2">{MOVE_SECRETS}</p>
      <button type="button" className={`${BUTTON} mt-3`} onClick={dismiss}>
        Dismiss
      </button>
    </div>
  );
}

/** FirstRun is the welcome a store with no account shows in place of the modules. */
export function FirstRun() {
  const thisComputer = onThisComputer();
  // The move note is the Flatpak's: elsewhere the app does not ask about a
  // host install (docs/specs/ui.md, First run, the move note).
  const flatpak = thisComputer && inFlatpak();
  const [hostMail, setHostMail] = useState(false);
  const asked = useRef(false);
  // The shell answers whether another install's mail is here, once. A shell
  // that cannot answer, and any answer other than true, shows no note.
  useEffect(() => {
    if (asked.current || !flatpak) return;
    asked.current = true;
    void invoke<boolean>("host_mail_found")
      .then((found) => setHostMail(found === true))
      .catch(() => setHostMail(false));
  }, [flatpak]);
  const text = thisComputer
    ? "Add a mail account to start. Frostmail keeps its mail on this computer."
    : "Add a mail account to start.";
  return (
    <section aria-label="Welcome" className="flex min-h-0 flex-1 items-center justify-center bg-window px-6">
      <div className="w-full max-w-[420px]">
        <h1 className="text-[17px] leading-[22px] font-semibold">Welcome to Frostmail</h1>
        <p className="mt-2 text-[13px] leading-[18px] text-secondary">{text}</p>
        <div className="mt-5 flex items-center gap-2">
          <button type="button" className={PRIMARY_BUTTON} onClick={() => showSettings("accounts")}>
            Add Account…
          </button>
          {thisComputer && (
            <button type="button" className={BUTTON} onClick={() => showSettings("server")}>
              Connect to lodge…
            </button>
          )}
        </div>
        {flatpak && hostMail && !moveNoteDismissed() && <MoveNote />}
      </div>
    </section>
  );
}
