import { useEffect, useId, useRef } from "react";

import { BUTTON } from "../settings/labels";

/** RawSourceSheetProps supplies the source and the sheet's close callback. */
export interface RawSourceSheetProps {
  text: string;
  truncated: boolean;
  onClose: () => void;
}

/** RawSourceSheet shows the message as the server holds it. */
export function RawSourceSheet({ text, truncated, onClose }: RawSourceSheetProps) {
  const titleId = useId();
  const close = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const previous = document.activeElement;
    close.current?.focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-window/60 print:hidden">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="w-[720px] max-w-full max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (event.key === "Escape") {
            event.preventDefault();
            onClose();
          } else if (event.key === "Tab") {
            event.preventDefault();
            close.current?.focus();
          }
        }}
      >
        <h2 id={titleId} className="mb-4 text-[13px] font-semibold">
          Raw Source
        </h2>
        <pre className="h-[480px] overflow-auto select-text font-mono text-[12px] leading-[18px]">{text}</pre>
        {truncated && <p className="mt-3 text-[12px] text-secondary">The message is longer than what is shown.</p>}
        <div className="mt-4 flex justify-end">
          <button ref={close} type="button" className={BUTTON} onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
