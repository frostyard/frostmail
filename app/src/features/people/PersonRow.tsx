// A row of the People list and the list's letter headers
// (docs/specs/pim-ui.md, People list), with focus-sensitive selection.
import type { PersonSummary } from "../../rpc/gen/api";

import { Avatar } from "./Avatar";

/** PersonRowProps are a row's inputs. */
export interface PersonRowProps {
  person: PersonSummary;
  selected: boolean;
  /** The list has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  onSelect: (id: number) => void;
}

/** PersonRow shows a person's avatar, name and organization or email. */
export function PersonRow({ person, selected, focused, onSelect }: PersonRowProps) {
  const contrast = selected && focused;
  const name = person.displayName || person.email;
  const detail = person.organization || person.email;
  return (
    <div
      role="option"
      data-person-id={person.id}
      tabIndex={-1}
      aria-selected={selected}
      className={`relative flex h-[44px] items-center gap-[10px] px-3 ${selected ? (contrast ? "bg-accent text-accent-contrast" : "bg-selection-inactive") : ""}`}
      onClick={() => onSelect(person.id)}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onSelect(person.id);
        }
      }}
    >
      <Avatar name={person.displayName} email={person.email} size={28} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-list-sender font-semibold">{name}</div>
        {detail && detail !== name && (
          <div className={`truncate text-list-preview ${contrast ? "text-accent-contrast" : "text-secondary"}`}>
            {detail}
          </div>
        )}
      </div>
      {!selected && <div className="absolute bottom-0 left-[50px] right-0 h-px bg-separator" />}
    </div>
  );
}

/** IndexHeader is the letter heading a run of people in the list. */
export function IndexHeader({ letter }: { letter: string }) {
  return (
    <div
      role="presentation"
      className="sticky top-0 z-10 flex h-[24px] items-center bg-sidebar pl-3 text-sidebar-section text-secondary"
    >
      {letter}
    </div>
  );
}
