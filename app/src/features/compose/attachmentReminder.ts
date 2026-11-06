// The attachment reminder's pure checks (docs/specs/compose-ui.md, Attachment
// reminder): does the user's own text mention an attachment that is not there?

const BLOCKS = new Set(["P", "DIV", "LI", "BLOCKQUOTE", "H1", "H2", "H3", "H4", "H5", "H6", "PRE"]);

function squash(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}

function textOf(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
  if (node.nodeType !== Node.ELEMENT_NODE) return "";
  const el = node as Element;
  if (el.tagName === "BR") return "\n";
  let out = "";
  for (const child of Array.from(el.childNodes)) out += textOf(child);
  return BLOCKS.has(el.tagName) ? `${out}\n` : out;
}

function parse(html: string): Document {
  return new DOMParser().parseFromString(html, "text/html");
}

/** htmlText is the text of html: blocks and line breaks end a line, then white space is squashed. */
export function htmlText(html: string): string {
  return squash(textOf(parse(html).body));
}

/** ownText is the text the user wrote: no quotes, no attribution line, no signature. */
export function ownText(html: string, signatureHtml: string): string {
  const body = parse(html).body;
  for (const quote of Array.from(body.children)) {
    if (quote.tagName !== "BLOCKQUOTE") continue;
    let before = quote.previousSibling;
    while (before && before.nodeType === Node.TEXT_NODE && (before.textContent ?? "").trim() === "") {
      before = before.previousSibling;
    }
    if (before && before.nodeType === Node.ELEMENT_NODE) {
      const line = htmlText((before as Element).outerHTML);
      if (line.endsWith("wrote:") || line === "Begin forwarded message:") before.parentNode?.removeChild(before);
    }
  }
  for (const quote of Array.from(body.querySelectorAll("blockquote"))) quote.remove();
  let text = squash(textOf(body));
  const sig = htmlText(signatureHtml);
  if (sig !== "") {
    const at = text.lastIndexOf(sig);
    if (at >= 0) text = squash(`${text.slice(0, at)} ${text.slice(at + sig.length)}`);
  }
  return text;
}

const WORDS =
  /(?<![\p{L}\p{N}])(?:attach|attaches|attached|attaching|attachments?|enclosed|enclosures?)(?![\p{L}\p{N}])/iu;

/** mentionsAttachment is true when text holds an attachment word as a whole word. */
export function mentionsAttachment(text: string): boolean {
  return WORDS.test(text);
}

/** hasAttachment is true with a chip or with an image outside every blockquote. */
export function hasAttachment(html: string, attachmentCount: number): boolean {
  if (attachmentCount > 0) return true;
  return Array.from(parse(html).body.querySelectorAll("img")).some((img) => img.closest("blockquote") === null);
}

/** needsAttachmentReminder is true when the own text mentions an attachment and none is attached. */
export function needsAttachmentReminder(html: string, signatureHtml: string, attachmentCount: number): boolean {
  return mentionsAttachment(ownText(html, signatureHtml)) && !hasAttachment(html, attachmentCount);
}
