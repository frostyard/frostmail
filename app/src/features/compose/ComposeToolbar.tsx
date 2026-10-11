// The compose window's toolbar and its format bar
// (docs/specs/compose-ui.md, Layout: Toolbar and Format bar). Both are
// presentational: the container wires them to maild and to the TipTap
// editor, and every control only reports the command it asks for.

import {
  Bold,
  ChevronDown,
  Copy,
  Italic,
  Link,
  List,
  ListOrdered,
  Minus,
  Paperclip,
  RemoveFormatting,
  Send,
  Square,
  Strikethrough,
  TextQuote,
  Trash2,
  Type,
  Underline,
  X,
} from "lucide-react";
import { type ReactNode, useState } from "react";

import type { LaterChoice } from "../../lib/later";
import { ContextMenu, type MenuItem } from "../menu/ContextMenu";

/** ComposeCommand is what a compose toolbar control asks for. */
export type ComposeCommand =
  | "send"
  | "sendLater"
  | "attach"
  | "toggleFormatBar"
  | "delete"
  | "minimize"
  | "toggleMaximize"
  | "close";

/** ComposeToolbarProps are the compose toolbar's inputs. */
export interface ComposeToolbarProps {
  /** The draft's subject; the title shows "New Message" when it is blank. */
  subject: string;
  /** The draft can be sent (spec, Behavior: Saving and sending). */
  canSend: boolean;
  formatBarShown: boolean;
  maximized: boolean;
  laterChoices?: LaterChoice[];
  onSendAt?: (at: Date) => void;
  onCommand: (command: ComposeCommand) => void;
}

// The main toolbar's button style, with Send drawn in the accent color.
// Send never carries text-secondary: two text colors in one class list
// would make the result depend on CSS order.
const BUTTON_CLASS =
  "flex size-7 items-center justify-center rounded-md text-secondary hover:bg-selection-inactive disabled:opacity-40";
const SEND_CLASS =
  "flex size-7 items-center justify-center rounded-md text-accent hover:bg-selection-inactive disabled:opacity-40";

interface ComposeButtonProps {
  name: string;
  title: string;
  icon: ReactNode;
  command: ComposeCommand;
  pressed?: boolean;
  disabled?: boolean;
  accent?: boolean;
  onCommand: (command: ComposeCommand) => void;
}

function ComposeButton(props: ComposeButtonProps) {
  const { name, title, icon, command, pressed, disabled, accent, onCommand } = props;
  return (
    <button
      type="button"
      aria-label={name}
      title={title}
      aria-pressed={pressed}
      disabled={disabled}
      onClick={() => onCommand(command)}
      className={accent ? SEND_CLASS : BUTTON_CLASS}
    >
      {icon}
    </button>
  );
}

function SendLaterButton(props: ComposeToolbarProps) {
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const choices = props.laterChoices ?? [];
  const items: MenuItem[] = choices.map((choice, index) => ({
    kind: "item",
    id: String(index),
    label: choice.label,
  }));
  items.push({ kind: "separator" }, { kind: "item", id: "sendLater", label: "Send Later…" });
  return (
    <>
      <button
        type="button"
        aria-label="Send Later"
        title="Send Later"
        aria-haspopup="menu"
        disabled={!props.canSend}
        className="flex h-7 w-4 items-center justify-center rounded-md text-accent hover:bg-selection-inactive disabled:opacity-40"
        onClick={(event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          setMenu({ x: rect.left, y: rect.bottom });
        }}
      >
        <ChevronDown size={12} />
      </button>
      {menu && (
        <ContextMenu
          items={items}
          x={menu.x}
          y={menu.y}
          onSelect={(id) => {
            if (id === "sendLater") props.onCommand("sendLater");
            else {
              const choice = choices[Number(id)];
              if (choice) props.onSendAt?.(choice.at);
            }
          }}
          onClose={() => setMenu(null)}
        />
      )}
    </>
  );
}

/** ComposeToolbar is the compose window's title bar: Send, the draft's
 * title in the drag region, its actions and the window controls. */
