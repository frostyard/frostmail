// The People module's sidebar (docs/specs/pim-ui.md, People sidebar).
// Task T-0063 writes it.

/** AddressBookRow is one address book in the sidebar. */
export interface AddressBookRow {
  id: number;
  name: string;
  readOnly: boolean;
}

/** AddressBookSection is an account and its address books. */
export interface AddressBookSection {
  accountId: number;
  title: string;
  books: AddressBookRow[];
}

/** PeopleSidebarProps are the People sidebar's inputs. */
export interface PeopleSidebarProps {
  sections: AddressBookSection[];
  /** "all" for All Contacts, or the selected address book's ID. */
  selected: "all" | number;
  /** The sidebar has keyboard focus: the selection uses the accent color. */
  focused: boolean;
  onSelect: (s: "all" | number) => void;
}

/** PeopleSidebar shows All Contacts and each account's address books. */
export function PeopleSidebar(_props: PeopleSidebarProps) {
  return null;
}
