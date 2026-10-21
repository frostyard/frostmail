import { Sheet } from "@frostyard/lodge-ui/react";
import { useEffect, useId, useRef } from "react";
import { BUTTON } from "../settings/labels";

/** RawSourceSheetProps supplies the source and the sheet's callbacks. */
export interface RawSourceSheetProps {
  text: string;
  truncated: boolean;
  onClose: () => void;
  /** Save As: the whole message, however much of it the sheet shows. */
  onSave?: () => void;
}

/** RawSourceSheet shows the message as the server holds it. */
export function RawSourceSheet({ text, truncated, onClose, onSave }: RawSourceSheetProps) {
  const close = useRef<HTMLButtonElement>(null);
  const saveAs = useRef<HTMLButtonElement>(null);
  return (
    <div className="print:hidden">
      <Sheet
        title="Raw Source"
        width={720}
        className="max-w-full rounded-[10px]"
        titleClassName="text-[13px] font-semibold"
        showClose={false}
        closeOnOverlay={false}
        initialFocus={close}
        onClose={onClose}
      >
        {/* biome-ignore lint/a11y/noStaticElementInteractions: the Sheet's frame is the dialog; this wrapper only keeps Tab and the keys inside it */}
        <div
          onKeyDown={(event) => {
            if (event.key !== "Tab") return;
            // Focus stays on the sheet's buttons.
            event.preventDefault();
            const next = document.activeElement === close.current && saveAs.current ? saveAs : close;
            next.current?.focus();
          }}
        >
          <pre className="h-[480px] overflow-auto select-text font-mono text-[12px] leading-[18px]">{text}</pre>
          {truncated && <p className="mt-3 text-[12px] text-secondary">The message is longer than what is shown.</p>}
          <div className="mt-4 flex justify-end gap-2">
            {onSave && (
              <button ref={saveAs} type="button" className={BUTTON} onClick={onSave}>
                Save As…
              </button>
            )}
            <button ref={close} type="button" className={BUTTON} onClick={onClose}>
              Close
            </button>
          </div>
        </div>
      </Sheet>
    </div>
  );
}
