// The command palette: a field that finds a command by name, each row with
// its keys, which runs the command its key would (docs/specs/ui.md, Command
// palette). It is presentational: PaletteContainer gives it the commands of
// the module shown and runs the one chosen.

import { Sheet } from "@frostyard/lodge-ui/react";
import { type KeyboardEvent, useId, useRef, useState } from "react";
import { type Command, matchesQuery, type PaletteItem } from "../../lib/keymap";

/** CommandPaletteProps are the palette's commands and what it asks for. */
export interface CommandPaletteProps {
  /** The module's commands, in the palette's order. */
  items: PaletteItem[];
  onRun: (command: Command) => void;
  onClose: () => void;
}

const ROW =
  "flex w-full items-center justify-between gap-3 rounded-md px-3 py-1.5 text-left text-[13px] leading-[18px]";

/** CommandPalette is the palette's sheet: the field, and the list under it. */
export function CommandPalette(props: CommandPaletteProps) {
  const [text, setText] = useState("");
  const [highlight, setHighlight] = useState(0);
  const field = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const base = useId();
  const listId = `${base}list`;
  const optionId = (index: number) => `${base}option-${index}`;
  const items = props.items.filter((item) => matchesQuery(item.name, text));
  // A shorter list may not hold the highlight's row any more.
  const active = items.length === 0 ? -1 : Math.min(highlight, items.length - 1);
  const current = active === -1 ? undefined : items[active];

  const move = (delta: number) => {
    if (items.length === 0) return;
    const next = Math.max(0, Math.min(items.length - 1, active + delta));
    setHighlight(next);
    list.current?.querySelector<HTMLElement>(`[data-row="${next}"]`)?.scrollIntoView?.({ block: "nearest" });
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      move(event.key === "ArrowDown" ? 1 : -1);
    } else if (event.key === "Enter") {
      // A row that needs what is missing stays: its reason says why.
      if (current && current.reason === undefined) props.onRun(current.command);
    }
  };

  return (
    <Sheet ariaLabel="Commands" width={480} showClose={false} initialFocus={field} onClose={props.onClose}>
      <input
        ref={field}
        type="text"
        aria-label="Command"
        placeholder="Find a Command"
        role="combobox"
        aria-expanded={true}
        aria-controls={listId}
        aria-activedescendant={current === undefined ? undefined : optionId(active)}
        aria-autocomplete="list"
        value={text}
        onChange={(event) => {
          setText(event.target.value);
          setHighlight(0);
        }}
        onKeyDown={onKeyDown}
        className="h-8 w-full rounded-md border border-separator bg-window px-2 text-[13px] leading-[18px] focus:outline-none focus:ring-2 focus:ring-focus"
      />
      <div
        ref={list}
        id={listId}
        role="listbox"
        aria-label="Commands"
        className="mt-2 max-h-[360px] overflow-auto focus:outline-none"
      >
        {items.map((item, index) => {
          const disabled = item.reason !== undefined;
          return (
            // biome-ignore lint/a11y/useKeyWithClickEvents: the field holds the keys (aria-activedescendant); a row is their mouse target.
            <div
              key={item.command}
              id={optionId(index)}
              role="option"
              tabIndex={-1}
              aria-selected={index === active}
              aria-disabled={disabled ? true : undefined}
              data-row={index}
              onClick={() => {
                if (!disabled) props.onRun(item.command);
              }}
              className={`${ROW} ${index === active ? "bg-selection-inactive" : ""} ${disabled ? "opacity-40" : ""}`}
            >
              <span className="flex min-w-0 flex-col">
                <span className="truncate">{item.name}</span>
                {disabled && <span className="truncate text-[12px] leading-4 text-secondary">{item.reason}</span>}
              </span>
              <span className="shrink-0 text-[12px] leading-4 text-secondary">{item.shortcut}</span>
            </div>
          );
        })}
      </div>
      {items.length === 0 && (
        <p className="px-3 py-1.5 text-[13px] leading-[18px] text-secondary" role="status">
          No Commands
        </p>
      )}
    </Sheet>
  );
}
