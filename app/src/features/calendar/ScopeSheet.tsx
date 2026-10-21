import { Sheet } from "@frostyard/lodge-ui/react";
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
  const verb = action === "change" ? "Change" : "Delete";
  return (
    <Sheet
      title={`${verb} Repeating Event`}
      width={420}
      className="rounded-[10px]"
      titleClassName="text-[13px] font-semibold"
      showClose={false}
      closeOnOverlay={false}
      initialFocus={"first"}
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
    </Sheet>
  );
}
