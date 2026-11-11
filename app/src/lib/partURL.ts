// URLs of the parts cache (docs/specs/rpc-protocol.md): maild writes
// mailpart://localhost/<path> (render.PartURL) on every platform. On Windows
// the app turns maild's URLs into the http://mailpart.localhost/ form for the
// page (ADR-0038), and back into maild's when HTML returns to maild (a draft).

/** PART_URL is the prefix maild writes. */
export const PART_URL = "mailpart://localhost/";

/** WINDOWS_PART_URL is the same parts on Windows. */
export const WINDOWS_PART_URL = "http://mailpart.localhost/";

/** onWindows reports whether the app runs on Windows. */
export const onWindows = typeof navigator !== "undefined" && /\bWindows\b/.test(navigator.userAgent);

/** toPageParts gives HTML from maild the parts URLs the page loads. */
export function toPageParts(html: string, windows = onWindows): string {
  return windows ? html.replaceAll(PART_URL, WINDOWS_PART_URL) : html;
}

/** toMaildParts gives HTML for maild its own parts URLs back. */
export function toMaildParts(html: string, windows = onWindows): string {
  return windows ? html.replaceAll(WINDOWS_PART_URL, PART_URL) : html;
}

/** isPartURL reports whether src names the parts cache, as the page loads it. */
export function isPartURL(src: string, windows = onWindows): boolean {
  const s = src.toLowerCase();
  return s.startsWith("mailpart:") || (windows && s.startsWith(WINDOWS_PART_URL));
}
