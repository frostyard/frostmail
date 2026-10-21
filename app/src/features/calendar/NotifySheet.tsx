import { Sheet } from "@frostyard/lodge-ui/react";
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
  const sendButton = useRef<HTMLButtonElement>(null);
  return (
    <Sheet
      title={noticeTitle(notice.kind)}
      width={420}
      className="rounded-[10px]"
      titleClassName="text-[13px] font-semibold"
      showClose={false}
      closeOnOverlay={false}
      initialFocus={sendButton}
      onClose={onCancel}
    >
      {/* biome-ignore lint/a11y/noStaticElementInteractions: the Sheet's frame is the dialog; this wrapper only keeps Tab and the keys inside it */}
      <div
        onKeyDown={(event) => {
          if (event.key !== "Tab") return;
          const controls = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>("button"));
          const index = controls.indexOf(document.activeElement as HTMLButtonElement);
          event.preventDefault();
          controls[(index + (event.shiftKey ? controls.length - 1 : 1)) % controls.length]?.focus();
        }}
        onKeyUp={(event) => event.stopPropagation()}
      >
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
    </Sheet>
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
