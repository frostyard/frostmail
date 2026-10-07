// The app's own menus: context menus on list rows and the toolbar's flag and
// move menus (docs/specs/ui.md, Behavior: Context menu). Task T-0033
// implements ContextMenu; the stub renders nothing.
import type { ReactNode } from "react";

/** MenuItem is one entry of a menu. */
export type MenuItem =
  | {
      kind: "item";
      id: string;
      label: string;
      disabled?: boolean;
      /** Shows a check mark when true. */
      checked?: boolean;
      /** A 14px icon or swatch before the label. */
      icon?: ReactNode;
      /** Shortcut text shown at the right, such as "Ctrl+Shift+U". */
      shortcut?: string;
    }
  | { kind: "separator" }
  | { kind: "submenu"; id: string; label: string; disabled?: boolean; items: MenuItem[] };

/** ContextMenuProps are a menu's inputs. */
export interface ContextMenuProps {
  items: MenuItem[];
  /** Where the menu's top-left corner goes, in viewport pixels. */
  x: number;
  y: number;
  /** An enabled item was chosen; onClose follows. */
  onSelect: (id: string) => void;
  /** The menu should disappear. */
  onClose: () => void;
}

/** ContextMenu is a popup menu with keyboard navigation and submenus. */
export function ContextMenu(_props: ContextMenuProps) {
  return null;
}
