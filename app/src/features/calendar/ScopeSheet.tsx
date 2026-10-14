import { useEffect, useId, useRef } from "react";

import type { EventScope } from "../../rpc/gen/api";
import { BUTTON, PRIMARY_BUTTON } from "../settings/labels";

/** ScopeSheetProps supplies the action, occurrence and choice callbacks. */
export interface ScopeSheetProps {
  action: "change" | "delete";
  first: boolean;
  onChoose: (scope: EventScope) => void;
  onCancel: () => void;
}

/** ScopeSheet asks which occurrences a change or deletion affects. */
export function ScopeSheet({ action, first, onChoose, onCancel }: ScopeSheetProps) {
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const verb = action === "change" ? "Change" : "Delete";
  useEffect(() => {
    const previous = document.activeElement;
    dialog.current?.querySelector<HTMLButtonElement>("button")?.focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-window/60">
      <div
        ref={dialog}
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
          {verb} Repeating Event
        </h2>
        <p className="mb-4 text-[13px] leading-[18px]">
          {verb} only this event, this and all future events, or every event in the series?
        </p>
        <div className="flex flex-col items-end gap-2">
          <button type="button" className={PRIMARY_BUTTON} onClick={() => onChoose("this")}>
            This Event
          </button>
          {!first && (
            <button type="button" className={BUTTON} onClick={() => onChoose("future")}>
              All Future Events
            </button>
          )}
          <button type="button" className={BUTTON} onClick={() => onChoose("all")}>
            All Events
          </button>
          <button type="button" className={BUTTON} onClick={onCancel}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
