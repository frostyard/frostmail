// Mail.app's flag colors (docs/specs/ui.md, Tokens: Flags).

/** FLAG_NAMES names flag colors 1-7; FLAG_NAMES[color - 1]. */
export const FLAG_NAMES = ["Red", "Orange", "Yellow", "Green", "Blue", "Purple", "Gray"] as const;

/** flagName names a flag color, or "" for 0 and unknown colors. */
export function flagName(color: number): string {
  return FLAG_NAMES[color - 1] ?? "";
}
