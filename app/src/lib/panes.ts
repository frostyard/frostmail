/**
 * panePercent is a pane's width as its splitter's aria-valuenow: its share
 * of the window, in whole percent (docs/specs/ui.md, Layout: Panes).
 */
export function panePercent(width: number, total = globalThis.window?.innerWidth ?? 0): number {
  if (total <= 0) return 0;
  return Math.round((width / total) * 100);
}
