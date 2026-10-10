// The app's own menus: context menus on list rows and the toolbar's flag and
// move menus (docs/specs/ui.md, Behavior: Context menu). ContextMenu is a
// positioned popup with items, separators, check marks, shortcuts, submenus,
// keyboard navigation and outside-click closing.
import { Check, ChevronRight } from "lucide-react";
import {
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";

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

/** Level is one open menu: its rows, the highlighted row, and the row whose submenu is open. */
interface Level {
  items: MenuItem[];
  highlighted: number | null;
  openIndex: number | null;
}

const EDGE_GAP = 4;

function usable(item: MenuItem | undefined): boolean {
  return item !== undefined && item.kind !== "separator" && item.disabled !== true;
}

/** nextIndex returns the next usable row after from, stepping and wrapping, or null. */
function nextIndex(items: MenuItem[], from: number, step: number): number | null {
  const n = items.length;
  for (let i = 1; i <= n; i++) {
    const idx = (((from + step * i) % n) + n) % n;
    if (usable(items[idx])) return idx;
  }
  return null;
}

function submenuLevel(items: MenuItem[]): Level {
  return { items, highlighted: nextIndex(items, -1, 1), openIndex: null };
}

/** withSubmenu replaces the level at depth with its opened row and appends the submenu. */
function withSubmenu(prev: Level[], depth: number, index: number, items: MenuItem[]): Level[] {
  const level = prev[depth];
  if (!level) return prev;
  const opened: Level = { ...level, highlighted: index, openIndex: index };
  return [...prev.slice(0, depth), opened, submenuLevel(items)];
}

function clamp(value: number, max: number): number {
  return Math.max(0, Math.min(value, max));
}

function rowClass(highlighted: boolean, disabled: boolean): string {
  const base = "flex h-6 w-full cursor-default items-center gap-2 border-0 px-3 text-left text-[13px]";
  if (disabled) return `${base} bg-transparent text-tertiary`;
  if (highlighted) return `${base} bg-accent text-accent-contrast`;
  return `${base} bg-transparent text-primary`;
}

/** ContextMenu is a popup menu with keyboard navigation and submenus. */
export function ContextMenu(props: ContextMenuProps) {
  const [levels, setLevels] = useState<Level[]>([{ items: props.items, highlighted: null, openIndex: null }]);
  const [pos, setPos] = useState({ left: props.x, top: props.y });
  const rootRef = useRef<HTMLDivElement | null>(null);
  const menuEls = useRef<(HTMLDivElement | null)[]>([]);
  const rowEls = useRef<(HTMLButtonElement | null)[][]>([]);

  useEffect(() => {
    rootRef.current?.focus();
  }, []);

  useLayoutEffect(() => {
    const el = rootRef.current;
    if (!el) return;
    const left = clamp(props.x, window.innerWidth - el.offsetWidth - EDGE_GAP);
    const top = clamp(props.y, window.innerHeight - el.offsetHeight - EDGE_GAP);
    setPos((prev) => (prev.left === left && prev.top === top ? prev : { left, top }));
  }, [props.x, props.y]);

  useLayoutEffect(() => {
    for (let i = 1; i < levels.length; i++) {
      const anchorIndex = levels[i - 1]?.openIndex;
      if (anchorIndex === null || anchorIndex === undefined) continue;
      const anchor = rowEls.current[i - 1]?.[anchorIndex];
      const sub = menuEls.current[i];
      if (!anchor || !sub) continue;
      // Beside its row, flipped to the parent's left when the window ends
      // first, and moved up so its last items stay in the window.
      const rect = anchor.getBoundingClientRect();
      const parent = menuEls.current[i - 1]?.getBoundingClientRect();
      const maxLeft = window.innerWidth - sub.offsetWidth - EDGE_GAP;
      const left = rect.right > maxLeft && parent ? parent.left - sub.offsetWidth : rect.right;
      sub.style.left = `${Math.round(clamp(left, maxLeft))}px`;
      sub.style.top = `${Math.round(clamp(rect.top, window.innerHeight - sub.offsetHeight - EDGE_GAP))}px`;
    }
  }, [levels]);

  useEffect(() => {
    const onDocMouseDown = (event: globalThis.MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) return;
      for (const el of menuEls.current) {
        if (el?.contains(target)) return;
      }
      props.onClose();
    };
    document.addEventListener("mousedown", onDocMouseDown);
    return () => document.removeEventListener("mousedown", onDocMouseDown);
  }, [props.onClose]);

  const setHighlight = (depth: number, index: number | null) => {
    setLevels((prev) => prev.map((lvl, i) => (i === depth ? { ...lvl, highlighted: index } : lvl)));
  };

  const openSubmenu = (depth: number, index: number) => {
    setLevels((prev) => {
      const row = prev[depth]?.items[index];
      if (row?.kind !== "submenu" || row.disabled === true) return prev;
      return withSubmenu(prev, depth, index, row.items);
    });
  };

  const closeSubmenu = (depth: number) => {
    setLevels((prev) => {
      const parent = prev[depth - 1];
      if (depth < 1 || !parent) return prev;
      const restored: Level = { ...parent, highlighted: parent.openIndex ?? parent.highlighted, openIndex: null };
      return [...prev.slice(0, depth - 1), restored];
    });
  };

  const hover = (depth: number, index: number) => {
    setLevels((prev) => {
      const level = prev[depth];
      const row = level?.items[index];
      if (!level || row === undefined || row.kind === "separator" || row.disabled === true) return prev;
      if (row.kind === "submenu") {
        if (level.openIndex === index) return prev;
        return withSubmenu(prev, depth, index, row.items);
      }
      return [...prev.slice(0, depth), { ...level, highlighted: index }];
    });
  };

  const attachRow = (depth: number, index: number) => (el: HTMLButtonElement | null) => {
    const rows = rowEls.current[depth] ?? [];
    rows[index] = el;
    rowEls.current[depth] = rows;
    if (!el) return;
    const enter = () => hover(depth, index);
    el.addEventListener("mouseenter", enter);
    return () => el.removeEventListener("mouseenter", enter);
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const depth = levels.length - 1;
    const level = levels[depth];
    if (!level) return;
    const current = level.highlighted === null ? undefined : level.items[level.highlighted];

    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      const from = level.highlighted ?? (step > 0 ? -1 : level.items.length);
      setHighlight(depth, nextIndex(level.items, from, step));
    } else if (event.key === "ArrowRight") {
      if (current?.kind === "submenu" && level.highlighted !== null) {
        event.preventDefault();
        openSubmenu(depth, level.highlighted);
      }
    } else if (event.key === "ArrowLeft") {
      if (depth > 0) {
        event.preventDefault();
        closeSubmenu(depth);
      }
    } else if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (current?.kind === "submenu") {
        if (level.highlighted !== null) openSubmenu(depth, level.highlighted);
      } else if (current?.kind === "item") {
        props.onSelect(current.id);
        props.onClose();
      }
    } else if (event.key === "Escape") {
      if (depth > 0) closeSubmenu(depth);
      else props.onClose();
    }
  };

  const renderRow = (depth: number, level: Level, item: MenuItem, index: number, deepest: boolean) => {
    if (item.kind === "separator") {
      return <hr key={index} className="my-1 h-px border-0 bg-separator" />;
    }
    const checkable = item.kind === "item" && item.checked !== undefined;
    const checked = item.kind === "item" && item.checked === true;
    const icon = item.kind === "item" ? item.icon : undefined;
    const shortcut = item.kind === "item" ? item.shortcut : undefined;
    const disabled = item.disabled === true;
    const open = level.openIndex === index;
    const highlighted = deepest && !open && level.highlighted === index;
    const choose = () => {
      if (disabled) return;
      if (item.kind === "item") {
        props.onSelect(item.id);
        props.onClose();
      } else {
        openSubmenu(depth, index);
      }
    };
    const content = (
      <>
        <span className="flex w-[14px] shrink-0 items-center justify-center">
          {checked ? <Check size={14} /> : null}
        </span>
        {icon ? <span className="flex shrink-0 items-center">{icon}</span> : null}
        <span className="flex-1 truncate">{item.label}</span>
        {shortcut ? <span className="ml-6 shrink-0 text-secondary">{shortcut}</span> : null}
        {item.kind === "submenu" ? <ChevronRight size={14} className="shrink-0 text-tertiary" /> : null}
      </>
    );
    const common = {
      type: "button" as const,
      ref: attachRow(depth, index),
      className: rowClass(highlighted, disabled),
      "aria-disabled": disabled ? true : undefined,
      "data-highlighted": highlighted ? "true" : undefined,
      "data-open": open ? "true" : undefined,
      onClick: choose,
    };
    // ARIA allows aria-checked only on checkable roles, so checkable items
    // are menuitemcheckbox and the rest menuitem.
    if (checkable) {
      return (
        <button key={index} role="menuitemcheckbox" aria-checked={checked} {...common}>
          {content}
        </button>
      );
    }
    return (
      <button
        key={index}
        role="menuitem"
        aria-haspopup={item.kind === "submenu" ? "menu" : undefined}
        aria-expanded={item.kind === "submenu" ? open : undefined}
        {...common}
      >
        {content}
      </button>
    );
  };

  const renderMenu = (depth: number) => {
    const level = levels[depth];
    if (!level) return null;
    const deepest = depth === levels.length - 1;
    const style: CSSProperties =
      depth === 0 ? { position: "fixed", left: `${pos.left}px`, top: `${pos.top}px` } : { position: "fixed" };
    return (
      <div
        key={depth}
        ref={(el) => {
          menuEls.current[depth] = el;
          if (depth === 0) rootRef.current = el;
        }}
        role="menu"
        tabIndex={depth === 0 ? -1 : undefined}
        className="z-50 min-w-[180px] rounded-md border border-separator bg-window py-1 shadow-lg outline-none"
        style={style}
        onKeyDown={depth === 0 ? handleKeyDown : undefined}
      >
        {level.items.map((item, index) => renderRow(depth, level, item, index, deepest))}
        {level.openIndex !== null ? renderMenu(depth + 1) : null}
      </div>
    );
  };

  return createPortal(renderMenu(0), document.body);
}
