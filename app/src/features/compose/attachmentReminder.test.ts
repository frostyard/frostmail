import { describe, expect, it } from "vitest";
import { hasAttachment, htmlText, mentionsAttachment, needsAttachmentReminder, ownText } from "./attachmentReminder";

describe("htmlText", () => {
  it("ends a line at each block", () => {
    expect(htmlText("<p>I attached</p><p>the file.</p>")).toBe("I attached the file.");
  });
  it("ends a line at br", () => {
    expect(htmlText("a<br>b")).toBe("a b");
  });
  it("adds nothing for inline elements", () => {
    expect(htmlText("<b>attach</b>ed")).toBe("attached");
  });
});

describe("ownText", () => {
  const quote = '<blockquote type="cite"><p>see attached</p></blockquote>';
  it("drops a reply quote and its wrote: line", () => {
    const html = `<p>Thanks</p><p>On Mon, Ann wrote:</p>${quote}`;
    expect(ownText(html, "")).toBe("Thanks");
  });
  it("drops a forward block", () => {
    const html = `<p><br></p><p>Begin forwarded message:</p>${quote}`;
    expect(ownText(html, "")).toBe("");
  });
  it("removes a multi-line signature and does not trigger", () => {
    const sig = "<p>Ann</p><p>Attached Corp</p>";
    const html = `<p>Hello</p><p><br></p>${sig}`;
    expect(ownText(html, sig)).toBe("Hello");
    expect(needsAttachmentReminder(html, sig, 0)).toBe(false);
  });
  it("removes the signature only at its last place", () => {
    expect(ownText("<p>Ann</p><p>Ann</p>", "<p>Ann</p>")).toBe("Ann");
  });
  it("keeps text between two quotes", () => {
    const html = `<p>On x wrote:</p>${quote}<p>see attached</p><p>On y wrote:</p>${quote}`;
    expect(ownText(html, "")).toBe("see attached");
  });
  it("keeps a list before a quote that ends with wrote:", () => {
    const html = `<ul><li>I attached what Ann wrote:</li></ul>${quote}`;
    expect(ownText(html, "")).toBe("I attached what Ann wrote:");
    expect(needsAttachmentReminder(html, "", 0)).toBe(true);
  });
  it("removes nothing when the signature is not found", () => {
    expect(ownText("<p>Hello</p>", "<p>Ann</p>")).toBe("Hello");
  });
});

describe("mentionsAttachment", () => {
  it.each([
    "attach",
    "attaches",
    "attached",
    "attaching",
    "attachment",
    "attachments",
    "enclosed",
    "enclosure",
    "enclosures",
  ])("matches %s in any case", (word) => {
    expect(mentionsAttachment(`See the ${word}.`)).toBe(true);
    expect(mentionsAttachment(word.toUpperCase())).toBe(true);
  });
  it.each(["unattached", "detached", "attachés"])("does not match %s", (word) => {
    expect(mentionsAttachment(`a ${word} b`)).toBe(false);
  });
});

describe("hasAttachment", () => {
  it("counts an image outside a blockquote", () => {
    expect(hasAttachment('<p><img src="data:image/png;base64,AA"></p>', 0)).toBe(true);
  });
  it("does not count an image only inside a blockquote", () => {
    expect(hasAttachment('<blockquote><img src="x"></blockquote>', 0)).toBe(false);
  });
  it("counts a chip", () => {
    expect(hasAttachment("<p>x</p>", 1)).toBe(true);
  });
});
