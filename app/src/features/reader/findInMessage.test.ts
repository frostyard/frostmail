// Find in the message shown (ADR-0052; docs/specs/ui.md, Find in the
// message): the matches in what the reader shows, their marks and the
// reader's scroll to the current one.
import { describe, expect, it } from "vitest";

import { clearMarks, findRanges, highlightWindows, markRanges, scrollToRange } from "./findInMessage";

const FIND = "frostmail-find";
const CURRENT = "frostmail-find-current";

/** root builds a detached tree, as the reader's scroller is. */
function root(html: string): HTMLElement {
  const el = document.createElement("div");
  el.innerHTML = html;
  return el;
}

/** inPage puts a tree in the page, which the computed styles of a test need. */
function inPage(html: string): HTMLElement {
  const el = root(html);
  document.body.append(el);
  return el;
}

/** texts is what each match covers, so a test reads the matches in order. */
function texts(el: Element, query: string): string[] {
  return findRanges(el, query).map((range) => range.toString());
}

/** framesWork is whether this environment gives a frame a document the test
 *  can write; the frame test is skipped when it does not. */
function framesWork(): boolean {
  const frame = document.createElement("iframe");
  document.body.append(frame);
  const body = frame.contentDocument?.body;
  const ok = body !== null && body !== undefined;
  frame.remove();
  return ok;
}

const FRAMES = framesWork();

/** stubRect gives an element or a range a box, as a page with layout has. */
function stubRect(target: object, rect: { top: number; height: number }): void {
  Object.defineProperty(target, "getBoundingClientRect", {
    configurable: true,
    value: () => ({ ...rect, bottom: rect.top + rect.height, left: 0, right: 0, width: 0, x: 0, y: rect.top }),
  });
}

/** scrollerOf is a reader that shows 1 screen of clientHeight, starting at
 *  top, and that has scrolled to scrollTop. */
function scrollerOf(clientHeight: number, top: number, scrollTop = 0): HTMLElement {
  const el = document.createElement("section");
  Object.defineProperty(el, "clientHeight", { configurable: true, value: clientHeight });
  el.scrollTop = scrollTop;
  stubRect(el, { top, height: clientHeight });
  return el;
}

/** rangeWith is a match with a box of its own, in the app's document or in
 *  doc's: these trees have no layout here. */
function rangeWith(rect: { top: number; height: number }, doc?: Document): Range {
  const match = {
    startContainer: { ownerDocument: doc ?? document },
    getBoundingClientRect: () => ({
      ...rect,
      bottom: rect.top + rect.height,
      left: 0,
      right: 0,
      width: 0,
      x: 0,
      y: rect.top,
    }),
  };
  return match as unknown as Range;
}

