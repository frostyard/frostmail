import { afterEach, describe, expect, it, vi } from "vitest";

import { readQuoteSelection } from "./quoteSelection";

function body(id: number, text: string, marked = true): HTMLElement {
  const el = document.createElement("div");
  if (marked) el.dataset.quoteSource = String(id);
  el.textContent = text;
  document.body.append(el);
  return el;
}

function select(start: Node, end = start): void {
  const range = document.createRange();
  range.setStart(start, 0);
  range.setEnd(end, end.textContent?.length ?? 0);
  document.getSelection()?.removeAllRanges();
  document.getSelection()?.addRange(range);
}

function frame(id: number, text: string, marked = true): HTMLIFrameElement {
  const el = document.createElement("iframe");
  if (marked) el.dataset.quoteSource = String(id);
  document.body.append(el);
  // jsdom does not load srcdoc; only the selection API matters here.
  if (!el.contentWindow) throw new Error("frame has no window");
  vi.spyOn(el.contentWindow, "getSelection").mockReturnValue({ toString: () => text } as Selection);
  return el;
}

afterEach(() => {
  document.getSelection()?.removeAllRanges();
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe("readQuoteSelection", () => {
  it("reads the first marked frame with a non-blank selection", () => {
    frame(9, "not a reader frame", false);
    frame(1, " \n\t ");
    frame(2, "selected < & text");
    frame(3, "later");
    expect(readQuoteSelection(document, false)).toEqual({ messageId: 2, text: "selected < & text" });
  });

  it("reads text only when both ends are in the same plain-text body", () => {
    const el = body(4, "selected text");
    const text = el.firstChild;
    if (!text) throw new Error("missing text");
    select(text);
    expect(readQuoteSelection(document, false)).toEqual({ messageId: 4, text: "selected text" });
  });

  it("rejects a selection in the headers", () => {
    const el = body(1, "header", false).firstChild;
    if (!el) throw new Error("missing text");
    select(el);
    expect(readQuoteSelection(document, false)).toBeNull();
  });

  it("rejects whitespace-only selections", () => {
    const el = body(1, " \t\n ").firstChild;
    if (!el) throw new Error("missing text");
    select(el);
    frame(2, " \t ");
    expect(readQuoteSelection(document, false)).toBeNull();
  });

  it("rejects a selection across 2 bodies", () => {
    const a = body(1, "first").firstChild;
    const b = body(2, "second").firstChild;
    if (!a || !b) throw new Error("missing text");
    select(a, b);
    expect(readQuoteSelection(document, false)).toBeNull();
  });

  it("prefers the focused frame, then the app document, then frame order", () => {
    frame(1, "first");
    const focused = frame(2, "focused");
    const el = body(3, "plain").firstChild;
    if (!el) throw new Error("missing text");
    select(el);
    focused.focus();
    expect(readQuoteSelection(document, false)).toEqual({ messageId: 2, text: "focused" });
    if (!focused.contentWindow) throw new Error("frame has no window");
    vi.mocked(focused.contentWindow.getSelection).mockReturnValue({ toString: () => " " } as Selection);
    expect(readQuoteSelection(document, false)).toEqual({ messageId: 3, text: "plain" });
    document.getSelection()?.removeAllRanges();
    expect(readQuoteSelection(document, false)).toEqual({ messageId: 1, text: "first" });
  });

  it("never reads either kind of selection in the web build", () => {
    const el = body(1, "plain").firstChild;
    if (!el) throw new Error("missing text");
    select(el);
    frame(2, "frame");
    const get = vi.spyOn(document, "getSelection");
    expect(readQuoteSelection(document, true)).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("returns null if a frame throws", () => {
    const el = frame(1, "text");
    if (!el.contentWindow) throw new Error("frame has no window");
    vi.mocked(el.contentWindow.getSelection).mockImplementation(() => {
      throw new Error("denied");
    });
    expect(readQuoteSelection(document, false)).toBeNull();
  });
});
