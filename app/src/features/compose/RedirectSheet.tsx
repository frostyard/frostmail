import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";

import { isValidAddress } from "../../lib/addressParse";
import type { Address } from "../../rpc/gen/api";
import { BUTTON, PRIMARY_BUTTON } from "../settings/labels";
import { RecipientField } from "./RecipientField";

/** RedirectSheetProps supplies the message subject, recipients and sheet callbacks. */
export interface RedirectSheetProps {
  subject: string;
  suggest: (prefix: string) => Promise<Address[]>;
  onRedirect: (to: Address[]) => Promise<void>;
  onCancel: () => void;
}

function sheetKeyDown(event: KeyboardEvent<HTMLDivElement>, onCancel: () => void) {
  event.stopPropagation();
  if (event.key === "Escape") {
    event.preventDefault();
    if (!(event.target instanceof HTMLInputElement && event.target.getAttribute("aria-expanded") === "true")) {
      onCancel();
    }
  } else if (event.key === "Tab" && !event.defaultPrevented) {
    const controls = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("input, button")).filter(
      (control) => !control.hasAttribute("disabled") && control.tabIndex >= 0,
    );
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
}

/** RedirectSheet asks whom to send the original message to. */
export function RedirectSheet(props: RedirectSheetProps) {
  const [to, setTo] = useState<Address[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const valid = to.length > 0 && to.every((address) => isValidAddress(address.address));
  useEffect(() => {
    const previous = document.activeElement;
    dialog.current?.querySelector<HTMLInputElement>("input")?.focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);

  async function redirect() {
    if (!valid || pending) return;
    setPending(true);
    setError(null);
    try {
      await props.onRedirect(to);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-window/60">
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="w-[420px] max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
        onKeyDown={(event) => sheetKeyDown(event, props.onCancel)}
      >
        <h2 id={titleId} className="text-[13px] font-semibold">
          Redirect
        </h2>
        <p className="mt-1 mb-4 text-[12px] leading-4 text-secondary">{props.subject.trim() || "(no subject)"}</p>
        <div className="flex items-center gap-2 text-[13px]">
          <span>To</span>
          <RecipientField label="To" value={to} onChange={setTo} suggest={props.suggest} />
        </div>
        {error !== null && (
          <p role="alert" className="mt-2 text-[12px] leading-4 text-flag-1">
            {error}
          </p>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button type="button" className={BUTTON} onClick={props.onCancel}>
            Cancel
          </button>
          <button type="button" className={PRIMARY_BUTTON} disabled={!valid || pending} onClick={() => void redirect()}>
            Redirect
          </button>
        </div>
      </div>
    </div>
  );
}
