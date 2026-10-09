// The toolbar's search field and the scope bar under the toolbar
// (docs/specs/ui.md, Behavior: Search).
import { CircleX, Search } from "lucide-react";
import { type ChangeEvent, type KeyboardEvent, type Ref, useCallback, useEffect, useRef } from "react";

/** SearchFieldProps are the search field's inputs. */
export interface SearchFieldProps {
  /** The text shown; the field is controlled. */
  value: string;
  placeholder?: string;
  /** Every edit, at once. */
  onChange: (text: string) => void;
  /** The trimmed text, 250 ms after the last edit, or at once on Enter. */
  onSearch: (text: string) => void;
  /** Escape or the clear button. */
  onClear: () => void;
  inputRef?: Ref<HTMLInputElement>;
}

function useSearchHandlers(props: SearchFieldProps) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancel = useCallback(() => {
    if (timer.current !== null) {
      clearTimeout(timer.current);
      timer.current = null;
    }
  }, []);
  useEffect(() => cancel, [cancel]);
  useEffect(() => {
    if (props.value === "") cancel();
  }, [props.value, cancel]);

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const text = event.target.value;
    props.onChange(text);
    cancel();
    timer.current = setTimeout(() => props.onSearch(text.trim()), 250);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter") {
      cancel();
      props.onSearch(props.value.trim());
    } else if (event.key === "Escape") {
      cancel();
      props.onClear();
    }
  };

  const clear = () => {
    cancel();
    props.onClear();
  };

  return { handleChange, handleKeyDown, clear };
}

/** SearchField is the toolbar's search box: it reports edits at once and
 * searches the trimmed text 250 ms after the last one. */
export function SearchField(props: SearchFieldProps) {
  const { handleChange, handleKeyDown, clear } = useSearchHandlers(props);
  return (
    <div className="flex h-[28px] w-[220px] items-center rounded-md bg-selection-inactive px-[6px]">
      <Search size={14} className="text-tertiary" />
      <input
        type="search"
        aria-label="Search"
        placeholder={props.placeholder ?? "Search"}
        className="flex-1 border-0 bg-transparent text-[13px] outline-none"
        value={props.value}
        ref={props.inputRef}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
      />
      {props.value !== "" && (
        <button type="button" aria-label="Clear search" onClick={clear}>
          <CircleX size={14} className="text-tertiary" />
        </button>
      )}
    </div>
  );
}

/** Scope is one choice of where to search. */
export interface Scope {
  key: string;
  label: string;
}

/** ScopeBarProps are the scope bar's inputs. */
export interface ScopeBarProps {
  scopes: Scope[];
  selected: string;
  onSelect: (key: string) => void;
}

/** ScopeBar picks where a search looks: all mailboxes or the current one. */
export function ScopeBar(props: ScopeBarProps) {
  return (
    <div className="flex h-[28px] items-center gap-2 border-b border-separator bg-toolbar pl-3">
      <span className="text-[12px] text-secondary">Search:</span>
      {props.scopes.map((scope) => {
        const active = scope.key === props.selected;
        const style = active ? "bg-selection-inactive text-primary" : "bg-transparent text-secondary";
        return (
          <button
            key={scope.key}
            type="button"
            aria-pressed={active}
            onClick={() => props.onSelect(scope.key)}
            className={`flex h-[20px] items-center rounded border-0 px-2 text-[12px] ${style}`}
          >
            {scope.label}
          </button>
        );
      })}
    </div>
  );
}
