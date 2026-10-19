// The Server pane of the desktop app's settings window
// (docs/design/desktop.md, On a maild behind lodge): the maild on this
// computer, or the one behind lodge at an address. The choice is the app's
// own, kept beside it, never maild's.
import { useEffect, useState } from "react";

import { ALERT, FIELD, GRID, LABEL, PRIMARY_BUTTON } from "./labels";

/** ServerPaneProps are the Server pane's inputs. */
export interface ServerPaneProps {
  /** lodge's address, or null for the maild on this computer. */
  lodge: string | null;
  busy: boolean;
  error: string | null;
  /** Saves the choice; the app's windows then reconnect. */
  onSave: (lodge: string | null) => void;
}

/** ServerPane chooses which maild the app asks. */
export function ServerPane({ lodge, busy, error, onSave }: ServerPaneProps) {
  const [remote, setRemote] = useState(lodge !== null);
  const [address, setAddress] = useState(lodge ?? "");
  useEffect(() => {
    setRemote(lodge !== null);
    setAddress(lodge ?? "");
  }, [lodge]);

  const chosen = remote ? address.trim() : null;
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
