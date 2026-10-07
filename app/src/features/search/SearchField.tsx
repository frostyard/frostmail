// The toolbar's search field and the scope bar under the toolbar
// (docs/specs/ui.md, Behavior: Search). Task T-0035 implements both; the
// stubs render a bare input and nothing.
import type { Ref } from "react";

/** SearchFieldProps are the search field's inputs. */
export interface SearchFieldProps {
  /** The text shown; the field is controlled. */
  value: string;
  /** Every edit, at once. */
  onChange: (text: string) => void;
  /** The trimmed text, 250 ms after the last edit, or at once on Enter. */
  onSearch: (text: string) => void;
  /** Escape or the clear button. */
  onClear: () => void;
  inputRef?: Ref<HTMLInputElement>;
}

/** SearchField is the toolbar's search box. */
export function SearchField(props: SearchFieldProps) {
  return <input aria-label="Search" value={props.value} readOnly />;
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

/** ScopeBar picks where a search looks. */
export function ScopeBar(_props: ScopeBarProps) {
  return null;
}