describe("findRanges", () => {
  it("finds case-insensitively", () => {
    expect(texts(root("<p>Lisbon works for me.</p>"), "LISBON")).toEqual(["Lisbon"]);
    expect(texts(root("<p>Lisbon works for me.</p>"), "wOrKs")).toEqual(["works"]);
  });

  it("finds across inline elements", () => {
    const [range] = findRanges(root("New <b>York</b>"), "new york");
    expect(range?.toString()).toBe("New York");
    expect(range?.startContainer.nodeValue).toBe("New ");
    expect(range?.startOffset).toBe(0);
    expect(range?.endContainer.nodeValue).toBe("York");
    expect(range?.endOffset).toBe(4);
  });

  it("finds every match in the order shown, and none that overlaps another", () => {
    expect(texts(root("<p>red and red</p><p>blue and red</p>"), "red")).toEqual(["red", "red", "red"]);
    expect(texts(root("<p>aaa</p>"), "aa")).toEqual(["aa"]);
  });

  it("skips style, script, template and a subtree marked to be skipped", () => {
    expect(texts(root("<style>.a{color:red}</style><p>red</p>"), "red")).toEqual(["red"]);
    expect(texts(root("<script>var red = 1</script><p>red</p>"), "red")).toEqual(["red"]);
    expect(texts(root("<template><p>red</p></template><p>red</p>"), "red")).toEqual(["red"]);
    expect(texts(root("<div data-find-skip><p>red</p></div><p>red</p>"), "red")).toEqual(["red"]);
  });

  it("finds nothing for an empty or a blank query", () => {
    expect(findRanges(root("<p>red</p>"), "")).toEqual([]);
    expect(findRanges(root("<p>red</p>"), "   ")).toEqual([]);
  });

  it("keeps the source offsets when a letter folds to more than 1 code unit", () => {
    // "\u0130" lowercases to 2 code units, so a lowered copy of the text would
    // shift every offset after it. The match still selects the source's own
    // word.
    expect(texts(root("<p>\u0130red</p>"), "red")).toEqual(["red"]);
    expect(texts(root("<p>\u0130red</p>"), "i")).toEqual(["\u0130"]);
  });

  it("skips text the page does not render", () => {
    // A newsletter's preheader is the common case of words no reader sees.
    const preheader = inPage('<div style="display:none">secret needle</div><p>needle</p>');
    expect(texts(preheader, "secret")).toEqual([]);
    expect(texts(preheader, "needle")).toEqual(["needle"]);
    preheader.remove();

    const hidden = inPage("<p hidden>needle</p><p>needle</p>");
    expect(texts(hidden, "needle")).toEqual(["needle"]);
    hidden.remove();

    const invisible = inPage('<p style="visibility:hidden">needle</p><p>needle</p>');
    expect(texts(invisible, "needle")).toEqual(["needle"]);
    invisible.remove();
  });

  it("sees the line break a br makes, and the 1 space a newline renders as", () => {
    // 2 lines are no 1 word.
    expect(texts(root("<p>red<br>blue</p>"), "redblue")).toEqual([]);
    expect(texts(root("<p>red<br>blue</p>"), "blue")).toEqual(["blue"]);
    // A newline in the markup is whitespace, which the page shows as 1 space.
    expect(texts(root("New\n<b>York</b>"), "new york")).toEqual(["New\nYork"]);
    // Whitespace runs collapse across an inline element too, and whitespace
    // at the start of a block shows nothing.
    expect(texts(root("New  <b> York</b>"), "new york")).toHaveLength(1);
    expect(texts(root("<p>  red</p>"), "red")).toEqual(["red"]);
  });

  it("reads every run of whitespace as 1 space, in the text and the query", () => {
    // 2 spaces are 1 space to the search, and a query with 2 is too.
    expect(texts(root("<pre>red  blue</pre>"), "red  blue")).toHaveLength(1);
    expect(texts(root("<pre>red  blue</pre>"), "red blue")).toHaveLength(1);
    expect(texts(root("<p>red  blue</p>"), "red  blue")).toHaveLength(1);
    const styled = inPage('<div style="white-space:pre-line">red  blue</div>');
    expect(texts(styled, "red blue")).toHaveLength(1);
    styled.remove();
  });

  it("ends a match at a line end the page keeps", () => {
    // A newline in preformatted text, and in white-space: pre-line, is a line
    // the page shows: 2 lines are no 1 word.
    expect(texts(root("<pre>red\nblue</pre>"), "red blue")).toEqual([]);
    const preLine = inPage('<div style="white-space:pre-line">red\nblue</div>');
    expect(texts(preLine, "red blue")).toEqual([]);
    preLine.remove();
    // The CSS wins over the tag: this pre keeps no newline, so the 2 lines
    // read as 1 word.
    const normal = inPage('<pre style="white-space:normal">red\nblue</pre>');
    expect(texts(normal, "red blue")).toHaveLength(1);
    normal.remove();
    // A tab is whitespace, not a line end, wherever it sits.
    expect(texts(root("<pre>red\tblue</pre>"), "red blue")).toHaveLength(1);
  });

  it("makes a final sigma the same letter as a sigma", () => {
    expect(texts(root("<p>\u039F\u03A3</p>"), "\u03BF\u03C2")).toEqual(["\u039F\u03A3"]);
    expect(texts(root("<p>\u03BF\u03C2</p>"), "\u039F\u03A3")).toEqual(["\u03BF\u03C2"]);
  });

  it("sees the words a hidden br leaves on 1 line", () => {
    // The page hides this br, so its 2 words are 1 line, and 1 word.
    const hidden = inPage('<p>red<br style="display:none">blue</p>');
    expect(texts(hidden, "redblue")).toEqual(["redblue"]);
    hidden.remove();
  });

  it("finds 1 match in each of 5,000 paragraphs", () => {
    const el = document.createElement("div");
    for (let i = 0; i < 5000; i += 1) {
      const p = document.createElement("p");
      p.textContent = `paragraph ${i} holds the word needle in it`;
      el.append(p);
    }
    // Each match is 1 lookup in the joined text's map, so 1 long message is
    // still 1 pass.
    expect(findRanges(el, "needle")).toHaveLength(5000);
  });

  it.skipIf(!FRAMES)("searches a frame's body at the frame's place", () => {
    const el = root("<p>alpha</p>");
    // A frame reads its document only in the page it is part of.
    document.body.append(el);
    const frame = document.createElement("iframe");
    el.append(frame);
    const body = frame.contentDocument?.body;
    if (body === null || body === undefined) throw new Error("no frame body");
    body.innerHTML = "<p>beta</p>";
    const after = document.createElement("p");
    after.textContent = "gamma";
    el.append(after);
    const ranges = findRanges(el, "a");
    expect(ranges.map((range) => range.toString())).toEqual(["a", "a", "a"]);
    // The app's document, the frame's, then the app's again: the order shown.
    expect(ranges.map((range) => range.startContainer.ownerDocument === document)).toEqual([true, false, true]);
    el.remove();
  });
});

