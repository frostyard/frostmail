import { Sheet } from "@frostyard/lodge-ui/react";
import { useEffect, useId, useRef, useState } from "react";
import { ALERT, BUTTON, FIELD, PRIMARY_BUTTON } from "../settings/labels";

/** MailboxSheetMode names the mailbox operation being edited. */
export type MailboxSheetMode = "new" | "rename" | "move";

/** MailboxChoice is a possible parent mailbox. */
export interface MailboxChoice {
  id: number;
  path: string;
}

/** MailboxSheetProps supplies the fields and callbacks for a mailbox operation. */
export interface MailboxSheetProps {
  mode: MailboxSheetMode;
  name: string;
  parentId?: number;
  locations: MailboxChoice[];
  busy?: boolean;
  error?: string;
  onSave: (draft: { name: string; parentId?: number }) => void;
  onCancel: () => void;
}

/** MailboxSheet edits a mailbox's name or location. */
export function MailboxSheet(props: MailboxSheetProps) {
  const [name, setName] = useState(props.name);
  const [parent, setParent] = useState(props.parentId === undefined ? "" : String(props.parentId));
  const title = { new: "New Mailbox", rename: "Rename Mailbox", move: "Move Mailbox" }[props.mode];
  return (
    <Sheet
      title={title}
      width={400}
      className="rounded-[10px]"
      titleClassName="text-[13px] font-semibold"
      showClose={false}
      closeOnOverlay={false}
      initialFocus={"first"}
      onClose={props.onCancel}
    >
      {/* biome-ignore lint/a11y/noStaticElementInteractions: the Sheet's frame is the dialog; this wrapper only keeps Tab and the keys inside it */}
      <div
        onKeyDown={(event) => {
          if (event.key !== "Tab") return;
          const controls = Array.from(
            event.currentTarget.querySelectorAll<HTMLElement>("input, select, button"),
          ).filter((control) => !control.hasAttribute("disabled"));
          const first = controls[0];
          const last = controls.at(-1);
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last?.focus();
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first?.focus();
          }
        }}
      >
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (!name.trim() || props.busy) return;
            props.onSave({
              name: name.trim(),
              ...(props.mode !== "rename" && parent !== "" ? { parentId: Number(parent) } : {}),
            });
          }}
        >
          {props.mode !== "move" && (
            <label className="flex items-center gap-2 text-[13px]">
              Name:
              <input
                className={`${FIELD} min-w-0 flex-1`}
                maxLength={100}
                value={name}
                onChange={(event) => setName(event.currentTarget.value)}
              />
            </label>
          )}
          {props.mode !== "rename" && (
            <label className="flex items-center gap-2 text-[13px]">
              Location:
              <select
                className={`${FIELD} min-w-0 flex-1`}
                value={parent}
                onChange={(event) => setParent(event.currentTarget.value)}
              >
                <option value="">Top Level</option>
                {props.locations.map((location) => (
                  <option key={location.id} value={location.id}>
                    {location.path}
                  </option>
                ))}
              </select>
            </label>
          )}
          {props.error && (
            <p role="alert" className={ALERT}>
              {props.error}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button type="button" className={BUTTON} onClick={props.onCancel}>
              Cancel
            </button>
            <button type="submit" className={PRIMARY_BUTTON} disabled={!name.trim() || props.busy}>
              OK
            </button>
          </div>
        </form>
      </div>
    </Sheet>
  );
}
