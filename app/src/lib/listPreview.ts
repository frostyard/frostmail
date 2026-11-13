// The message list's preview lines (docs/specs/ui.md, Message list; P-211).
import type { Settings } from "../rpc/gen/api";

/** PREVIEW_LINES is how many lines of preview a row shows until the user chooses. */
export const PREVIEW_LINES = 2;

/** previewLines is the user's choice of preview lines, 0 to 5, else 2. */
export function previewLines(settings: Settings | null | undefined): number {
  const lines = settings?.previewLines;
  return lines === undefined || !Number.isInteger(lines) || lines < 0 || lines > 5 ? PREVIEW_LINES : lines;
}

/** rowHeight is a list row's height with lines of preview: 84 for 2, and 16 more or less for each other. */
export function rowHeight(lines: number): number {
  return 84 + (Math.max(lines, 0) - PREVIEW_LINES) * 16;
}