/** FakeHighlight stands in for the CSS Custom Highlight API's Highlight. */
class FakeHighlight {
  readonly ranges: Range[];

  constructor(...ranges: Range[]) {
    this.ranges = ranges;
  }
}

/** fakeWindow is a window with the API, or without it when registry is null. */
function fakeWindow(registry: Map<string, unknown> | null): Window {
  const scope = registry === null ? {} : { CSS: { highlights: registry }, Highlight: FakeHighlight };
  return scope as unknown as Window;
}

/** fakeRange is a match that lives in win, for the marks' grouping. */
function fakeRange(win: Window): Range {
  return { startContainer: { ownerDocument: { defaultView: win } } } as unknown as Range;
}

describe("markRanges", () => {
  it("marks every match and the current one in its own window", () => {
    const registry = new Map<string, unknown>();
    const win = fakeWindow(registry);
    const first = fakeRange(win);
    const second = fakeRange(win);

    markRanges([first, second], first);
    const all = registry.get(FIND) as FakeHighlight | undefined;
    expect(all?.ranges).toHaveLength(2);
    expect(all?.ranges[0]).toBe(first);
    expect(all?.ranges[1]).toBe(second);
    const current = registry.get(CURRENT) as FakeHighlight | undefined;
    expect(current?.ranges).toHaveLength(1);
    expect(current?.ranges[0]).toBe(first);

    // A move marks the new current one only.
    markRanges([first, second], second);
    expect((registry.get(CURRENT) as FakeHighlight | undefined)?.ranges[0]).toBe(second);

    // No current match takes that highlight away.
    markRanges([first, second], null);
    expect(registry.has(CURRENT)).toBe(false);
    expect(registry.has(FIND)).toBe(true);

    const windows = highlightWindows([first, second]);
    expect(windows).toHaveLength(1);
    expect(windows[0]).toBe(win);

    clearMarks([win]);
    expect(registry.size).toBe(0);
  });

  it("does nothing, and does not throw, without the API", () => {
    const win = fakeWindow(null);
    const range = fakeRange(win);
    expect(() => markRanges([range], range)).not.toThrow();
    expect(() => clearMarks([win])).not.toThrow();
  });
});

describe("scrollToRange", () => {
  it("puts a match that is out of view in the middle, and leaves a visible one alone", () => {
    const scroller = scrollerOf(400, 200, 100);
    scrollToRange(rangeWith({ top: 900, height: 20 }), scroller);
    // 700 down the scroller's own top, centred in 400: 100 + 700 - 190.
    expect(scroller.scrollTop).toBe(610);

    scrollToRange(rangeWith({ top: 250, height: 20 }), scroller);
    expect(scroller.scrollTop).toBe(610);
  });

  it("counts a frame's place for a match inside the frame", () => {
    const scroller = scrollerOf(400, 0, 0);
    const frame = document.createElement("div");
    stubRect(frame, { top: 500, height: 300 });
    const frameDoc = { defaultView: { frameElement: frame } } as unknown as Document;
    scrollToRange(rangeWith({ top: 10, height: 20 }, frameDoc), scroller);
    // 510 below the top window, centred in 400: 510 - 190.
    expect(scroller.scrollTop).toBe(320);
  });
});
