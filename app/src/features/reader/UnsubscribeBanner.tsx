import { useEffect, useId, useRef, useState } from "react";

import type { Unsubscribe, UnsubscribeMethod } from "../../rpc/gen/api";
import { BUTTON, PRIMARY_BUTTON } from "../settings/labels";

/** UnsubscribeBannerProps supplies the list, first method and action state. */
export interface UnsubscribeBannerProps {
  info: Unsubscribe;
  method?: UnsubscribeMethod;
  busy: boolean;
  failed: boolean;
  onUnsubscribe: () => void;
}

/** UnsubscribeBanner offers to leave a list after confirmation. */
export function UnsubscribeBanner(props: UnsubscribeBannerProps) {
  const [confirming, setConfirming] = useState(false);
  const { info, method, busy, failed } = props;
  if (info.done) {
    return (
      <div role="status" className="flex items-center gap-2 bg-banner px-5 py-2 text-[12px]">
        {`You unsubscribed from ${info.list}.`}
      </div>
    );
  }
  if (info.methods.length === 0) return null;
  return (
    <>
      <div role="status" className="flex items-center gap-2 bg-banner px-5 py-2 text-[12px]">
        <span>{`This message is from the mailing list ${info.list}.${failed ? " Couldn't unsubscribe." : ""}`}</span>
        <button
          type="button"
          className="ml-auto h-6 shrink-0 whitespace-nowrap rounded border border-separator bg-window px-2 disabled:opacity-40"
          disabled={busy || !method}
          onClick={() => setConfirming(true)}
        >
          {busy ? "Unsubscribing…" : "Unsubscribe"}
        </button>
      </div>
      {confirming && method && (
        <UnsubscribeDialog
          info={info}
          method={method}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            setConfirming(false);
            props.onUnsubscribe();
          }}
        />
      )}
    </>
  );
}

function UnsubscribeDialog(props: {
  info: Unsubscribe;
  method: UnsubscribeMethod;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const titleId = useId();
  const confirm = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const previous = document.activeElement;
    confirm.current?.focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-window/60">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="w-[360px] max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            props.onCancel();
          } else if (event.key === "Tab") {
            const buttons = event.currentTarget.querySelectorAll("button");
            const target = event.shiftKey ? buttons[0] : buttons[1];
            if (document.activeElement === target) {
              event.preventDefault();
              (event.shiftKey ? buttons[1] : buttons[0])?.focus();
            }
          }
        }}
        onKeyUp={(event) => event.stopPropagation()}
      >
        <h2 id={titleId} className="mb-4 text-[13px] font-semibold">{`Unsubscribe from ${props.info.list}?`}</h2>
        <p className="mb-4 text-[13px]">{methodSentence(props.info, props.method)}</p>
        <div className="flex justify-end gap-2">
          <button type="button" className={BUTTON} onClick={props.onCancel}>
            Cancel
          </button>
          <button ref={confirm} type="button" className={PRIMARY_BUTTON} onClick={props.onConfirm}>
            Unsubscribe
          </button>
        </div>
      </div>
    </div>
  );
}

function methodSentence(info: Unsubscribe, method: UnsubscribeMethod): string {
  if (method === "oneclick") return `Frostmail will ask ${info.host} to take you off the list.`;
  if (method === "mail") return `Frostmail will send a message to ${info.address} asking to take you off the list.`;
  return `The list's page on ${hostOf(info.url)} will open in your browser.`;
}

function hostOf(url: string | undefined): string {
  try {
    return new URL(url ?? "").host;
  } catch {
    return url ?? "";
  }
}
