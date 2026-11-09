// The reader's find bar (ADR-0052; docs/specs/ui.md, Find in the message):
// it finds in what the reader shows, counts the matches, marks them and
// moves through them. It sits above the reader's scroller, so it stays in
// view while the message scrolls, and it keeps its own text out of the
// search, as it does for any marked subtree.

import { ChevronDown, ChevronUp, Search } from "lucide-react";
import { type KeyboardEvent, type RefObject, useCallback, useEffect, useRef, useState } from "react";

import { clearMarks, findRanges, highlightWindows, markRanges, scrollToRange } from "./findInMessage";

/** FindBarProps are the bar's inputs. */
export interface FindBarProps {
  /** The reader's scroller: everything it shows is searched. */
  scroller: HTMLElement | null;
  /** Escape or Done. */
  onClose: () => void;
  /** The bar's field, which the window's Ctrl+F focuses. */
  inputRef?: RefObject<HTMLInputElement | null>;
}

/** FindBar finds in the message shown and counts what it finds. */
export function FindBar({ scroller, onClose, inputRef }: FindBarProps) {
  const [text, setText] = useState("");
  const [matches, setMatches] = useState<Range[]>([]);
  const [index, setIndex] = useState(0);
  const own = useRef<HTMLInputElement>(null);
  const field = inputRef ?? own;
  // The windows this bar marked: a window that holds no match any more keeps
  // no highlight, and the bar takes its marks away when it closes.
  const marked = useRef<Window[]>([]);

  const show = useCallback(
    (found: Range[], wanted: number) => {
      const windows = highlightWindows(found);
      const stale = marked.current.filter((win) => !windows.includes(win));
      if (stale.length > 0) clearMarks(stale);
      marked.current = windows;
      const at = found.length === 0 ? 0 : Math.min(Math.max(wanted, 0), found.length - 1);
      const current = found[at] ?? null;
      setMatches(found);
      setIndex(at);
      markRanges(found, current);
      if (current !== null && scroller !== null) scrollToRange(current, scroller);
    },
    [scroller],
  );

  const search = (query: string) => show(scroller === null ? [] : findRanges(scroller, query), 0);

  // Each move finds the matches again, so a message that expanded while the
  // bar was open joins at the next move (docs/specs/ui.md, Find in the
  // message: Moving).
  const step = (direction: 1 | -1) => {
    const found = scroller === null ? [] : findRanges(scroller, text);
    if (found.length === 0) {
      show(found, 0);
      return;
    }
    const base = Math.min(index, found.length - 1);
    show(found, (base + direction + found.length) % found.length);
  };

  useEffect(
    () => () => {
      clearMarks(marked.current);
      marked.current = [];
    },
    [],
  );

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // The window's keys leave plain Ctrl+F to a text field, so the bar takes
    // its own Ctrl+F back: the key focuses the field and selects its text
    // (docs/specs/ui.md, Find in the message).
    if (event.ctrlKey && !event.altKey && !event.shiftKey && !event.metaKey && event.key.toLowerCase() === "f") {
      event.preventDefault();
      event.stopPropagation();
      field.current?.focus();
      field.current?.select();
      return;
    }
    if (event.key === "Escape") {
      // A sheet or the search field would take Escape too; the bar goes first.
      event.preventDefault();
      event.stopPropagation();
      onClose();
      return;
    }
    // Enter never leaves the bar: from the list's keys the window would take
    // it as open, and cancel the button's own activation (Done would not
    // close). Only the field's Enter moves the match.
    if (event.key === "Enter") {
      event.stopPropagation();
      if (event.target !== field.current) return;
      event.preventDefault();
      step(event.shiftKey ? -1 : 1);
    }
  };

  const empty = text === "";
  const count = empty ? "" : matches.length === 0 ? "No Matches" : `${index + 1} of ${matches.length}`;
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: the bar's keys work from its field and its buttons, which sit in it.
    <div
      data-find-skip
      onKeyDown={onKeyDown}
      className="flex shrink-0 items-center gap-2 border-b border-separator bg-window px-3 py-2 print:hidden"
    >
      <div className="flex h-[28px] w-[280px] max-w-full items-center gap-1 rounded-md bg-selection-inactive px-[6px]">
        <Search size={14} className="shrink-0 text-tertiary" />
        <input
          ref={field}
          type="search"
          aria-label="Find in Message"
          placeholder="Find in Message"
          className="min-w-0 flex-1 border-0 bg-transparent text-[13px] outline-none"
          value={text}
          onChange={(event) => {
            setText(event.target.value);
            search(event.target.value);
          }}
        />
      </div>
      <p aria-live="polite" className="text-[12px] leading-4 text-secondary">
        {count}
      </p>
      <button
        type="button"
        aria-label="Previous Match"
        disabled={matches.length === 0}
        onClick={() => step(-1)}
        className="flex size-6 items-center justify-center rounded text-secondary disabled:opacity-40 enabled:hover:bg-selection-inactive"
      >
        <ChevronUp size={14} />
      </button>
      <button
        type="button"
        aria-label="Next Match"
        disabled={matches.length === 0}
        onClick={() => step(1)}
        className="flex size-6 items-center justify-center rounded text-secondary disabled:opacity-40 enabled:hover:bg-selection-inactive"
      >
        <ChevronDown size={14} />
      </button>
      <button type="button" onClick={onClose} className="ml-auto rounded px-2 py-1 text-[12px] text-accent">
        Done
      </button>
    </div>
  );
}
