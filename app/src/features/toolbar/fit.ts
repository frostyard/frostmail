/**
 * fitItems returns how many leading items fit beside an overflow button.
 *
 * An unmeasured container (available <= 0) shows every item, as does one
 * with room for all of them and the gaps between them. Otherwise it returns
 * the longest leading run that fits with the overflow button, or 0 when even
 * that button alone does not fit. There is one gap between each shown item
 * and the next, and so one gap before the overflow button.
 */
export function fitItems(available: number, widths: readonly number[], overflowWidth: number, gap: number): number {
  if (available <= 0) return widths.length;
  const total = widths.reduce((sum, width) => sum + width, 0);
  if (total + gap * Math.max(0, widths.length - 1) <= available) return widths.length;
  for (let count = widths.length - 1; count >= 0; count--) {
    const shown = widths.slice(0, count).reduce((sum, width) => sum + width, 0);
    if (shown + overflowWidth + gap * count <= available) return count;
  }
  return 0;
}
