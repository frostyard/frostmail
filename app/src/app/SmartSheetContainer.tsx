import { useState } from "react";

import { useClient } from "../data/session";
import { type SmartSheetState, useMail, useUI } from "../data/stores";
import { newSmartDraft, type SmartDraft, SmartSheet } from "../features/organize/SmartSheet";

function Sheet({ state, initial }: { state: SmartSheetState; initial: SmartDraft }) {
  const client = useClient();
  const { accounts, mailboxes, settings } = useMail();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const date = new Date();
  const today = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
  const save = async (draft: SmartDraft) => {
    if (busy) return;
    setBusy(true);
    setError(undefined);
    try {
      const smart =
        state.mode === "new" ? await client.smart.create(draft) : await client.smart.update({ id: state.id, ...draft });
      const ui = useUI.getState();
      if (ui.smartSheet === state) {
        ui.closeSmartSheet();
        if (state.mode === "new") ui.setSource({ kind: "smart", id: smart.id });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <SmartSheet
      title={state.mode === "new" ? "New Smart Mailbox" : "Edit Smart Mailbox"}
      initial={initial}
      accounts={accounts}
      mailboxes={mailboxes}
      flagNames={settings?.flagNames}
      today={today}
      busy={busy}
      error={error}
      onSave={(draft) => void save(draft)}
      onCancel={() => useUI.getState().closeSmartSheet()}
    />
  );
}

/** SmartSheetContainer connects the open smart mailbox sheet to maild. */
export function SmartSheetContainer() {
  const state = useUI((ui) => ui.smartSheet);
  const smarts = useMail((mail) => mail.smarts);
  if (!state) return null;
  const smart = state.mode === "edit" ? smarts.find((smart) => smart.id === state.id) : undefined;
  if (state.mode === "edit" && !smart) return null;
  const initial = state.mode === "new" ? (state.initial ?? newSmartDraft()) : smart;
  if (!initial) return null;
  return (
    <Sheet
      key={state.mode === "new" ? `new:${JSON.stringify(state.initial)}` : `edit:${state.id}`}
      state={state}
      initial={initial}
    />
  );
}
