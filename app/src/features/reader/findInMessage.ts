// Find in the message shown (ADR-0052; docs/specs/ui.md, Find in the
// message): the matches in what the reader shows, the CSS Custom Highlight
// API's marks behind them, and the reader's scroll to the current match.
//
// The search reads the text the page renders, not the text its DOM holds: a
// message's markup carries words no reader sees (a newsletter's preheader in
// display:none, a hidden block, a template, an alternate part), and its
// whitespace collapses where the CSS says so. So the joined text is built
// from what shows, and each of its code units remembers the text node and
// the source offsets it came from, which a Range needs.
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

// These 2 carry the browser's own white-space: pre, whatever the stylesheets
// of the document say.
const PRE_TAGS = new Set(["pre", "textarea"]);

const BREAK = "\n";
const NBSP = "\u00A0";

/** Unit is where 1 code unit of the joined text came from: its text node, and
 *  the code unit offsets of the source code point, as a Range takes them. A
 *  break has no unit, so no match may cross it. */
interface Unit {
  node: Text;
  start: number;
  end: number;
}

/** Joined is the rendered text, and where each of its code units came from. */
interface Joined {
  text: string;
  units: (Unit | null)[];
}

/** TextItem is 1 text node's place in the rendered text; BreakItem is a place
 *  where the text breaks into another line. */
type Item = { kind: "text"; node: Text; text: string; block: Node | null; pre: boolean } | { kind: "break" };

/** isSpace is the whitespace a page collapses into 1 space. */
function isSpace(cp: string): boolean {
  return cp === " " || cp === "\t" || cp === "\n" || cp === "\r" || cp === "\f";
}

/** styleOf is an element's computed style in its own window: a frame's
 *  document has its own, and a tree with no document has none. */