export function ComposeToolbar(props: ComposeToolbarProps) {
  const title = props.subject.trim() === "" ? "New Message" : props.subject;
  return (
    <div
      role="toolbar"
      aria-label="Compose"
      data-tauri-drag-region
      className="flex h-[52px] shrink-0 items-center gap-1 border-b border-separator bg-toolbar px-2"
    >
      <ComposeButton
        name="Send"
        title="Send (Ctrl+Enter)"
        icon={<Send size={16} />}
        command="send"
        accent
        disabled={!props.canSend}
        onCommand={props.onCommand}
      />
      <SendLaterButton {...props} />
      <span data-tauri-drag-region className="min-w-0 flex-1 truncate px-2 text-toolbar-title">
        {title}
      </span>
      <ComposeButton
        name="Attach Files"
        title="Attach Files (Ctrl+Shift+A)"
        icon={<Paperclip size={16} />}
        command="attach"
        onCommand={props.onCommand}
      />
      <ComposeButton
        name="Show Format Bar"
        title="Show Format Bar"
        icon={<Type size={16} />}
        command="toggleFormatBar"
        pressed={props.formatBarShown}
        onCommand={props.onCommand}
      />
      <ComposeButton
        name="Delete Draft"
        title="Delete Draft (Ctrl+Backspace)"
        icon={<Trash2 size={16} />}
        command="delete"
        onCommand={props.onCommand}
      />
      <ComposeButton
        name="Minimize"
        title="Minimize"
        icon={<Minus size={16} />}
        command="minimize"
        onCommand={props.onCommand}
      />
      {props.maximized ? (
        <ComposeButton
          name="Restore"
          title="Restore"
          icon={<Copy size={16} />}
          command="toggleMaximize"
          onCommand={props.onCommand}
        />
      ) : (
        <ComposeButton
          name="Maximize"
          title="Maximize"
          icon={<Square size={16} />}
          command="toggleMaximize"
          onCommand={props.onCommand}
        />
      )}
      <ComposeButton name="Close" title="Close" icon={<X size={16} />} command="close" onCommand={props.onCommand} />
    </div>
  );
}

/** FormatKey names a mark or node a format toggle shows and switches. */
export type FormatKey =
  | "bold"
  | "italic"
  | "underline"
  | "strike"
  | "bulletList"
  | "orderedList"
  | "blockquote"
  | "link";

/** FormatCommand is what a format bar control asks for. */
export type FormatCommand = FormatKey | "clear";

/** FormatBarProps are the format bar's inputs. */
export interface FormatBarProps {
  /** Which marks and nodes are active at the selection; missing is false. */
  active: Partial<Record<FormatKey, boolean>>;
  onCommand: (command: FormatCommand) => void;
}

interface FormatToggle {
  name: string;
  key: FormatKey;
  title: string;
  icon: ReactNode;
}

// The toggles in their three groups, split by a separator rule.
const FORMAT_GROUPS: FormatToggle[][] = [
  [
    { name: "Bold", key: "bold", title: "Bold (Ctrl+B)", icon: <Bold size={16} /> },
    { name: "Italic", key: "italic", title: "Italic (Ctrl+I)", icon: <Italic size={16} /> },
    { name: "Underline", key: "underline", title: "Underline (Ctrl+U)", icon: <Underline size={16} /> },
    { name: "Strikethrough", key: "strike", title: "Strikethrough", icon: <Strikethrough size={16} /> },
  ],
  [
    { name: "Bulleted List", key: "bulletList", title: "Bulleted List", icon: <List size={16} /> },
    { name: "Numbered List", key: "orderedList", title: "Numbered List", icon: <ListOrdered size={16} /> },
    { name: "Quote", key: "blockquote", title: "Quote", icon: <TextQuote size={16} /> },
  ],
  [{ name: "Link", key: "link", title: "Link (Ctrl+K)", icon: <Link size={16} /> }],
];

interface FormatControlProps {
  name: string;
  title: string;
  icon: ReactNode;
  command: FormatCommand;
  /** A toggle reports aria-pressed; Clear Formatting is not one. */
  toggle: boolean;
  pressed?: boolean;
  onCommand: (command: FormatCommand) => void;
}

function FormatControl(props: FormatControlProps) {
  const { name, title, icon, command, toggle, pressed, onCommand } = props;
  return (
    <button
      type="button"
      aria-label={name}
      title={title}
      aria-pressed={toggle ? pressed === true : undefined}
      onMouseDown={(event) => event.preventDefault()}
      onClick={() => onCommand(command)}
      className={`flex h-6 w-7 items-center justify-center rounded ${
        pressed ? "bg-selection-inactive text-primary" : "text-secondary hover:bg-selection-inactive"
      }`}
    >
      {icon}
    </button>
  );
}

/** FormatBar is the row of formatting controls above the editor: toggles
 * for the marks and nodes at the selection, then Clear Formatting. Its
 * controls never take focus from the editor. */
export function FormatBar(props: FormatBarProps) {
  return (
    <div
      role="toolbar"
      aria-label="Format"
      className="flex h-8 shrink-0 items-center gap-0.5 border-b border-separator bg-window px-2"
    >
      {FORMAT_GROUPS.map((group, index) => (
        <div className="contents" key={group[0]?.key ?? index}>
          {group.map((control) => (
            <FormatControl
              key={control.key}
              name={control.name}
              title={control.title}
              icon={control.icon}
              command={control.key}
              toggle
              pressed={props.active[control.key] === true}
              onCommand={props.onCommand}
            />
          ))}
          {index < FORMAT_GROUPS.length - 1 ? <span aria-hidden="true" className="mx-1 h-4 w-px bg-separator" /> : null}
        </div>
      ))}
      <FormatControl
        name="Clear Formatting"
        title="Clear Formatting"
        icon={<RemoveFormatting size={16} />}
        command="clear"
        toggle={false}
        onCommand={props.onCommand}
      />
    </div>
  );
}
