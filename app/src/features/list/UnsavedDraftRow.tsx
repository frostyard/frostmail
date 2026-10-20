// A draft no stored message holds yet, at the top of Drafts (docs/specs/ui.md,
// Message list): maild has it, but its copy has not reached the Drafts
// mailbox, so the list shows the draft itself, marked Not Saved. Who it is
// to and the date on line 1, its subject on line 2, the start of its text
// below. A click, Enter or Space opens it in Compose.
import type { KeyboardEvent } from "react";

import { formatAddressList, formatListDate } from "../../lib/format";
import type { Draft } from "../../rpc/gen/api";

/** draftPreview is the start of a draft's own text: its HTML without the quoted message, as plain text. */
export function draftPreview(html: string): string {
  const doc = new DOMParser().parseFromString(html, "text/html");
  for (const quote of doc.querySelectorAll("blockquote")) quote.remove();
  // Paragraphs and lines read as words apart.
  for (const block of doc.body.querySelectorAll("p, div, br, li, tr, h1, h2, h3, h4, h5, h6")) block.after(" ");
  return (doc.body.textContent ?? "").replace(/\s+/g, " ").trim().slice(0, 200);
}

/** UnsavedDraftRowProps are an unsaved draft row's inputs. */
export interface UnsavedDraftRowProps {
  draft: Draft;
  /** The clock dates are formatted against. */
  now: Date;
  onOpen: (id: number) => void;
}

/** UnsavedDraftRow renders one unsaved draft as a list row of a message's height. */
export function UnsavedDraftRow({ draft, now, onOpen }: UnsavedDraftRowProps) {
  const to = [...draft.content.to, ...draft.content.cc, ...draft.content.bcc];
  const subject = draft.content.subject.trim();
  const preview = draftPreview(draft.content.html);
  const open = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    event.stopPropagation();
    onOpen(draft.id);
  };
  return (
    <div
      role="option"
      aria-selected={false}
      tabIndex={0}
      data-draft-id={draft.id}
      title="Not yet saved to the server: open it in Compose"
      className="relative h-[84px] cursor-default pt-2 pr-3 pb-2 pl-6 outline-none focus-visible:bg-selection-inactive"
      onClick={() => onOpen(draft.id)}
      onKeyDown={open}
    >
      <div className="absolute right-0 bottom-0 left-6 h-px bg-separator" />
      <div className="flex items-center gap-1">
        <span className={`text-list-sender flex-1 truncate ${to.length === 0 ? "text-tertiary" : ""}`}>
          {to.length === 0 ? "No Recipients" : `To: ${formatAddressList(to)}`}
        </span>
        <span className="text-list-date shrink-0 tabular-nums text-secondary">
          {formatListDate(new Date(draft.updatedAt), now)}
        </span>
      </div>
      <div className="flex items-center gap-1">
        <span className={`text-list-subject flex-1 truncate ${subject === "" ? "text-tertiary" : ""}`}>
          {subject === "" ? "(No Subject)" : subject}
        </span>
        <span className="h-4 shrink-0 rounded-lg bg-badge px-[5px] text-[10px] font-semibold leading-4 text-badge-text">
          Not Saved
        </span>
      </div>
      <div className="text-list-preview line-clamp-2 text-secondary">{preview}</div>
    </div>
  );
}
