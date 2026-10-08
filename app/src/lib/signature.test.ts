import { describe, expect, it } from "vitest";
import { signatureHtml, signatureText } from "./signature";

describe("signatureHtml", () => {
  it("makes a paragraph per line and escapes the text", () => {
    expect(signatureHtml("Ann <ann@x.test>\n\nR&D")).toBe("<p>Ann &lt;ann@x.test&gt;</p><p><br></p><p>R&amp;D</p>");
  });
  it("drops trailing blank lines and is empty for blank text", () => {
    expect(signatureHtml("Ann\n\n\n")).toBe("<p>Ann</p>");
    expect(signatureHtml("  \n ")).toBe("");
  });
});

describe("signatureText", () => {
  it("reads a line per block and <br> as a line break", () => {
    expect(signatureText('<p>Ann &lt;ann@x.test&gt;</p><p><br></p><p>R&amp;D<br>Team <b>"A"</b></p>')).toBe(
      'Ann <ann@x.test>\n\nR&D\nTeam "A"',
    );
  });
  it("keeps text outside blocks", () => {
    expect(signatureText("Ann<br>Example")).toBe("Ann\nExample");
    expect(signatureText("")).toBe("");
  });
  it("round-trips what signatureHtml writes", () => {
    const text = 'Ann Example\n\n<CEO> & "founder"';
    expect(signatureText(signatureHtml(text))).toBe(text);
  });
});
