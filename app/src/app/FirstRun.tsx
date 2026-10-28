// The main window with no account (docs/specs/ui.md, First run): a welcome in
// place of the modules, which adds an account, and lodge as the secondary
// choice on this computer's maild (ADR-0046).
import { isTauri } from "@tauri-apps/api/core";
import { useEffect, useRef } from "react";

import { BUTTON, PRIMARY_BUTTON } from "../features/settings/labels";
import { remoteMaild } from "../rpc/transport/tauri";
import { openSettings } from "./settings";
import type { SettingsPane } from "./settingsPane";

/** onThisComputer reports whether the desktop app reads this computer's maild. */
function onThisComputer(): boolean {
  return isTauri() && !remoteMaild();
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

/** FirstRun is the welcome a store with no account shows in place of the modules. */
export function FirstRun() {
  const thisComputer = onThisComputer();
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
      </div>
    </section>
  );
}
