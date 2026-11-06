import { Sheet } from "@frostyard/lodge-ui/react";
import { useRef } from "react";
import { BUTTON, PRIMARY_BUTTON } from "../settings/labels";

/** AttachmentReminderSheetProps supplies the user's three choices. */
export interface AttachmentReminderSheetProps {
  onCancel: () => void;
  onAttach: () => void;
  onSendAnyway: () => void;
}

/** AttachmentReminderSheet asks before a message that mentions an attachment goes without one. */
export function AttachmentReminderSheet({ onCancel, onAttach, onSendAnyway }: AttachmentReminderSheetProps) {
  const attach = useRef<HTMLButtonElement>(null);
  return (
    <Sheet
      title="Send without an attachment?"
      width={360}
      className="rounded-[10px]"
      titleClassName="text-[13px] font-semibold"
      showClose={false}
      closeOnOverlay={false}
      initialFocus={attach}
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
        <p className="mb-4 text-[13px] leading-[18px]">Your message mentions an attachment, but nothing is attached.</p>
        <div className="flex justify-end gap-2">
          <button type="button" className={BUTTON} onClick={onCancel}>
            Cancel
          </button>
          <button ref={attach} type="button" className={PRIMARY_BUTTON} onClick={onAttach}>
            Attach Files…
          </button>
          <button type="button" className={BUTTON} onClick={onSendAnyway}>
            Send Anyway
          </button>
        </div>
      </div>
    </Sheet>
  );
}