function styleOf(element: Element): CSSStyleDeclaration | null {
  const win = element.ownerDocument.defaultView;
  return win === null ? null : win.getComputedStyle(element);
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

/** collect appends the text the page renders, in document order, entering a
 *  frame the app may read at the frame's place. Text that renders nothing is
 *  no part of it: a display:none or hidden subtree, a style, script,
 *  noscript or template, and text under a hidden or collapsed visibility. */
function collect(node: Node, block: Node | null, pre: boolean, visibility: string, items: Item[]): void {
  for (let child = node.firstChild; child !== null; child = child.nextSibling) {
    if (child.nodeType === 3) {
      if (visibility === "hidden" || visibility === "collapse") continue;
      items.push({ kind: "text", node: child as Text, text: child.nodeValue ?? "", block, pre });
      continue;
    }
    if (child.nodeType !== 1) continue;
    const element = child as Element;
    if (element.hasAttribute("data-find-skip") || element.hasAttribute("hidden")) continue;
    const tag = element.tagName.toLowerCase();
    if (SKIP_TAGS.has(tag)) continue;
    if (tag === "br") {
      items.push({ kind: "break" });
      continue;
    }
    if (tag === "iframe") {
      // A body is a block of its own: a match never runs into the frame or
      // out of it.
      const body = frameBody(element);
      if (body !== null) collect(body, body, false, "visible", items);
      continue;
    }
    const style = styleOf(element);
    // display:none takes the element, and every word in it, out of the page.
    if (style?.display === "none") continue;
    const whiteSpace = style?.whiteSpace ?? "";
    const visibilityOwn = style?.visibility ?? "";
    // Whitespace is kept where the page keeps it: an element the page marks
    // as preformatted, or 1 the document's styles say nothing about, under a
    // preformatted ancestor.
    const ownPre = PRE_TAGS.has(tag) || whiteSpace.startsWith("pre");
    collect(
      element,
      BLOCK_TAGS.has(tag) ? element : block,
      whiteSpace === "" ? pre || ownPre : ownPre,
      visibilityOwn === "" ? visibility : visibilityOwn,
      items,
    );
  }
}

/**
 * joined joins the text the page renders, in the order it shows it, and keeps
 * where each of its code units came from: the source of a match, so a Range
 * can select it. Whitespace collapses to 1 space as the page shows it, 1
 * space stands where a block or a br ends, and preformatted text keeps its
 * own whitespace.
 */
function joined(root: Element): Joined {
  const items: Item[] = [];
  collect(root, BLOCK_TAGS.has(root.tagName.toLowerCase()) ? root : null, false, "visible", items);
  const units: (Unit | null)[] = [];
  let text = "";
  // 1 run of whitespace waiting for the 1 space it renders as, and whether
  // the block has shown nothing yet: whitespace at its start renders nothing.
  let pending: Unit | null = null;
  let atStart = true;
  let block: Node | null = null;

  const emit = (unit: Unit | null, chars: string) => {
    for (let i = 0; i < chars.length; i += 1) {
      text += chars.charAt(i);
      units.push(unit);
    }
    atStart = false;
  };
  const flush = () => {
    const space = pending;
    pending = null;
    if (space === null || atStart) return;
    emit(space, " ");
  };
  const lineBreak = () => {
    // Whitespace at the end of a line renders nothing either.
    pending = null;
    if (text !== "") emit(null, BREAK);
    atStart = true;
  };

  for (const item of items) {
    if (item.kind === "break") {
      lineBreak();
      continue;
    }
    if (item.block !== block) {
      lineBreak();
      block = item.block;
    }
    if (item.text === "") continue;
    // A space before preformatted text still shows.
    if (item.pre) flush();
    for (let i = 0; i < item.text.length; ) {
      const cp = item.text.codePointAt(i) ?? 0;
      const chars = String.fromCodePoint(cp);
      const unit: Unit = { node: item.node, start: i, end: i + chars.length };
      i += chars.length;
      if (item.pre) {
        emit(unit, chars === NBSP ? " " : chars.toLowerCase());
        continue;
      }
      if (isSpace(chars)) {
        if (pending === null) pending = unit;
        continue;
      }
      if (chars === NBSP) {
        // A nbsp is the 1 space the page never collapses.
        flush();
        emit(unit, " ");
        continue;
      }
      flush();
      // A letter may fold to more than 1 code unit ("İ"); each of them is
      // this same code point of the source.
      emit(unit, chars.toLowerCase());
    }
  }
  return { text, units };
}

/** needleOf normalizes a query as the page normalizes text: trimmed,
 *  whitespace runs of 1 space, and folded to lower case. */
function needleOf(query: string): string {
  let text = "";
  let space = false;
  for (const cp of query) {
    if (isSpace(cp) || cp === NBSP) {
      space = true;
      continue;
    }
    if (space && text !== "") text += " ";
    space = false;
    text += cp.toLowerCase();
  }
  return text;
}

/** crossesBreak reports whether a match from..to spans a line break, which no
 *  match may do: the page shows 2 lines there, not 1 word. */
function crossesBreak(units: readonly (Unit | null)[], from: number, to: number): boolean {
  for (let i = from; i < to; i += 1) if (units[i] === null) return true;
  return false;
}

/**
 * findRanges finds a query in everything root renders, in the order it shows
 * it, case-insensitively and across inline elements ("New <b>York</b>"
 * matches "new york"). Matches do not overlap, and none crosses a line
 * break. A frame the app may read is searched at its place, so matches run
 * through the reader top to bottom.
 */
export function findRanges(root: Element, query: string): Range[] {
  const needle = needleOf(query);
  if (needle === "") return [];
  const { text, units } = joined(root);
  const ranges: Range[] = [];
  let at = text.indexOf(needle);
  while (at !== -1) {
    const to = at + needle.length;
    // The next match starts after this 1: matches never overlap.
    const next = text.indexOf(needle, to);
    if (!crossesBreak(units, at, to)) {
      const first = units[at];
      const last = units[to - 1];
      const doc = first?.node.ownerDocument ?? root.ownerDocument;
      if (first && last && doc !== null) {
        const range = doc.createRange();
        range.setStart(first.node, first.start);
        range.setEnd(last.node, last.end);
        ranges.push(range);
      }
    }
    at = next;
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
