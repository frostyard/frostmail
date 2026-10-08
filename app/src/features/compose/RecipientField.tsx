// The To, Cc and Bcc fields of the compose window: the field's addresses as
// tokens, a text input that turns typed, committed and pasted text into
// tokens, and a suggestion list under it (docs/specs/compose-ui.md,
// Recipient field and Behavior: Recipients). The container owns the address
// list and asks maild for suggestions through `suggest`.
import type { ChangeEvent, ClipboardEvent, KeyboardEvent } from "react";
import { useId, useRef, useState } from "react";

import { isValidAddress, parseAddresses } from "../../lib/addressParse";
import type { Address } from "../../rpc/gen/api";

/** RecipientFieldProps are a recipient field's inputs. */
export interface RecipientFieldProps {
  /** The field's name and the input's accessible name: "To", "Cc" or "Bcc". */
  label: string;
  /** The field's addresses, in order. */
  value: Address[];
  /** Called with the new list whenever tokens are added or removed. */
  onChange: (next: Address[]) => void;
  /** Suggestions for typed text; the container asks maild. */
  suggest: (prefix: string) => Promise<Address[]>;
  autoFocus?: boolean;
}

/** MAX_SUGGESTIONS is how many suggestions the list shows at most. */
const MAX_SUGGESTIONS = 8;

/** PAST_LIST matches pasted text that is a list, so it commits at once. */
const PAST_LIST = /[,;\r\n]/;

/** joinClasses drops the empty parts and joins the rest with single spaces. */
function joinClasses(...parts: Array<string | false | undefined>): string {
  return parts.filter((part) => part !== undefined && part !== false).join(" ");
}

/** keyOf is the address key tokens, suggestions and duplicates are tracked by. */
function keyOf(address: string): string {
  return address.toLowerCase();
}

function hasAddress(list: Address[], address: string): boolean {
  const key = keyOf(address);
  return list.some((addr) => keyOf(addr.address) === key);
}

/** tokenText is a token's label: the name, or the address when it has none. */
function tokenText(addr: Address): string {
  return addr.name === "" ? addr.address : addr.name;
}

/** tokenTitle is a token's tooltip, or the complaint when the address is invalid. */
function tokenTitle(addr: Address): string {
  if (!isValidAddress(addr.address)) {
    return `${addr.address} is not a valid address`;
  }
  return addr.name === "" ? addr.address : `${addr.name} <${addr.address}>`;
}

