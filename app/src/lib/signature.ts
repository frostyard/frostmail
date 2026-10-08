// Signatures are stored as HTML and edited as plain text
// (docs/specs/settings-ui.md, Signature text).

const ESCAPES: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" };

/** signatureHtml is the stored HTML of a plain-text signature: a paragraph per line. */
export function signatureHtml(text: string): string {
  if (text.trim() === "") return "";
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  while (lines.length > 0 && lines[lines.length - 1]?.trim() === "") lines.pop();
  return lines
    .map((line) => (line === "" ? "<p><br></p>" : `<p>${line.replace(/[&<>"]/g, (c) => ESCAPES[c] ?? c)}</p>`))
    .join("");
}

/** signatureText is the plain text of a stored signature: a line per block. */
export function signatureText(html: string): string {
  if (html.trim() === "") return "";
  const doc = new DOMParser().parseFromString(html, "text/html");
  const lines: string[] = [];
  let inline = "";
  const flush = () => {
    if (inline !== "") lines.push(inline);
    inline = "";
  };
  for (const node of Array.from(doc.body.childNodes)) {
    if (node instanceof HTMLElement && node.tagName !== "BR" && isBlock(node)) {
      flush();
      lines.push(blockText(node));
    } else if (node instanceof HTMLElement && node.tagName === "BR") {
      inline += "\n";
    } else {
      inline += node.textContent ?? "";
    }
  }
  flush();
  return lines.join("\n");
}

function isBlock(el: HTMLElement): boolean {
  return ["P", "DIV", "LI", "BLOCKQUOTE", "H1", "H2", "H3", "H4", "H5", "H6", "PRE"].includes(el.tagName);
}

// blockText is a block's text with <br> as line breaks; a block that is
// only a <br> is an empty line.
function blockText(el: HTMLElement): string {
  let out = "";
  for (const node of Array.from(el.childNodes)) {
    if (node instanceof HTMLElement && node.tagName === "BR") out += "\n";
    else if (node instanceof HTMLElement) out += blockText(node);
    else out += node.textContent ?? "";
  }
  return out === "\n" ? "" : out;
}
