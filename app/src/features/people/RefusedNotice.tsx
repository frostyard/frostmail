import { X } from "lucide-react";

/** refusedText explains why a contact or event change was not saved. */
export function refusedText(name: string, reason: string): string {
  if (reason === "changed") return `“${name}” was changed on another device, so your change wasn't saved.`;
  if (reason === "deleted") return `“${name}” was deleted on another device.`;
  return `The server didn't accept your change to “${name}”.`;
}

/** RefusedNotice shows a dismissible refusal in the undo toast's frame. */
export function RefusedNotice({ text, onDismiss }: { text: string; onDismiss: () => void }) {
  return (
    <output className="flex h-9 items-center gap-3 rounded-lg border border-separator bg-toolbar px-3 text-[13px] leading-[18px] shadow-md">
      <span>{text}</span>
      <button type="button" aria-label="Dismiss" className="shrink-0 text-secondary" onClick={onDismiss}>
        <X size={14} aria-hidden="true" />
      </button>
    </output>
  );
}
