// The compose window's toolbar and its format bar
// (docs/specs/compose-ui.md, Layout: Toolbar and Format bar). Task T-0045
// implements both; the stubs draw empty strips.

/** ComposeCommand is what a compose toolbar control asks for. */
export type ComposeCommand = "send" | "attach" | "toggleFormatBar" | "delete" | "minimize" | "toggleMaximize" | "close";

/** ComposeToolbarProps are the compose toolbar's inputs. */
export interface ComposeToolbarProps {
  /** The draft's subject; the title shows "New Message" when it is blank. */
  subject: string;
  /** The draft can be sent (spec, Behavior: Saving and sending). */
  canSend: boolean;
  formatBarShown: boolean;
  maximized: boolean;
  onCommand: (command: ComposeCommand) => void;
}

/** ComposeToolbar is the compose window's top strip. */
export function ComposeToolbar(_props: ComposeToolbarProps) {
  return <div role="toolbar" aria-label="Compose" />;
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

/** FormatBar is the row of formatting controls above the editor. */
export function FormatBar(_props: FormatBarProps) {
  return <div role="toolbar" aria-label="Format" />;
}
