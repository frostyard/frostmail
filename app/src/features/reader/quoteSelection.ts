// Reply takes only plain text from marked reader bodies (ADR-0053).
import { IN_LODGE } from "../../lodge/lodge";

/** QuoteSelection identifies the message and the plain text to quote. */
export interface QuoteSelection {
  messageId: number;
  text: string;
}

function quote(source: Element, text: string | undefined): QuoteSelection | null {
  const messageId = Number(source.getAttribute("data-quote-source"));
  return text?.trim() && Number.isSafeInteger(messageId) && messageId > 0 ? { messageId, text } : null;
}

function frameQuote(frame: HTMLIFrameElement): QuoteSelection | null {
  return quote(frame, frame.contentWindow?.getSelection()?.toString());
}

/** QuoteSelectionOptions can preserve focus from before a toolbar press. */
export interface QuoteSelectionOptions {
  doc?: Document;
  web?: boolean;
  active?: Element | null;
}

/** readQuoteSelection reads a reader on this page, never lodge's content frame; no DOM nodes leave it. */
export function readQuoteSelection({
  doc = document,
  web = IN_LODGE,
  active = doc.activeElement,
}: QuoteSelectionOptions = {}): QuoteSelection | null {
  if (web) return null;
  try {
    const frames = Array.from(doc.querySelectorAll<HTMLIFrameElement>("iframe[data-quote-source]"));
    const focused = frames.find((frame) => frame === active);
    if (focused) {
      const selected = frameQuote(focused);
      if (selected) return selected;
    }
    const selection = doc.getSelection();
    if (selection && !selection.isCollapsed && selection.toString().trim()) {
      const body = Array.from(doc.querySelectorAll<HTMLElement>("[data-quote-source]:not(iframe)")).find(
        (el) => el.contains(selection.anchorNode) && el.contains(selection.focusNode),
      );
      // A header, banner or cross-message selection must not use a stale frame selection.
      return body ? quote(body, selection.toString()) : null;
    }
    for (const frame of frames) {
      const selected = frameQuote(frame);
      if (selected) return selected;
    }
    return null;
  } catch {
    return null;
  }
}