/** RecipientField is a token field for addresses. */
export function RecipientField(props: RecipientFieldProps) {
  const { label, value, onChange, suggest, autoFocus } = props;
  const listId = useId();
  const inputRef = useRef<HTMLInputElement | null>(null);
  // The trimmed text the latest suggest call was made for, so results for
  // text that has since changed are dropped.
  const latestRef = useRef("");
  const [text, setText] = useState("");
  const [suggestions, setSuggestions] = useState<Address[]>([]);
  const [open, setOpen] = useState(false);
  const [highlight, setHighlight] = useState<number | undefined>(undefined);
  const [selected, setSelected] = useState<string | undefined>(undefined);

  const visible = suggestions.filter((addr) => !hasAddress(value, addr.address)).slice(0, MAX_SUGGESTIONS);
  const listShown = open && visible.length > 0;
  const active = listShown && highlight !== undefined ? visible[highlight] : undefined;

  function closeList() {
    setSuggestions([]);
    setOpen(false);
    setHighlight(undefined);
  }

  function commit(raw: string) {
    const seen = new Set(value.map((addr) => keyOf(addr.address)));
    const added: Address[] = [];
    const parsed = parseAddresses(raw);
    for (const addr of parsed.addresses) {
      const key = keyOf(addr.address);
      if (seen.has(key)) continue;
      seen.add(key);
      added.push(addr);
    }
    for (const piece of parsed.invalid) {
      const key = keyOf(piece);
      if (seen.has(key)) continue;
      seen.add(key);
      added.push({ name: "", address: piece });
    }
    setText("");
    closeList();
    if (added.length > 0) {
      onChange([...value, ...added]);
    }
  }

  function pick(addr: Address) {
    setText("");
    closeList();
    setSelected(undefined);
    onChange([...value, addr]);
    inputRef.current?.focus();
  }

  function moveHighlight(step: number, count: number) {
    setHighlight((prev) => {
      if (prev === undefined) return step > 0 ? 0 : count - 1;
      return Math.min(Math.max(prev + step, 0), count - 1);
    });
  }

  function removeSelected() {
    if (selected === undefined) return;
    onChange(value.filter((addr) => keyOf(addr.address) !== selected));
    setSelected(undefined);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if ((event.key === "ArrowDown" || event.key === "ArrowUp") && listShown) {
      event.preventDefault();
      moveHighlight(event.key === "ArrowDown" ? 1 : -1, visible.length);
      return;
    }
    if (event.key === "Escape" && listShown) {
      setOpen(false);
      return;
    }
    if (active !== undefined && (event.key === "Enter" || event.key === "Tab")) {
      event.preventDefault();
      pick(active);
      return;
    }
    if (event.key === "Enter" || event.key === "," || event.key === ";") {
      event.preventDefault();
      if (text.trim() !== "") commit(text);
      return;
    }
    if (event.key === "Tab") {
      if (text.trim() !== "") commit(text);
      return;
    }
    if ((event.key === "Backspace" || event.key === "Delete") && text === "") {
      if (selected !== undefined) {
        event.preventDefault();
        removeSelected();
      } else if (event.key === "Backspace" && value.length > 0) {
        event.preventDefault();
        const last = value[value.length - 1];
        if (last !== undefined) setSelected(keyOf(last.address));
      }
    }
  }

  function onInputChange(event: ChangeEvent<HTMLInputElement>) {
    const next = event.target.value;
    setText(next);
    setSelected(undefined);
    setHighlight(undefined);
    const trimmed = next.trim();
    latestRef.current = trimmed;
    if (trimmed === "") {
      closeList();
      return;
    }
    void suggest(trimmed).then((results) => {
      if (latestRef.current !== trimmed) return;
      setSuggestions(results);
      setOpen(true);
    });
  }

  function onPaste(event: ClipboardEvent<HTMLInputElement>) {
    const pasted = event.clipboardData.getData("text");
    if (PAST_LIST.test(pasted)) {
      event.preventDefault();
      commit(text + pasted);
    }
  }

  function onBlur() {
    setSelected(undefined);
    if (text.trim() !== "") {
      commit(text);
    } else {
      setOpen(false);
    }
  }

  return (
    <div className="relative flex min-w-0 flex-1 flex-wrap items-center gap-1 py-1">
      {value.map((addr) => {
        const key = keyOf(addr.address);
        const invalid = !isValidAddress(addr.address);
        const isSelected = selected === key;
        return (
          <button
            key={key}
            type="button"
            tabIndex={-1}
            aria-pressed={isSelected}
            data-invalid={invalid ? "true" : undefined}
            title={tokenTitle(addr)}
            className={joinClasses(
              "h-[22px] rounded px-1.5 text-[13px] leading-[18px]",
              isSelected ? "bg-accent text-accent-contrast" : invalid ? "text-flag-1" : "text-accent",
            )}
            onMouseDown={(event) => {
              event.preventDefault();
              inputRef.current?.focus();
            }}
            onClick={() => setSelected(key)}
          >
            {tokenText(addr)}
          </button>
        );
      })}
      <input
        ref={inputRef}
        type="text"
        role="combobox"
        aria-label={label}
        aria-expanded={listShown}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={active === undefined ? undefined : `${listId}-${highlight}`}
        // biome-ignore lint/a11y/noAutofocus: the compose window opens with the cursor in its first recipient field.
        autoFocus={autoFocus}
        value={text}
        className="min-w-[80px] flex-1 border-none bg-transparent text-[13px] leading-[18px] text-primary outline-none"
        onChange={onInputChange}
        onKeyDown={onKeyDown}
        onPaste={onPaste}
        onBlur={onBlur}
      />
      {listShown && (
        <div
          role="listbox"
          id={listId}
          aria-label={`${label} suggestions`}
          className="absolute top-full left-0 z-10 w-full overflow-y-auto rounded-md border border-separator bg-window shadow-md"
        >
          {visible.map((addr, index) => {
            const on = highlight === index;
            return (
              // biome-ignore lint/a11y/useKeyWithClickEvents: the combobox input owns the keyboard; the option only reports the pointer.
              <div
                key={keyOf(addr.address)}
                role="option"
                tabIndex={-1}
                id={`${listId}-${index}`}
                aria-selected={on}
                className={joinClasses("flex h-9 flex-col justify-center px-2", on && "bg-accent text-accent-contrast")}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => pick(addr)}
              >
                {addr.name !== "" && <span className="text-[13px] leading-[18px]">{addr.name}</span>}
                <span className={joinClasses("text-[12px] leading-4", !on && "text-secondary")}>{addr.address}</span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
