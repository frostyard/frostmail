// Find in the message shown (ADR-0052; docs/specs/ui.md, Find in the
// message): the matches in what the reader shows, the CSS Custom Highlight
// API's marks behind them, and the reader's scroll to the current match.
//
// Everything here is plain DOM, so a message's markup stays untouched: a
// highlight changes no node, which the frame's height and the print would
// see. A frame's document is reached from the app's side, as the frame's
// sizing already is, and the frame still runs no script.

// The CSS Custom Highlight API's 2 names: every match, and the match shown
// now, in each window that holds one.
const FIND = "frostmail-find";
const FIND_CURRENT = "frostmail-find-current";

// A tree's own text is not the message's: style and script hold no words the
// reader shows, and a template's content never shows at all.
const SKIP_TAGS = new Set(["style", "script", "noscript", "template"]);

// Two blocks hold 2 pieces of text that do not touch: "Hi" and "there" of 2
// paragraphs are not "Hithere", so a match stops where the text of one block
// ends and another's starts.
const BLOCK_TAGS = new Set([
  "address",
  "article",
  "aside",
  "blockquote",
  "body",
  "dd",
  "div",
  "dl",
  "dt",
  "figcaption",
  "figure",
  "footer",
  "form",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "header",
  "hr",
  "html",
  "li",
  "main",
  "nav",
  "ol",
  "p",
  "pre",
  "section",
  "table",
  "tbody",
  "td",
  "tfoot",
  "th",
  "thead",
  "tr",
  "ul",
]);

const BLOCK_BREAK = "\n";

/** Piece is one text node of the tree, in the order the reader shows it. */
interface Piece {
  node: Text;
  text: string;
  /** The nearest block the piece's text belongs to. */
  block: Node | null;
}

/** Segment is a piece's place in the tree's joined text. */
interface Segment {
  node: Text;
  /** Where the node's text starts in the joined text. */
  start: number;
  length: number;
}

/** Point is a place in a text node, as a Range takes it. */
interface Point {
  node: Text;
  offset: number;
}

/** frameBody is the document's body of a frame the app may read: a frame
 *  from another origin, as lodge's web build shows, has none. */
function frameBody(frame: Element): Element | null {
  try {
    return (frame as HTMLIFrameElement).contentDocument?.body ?? null;
  } catch {
    return null;
  }
}

/** collect appends the tree's text nodes in document order, entering a
 *  reachable frame at the frame's place, and skipping what no match may
 *  touch. */
function collect(node: Node, block: Node | null, pieces: Piece[]): void {
  for (let child = node.firstChild; child !== null; child = child.nextSibling) {
    if (child.nodeType === 3) {
      pieces.push({ node: child as Text, text: child.nodeValue ?? "", block });
      continue;
    }
    if (child.nodeType !== 1) continue;
    const element = child as Element;
    if (element.hasAttribute("data-find-skip")) continue;
    const tag = element.tagName.toLowerCase();
    if (SKIP_TAGS.has(tag)) continue;
    if (tag === "iframe") {
      // A body is a block of its own: a match never runs into the frame or
      // out of it.
      const body = frameBody(element);
      if (body !== null) collect(body, body, pieces);
      continue;
    }
    collect(element, BLOCK_TAGS.has(tag) ? element : block, pieces);
  }
}

/** joined joins the tree's text in the order the reader shows it, and keeps
 *  where each text node's words are, so an offset maps back to a node. */
function joined(root: Element): { segments: Segment[]; text: string } {
  const pieces: Piece[] = [];
  collect(root, BLOCK_TAGS.has(root.tagName.toLowerCase()) ? root : null, pieces);
  const segments: Segment[] = [];
  let text = "";
  let block: Node | null = null;
  for (const piece of pieces) {
    if (piece.text === "") continue;
    if (segments.length > 0 && piece.block !== block) text += BLOCK_BREAK;
    segments.push({ node: piece.node, start: text.length, length: piece.text.length });
    text += piece.text;
    block = piece.block;
  }
  return { segments, text };
}

/** point maps an offset of the joined text back to a text node, clamped to
 *  the node it lands in or before; null when no text node holds it. */
function point(segments: readonly Segment[], offset: number): Point | null {
  const last = segments[segments.length - 1];
  if (last === undefined) return null;
  for (const segment of segments) {
    if (offset <= segment.start + segment.length)
      return { node: segment.node, offset: Math.max(0, Math.min(offset - segment.start, segment.length)) };
  }
  return { node: last.node, offset: last.length };
}

