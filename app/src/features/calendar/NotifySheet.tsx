import { useCallback, useEffect, useId, useRef, useState } from "react";

import { noticeText, noticeTitle } from "../../lib/notify";
import type { InviteNotice } from "../../rpc/gen/api";
import { BUTTON, PRIMARY_BUTTON } from "../settings/labels";

/** NotifySheetProps supplies the notice and the user's choice callbacks. */
export interface NotifySheetProps {
  notice: InviteNotice;
  onSend: () => void;
  onCancel: () => void;
}

/** NotifySheet asks before a calendar write tells invitees by email. */
export function NotifySheet({ notice, onSend, onCancel }: NotifySheetProps) {
  const titleId = useId();
  const sendButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const previous = document.activeElement;
    sendButton.current?.focus();
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
        className="w-[420px] max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            onCancel();
          } else if (event.key === "Tab") {
            const controls = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>("button"));
            const index = controls.indexOf(document.activeElement as HTMLButtonElement);
            event.preventDefault();
            controls[(index + (event.shiftKey ? controls.length - 1 : 1)) % controls.length]?.focus();
          }
        }}
        onKeyUp={(event) => event.stopPropagation()}
      >
        <h2 id={titleId} className="mb-4 text-[13px] font-semibold">
          {noticeTitle(notice.kind)}
        </h2>
        <p className="mb-4 text-[13px] leading-[18px]">{noticeText(notice.count)}</p>
        <div className="flex justify-end gap-2">
          <button type="button" className={BUTTON} onClick={onCancel}>
            Cancel
          </button>
          <button ref={sendButton} type="button" className={PRIMARY_BUTTON} onClick={onSend}>
            Send
          </button>
        </div>
      </div>
    </div>
  );
}

/** useNotify shows a sheet and resolves its question with the user's choice. */
export function useNotify() {
  const [pending, setPending] = useState<{
    notice: InviteNotice;
    resolve: (send: boolean) => void;
  } | null>(null);
  const ask = useCallback(
    (notice: InviteNotice) => new Promise<boolean>((resolve) => setPending({ notice, resolve })),
    [],
  );
  const answer = (send: boolean) => {
    pending?.resolve(send);
    setPending(null);
  };
  const sheet = pending ? (
    <NotifySheet notice={pending.notice} onSend={() => answer(true)} onCancel={() => answer(false)} />
  ) : null;
  return { ask, sheet };
}