/**
 * findRanges finds a query in everything root shows, in the order it shows
 * it, case-insensitively and across inline elements ("New <b>York</b>"
 * matches "new york"). Matches do not overlap. A frame the app may read is
 * searched at its place, so matches run through the reader top to bottom.
 */
export function findRanges(root: Element, query: string): Range[] {
  const needle = query.trim().toLowerCase();
  if (needle === "") return [];
  const { segments, text } = joined(root);
  if (text === "") return [];
  const haystack = text.toLowerCase();
  const ranges: Range[] = [];
  let at = haystack.indexOf(needle);
  while (at !== -1) {
    const start = point(segments, at);
    const end = point(segments, at + needle.length);
    const doc = start?.node.ownerDocument ?? root.ownerDocument;
    if (start !== null && end !== null && doc !== null) {
      const range = doc.createRange();
      range.setStart(start.node, start.offset);
      range.setEnd(end.node, end.offset);
      ranges.push(range);
    }
    at = haystack.indexOf(needle, at + needle.length);
  }
  return ranges;
}

/** HighlightScope is the part of a window the marks need: the CSS Custom
 *  Highlight API, which not every browser has. */
interface HighlightScope {
  CSS?: { highlights?: Map<string, unknown> };
  Highlight?: new (...ranges: Range[]) => unknown;
}

function windowOf(range: Range): Window | null {
  return range.startContainer.ownerDocument?.defaultView ?? null;
}

/** highlightWindows is the windows a set of matches lives in: the app's
 *  document, and each frame's own, which keeps its own highlights. */
export function highlightWindows(ranges: readonly Range[]): Window[] {
  const windows: Window[] = [];
  for (const range of ranges) {
    const win = windowOf(range);
    if (win !== null && !windows.includes(win)) windows.push(win);
  }
  return windows;
}

/**
 * markRanges marks every match in its own window and the current one more
 * strongly. Without the API it does nothing: the bar still counts and
 * scrolls then, and marks nothing.
 */
export function markRanges(all: readonly Range[], current: Range | null): void {
  const groups = new Map<Window, Range[]>();
  for (const range of all) {
    const win = windowOf(range);
    if (win === null) continue;
    const group = groups.get(win) ?? [];
    group.push(range);
    groups.set(win, group);
  }
  const currentWindow = current === null ? null : windowOf(current);
  for (const [win, ranges] of groups) marksIn(win, ranges, win === currentWindow ? current : null);
}

function marksIn(win: Window, ranges: readonly Range[], current: Range | null): void {
  const api = win as unknown as HighlightScope;
  const highlights = api.CSS?.highlights;
  const Highlight = api.Highlight;
  if (highlights === undefined || Highlight === undefined) return;
  highlights.set(FIND, new Highlight(...ranges));
  if (current === null) highlights.delete(FIND_CURRENT);
  else highlights.set(FIND_CURRENT, new Highlight(current));
}

/** clearMarks takes both highlights away in each window the bar marked. */
export function clearMarks(windows: Iterable<Window>): void {
  for (const win of windows) {
    const highlights = (win as unknown as HighlightScope).CSS?.highlights;
    highlights?.delete(FIND);
    highlights?.delete(FIND_CURRENT);
  }
}

/** frameTop is how far a frame's page sits below the top window's top, for
 *  a match inside a frame; 0 in the app's own document. */
function frameTop(doc: Document | null): number {
  const frame = doc?.defaultView?.frameElement;
  if (!frame) return 0;
  return frame.getBoundingClientRect().top + frameTop(frame.ownerDocument);
}

/**
 * scrollToRange brings the current match into view: the reader scrolls only
 * when the match is not wholly visible, and then puts it in the middle of
 * the reader, so the lines around it show too.
 */
export function scrollToRange(range: Range, scroller: HTMLElement): void {
  // A Range is asked for its box; a document without layout has none.
  if (typeof range.getBoundingClientRect !== "function") return;
  const rect = range.getBoundingClientRect();
  const top = rect.top + frameTop(range.startContainer.ownerDocument) - scroller.getBoundingClientRect().top;
  if (top >= 0 && top + rect.height <= scroller.clientHeight) return;
  const middle = Math.max(0, (scroller.clientHeight - rect.height) / 2);
  scroller.scrollTop = Math.max(0, scroller.scrollTop + top - middle);
}
