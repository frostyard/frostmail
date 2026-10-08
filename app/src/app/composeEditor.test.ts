import { Editor } from "@tiptap/react";
import { afterEach, describe, expect, it } from "vitest";

import { composeExtensions, isAllowedLink, isBlank, setLink, toEditorHTML, toMailHTML } from "./composeEditor";

let editor: Editor | undefined;
afterEach(() => editor?.destroy());

function edit(html: string): Editor {
  editor = new Editor({ extensions: composeExtensions(), content: toEditorHTML(html) });
  return editor;
}

describe("composeEditor", () => {
  it("round-trips mail's empty lines", () => {
    const mail = "<p><br></p><p>Hi</p><p><br></p><p>Bye</p>";
    const e = edit(mail);
    expect(e.state.doc.textContent).toBe("HiBye");
    expect(toMailHTML(e.getHTML())).toBe(mail);
  });

  it("starts typing on the first line of a new message", () => {
    const e = edit("<p><br></p>");
    e.commands.insertContent("Hello");
    expect(toMailHTML(e.getHTML())).toBe("<p>Hello</p>");
  });

  it("keeps quoted inline and data images but drops remote ones", () => {
    const e = edit(
      '<p><img src="mailpart://localhost/m/1/2.png"><img src="data:image/png;base64,AAAA"><img src="https://tracker.example/x.gif"></p>',
    );
    const html = e.getHTML();
    expect(html).toContain("mailpart://localhost/m/1/2.png");
    expect(html).toContain("data:image/png;base64,AAAA");
    expect(html).not.toContain("tracker.example");
  });

  it("keeps quotes and links, dropping links to other schemes", () => {
    const e = edit(
      '<blockquote type="cite"><p>quoted <a href="https://x.test/a">link</a> <a href="javascript:alert(1)">bad</a></p></blockquote>',
    );
    const html = e.getHTML();
    expect(html).toContain("<blockquote");
    expect(html).toContain('href="https://x.test/a"');
    expect(html).not.toContain("javascript:");
  });

  it("sets and removes links, refusing other schemes", () => {
    const e = edit("<p>word</p>");
    e.commands.selectAll();
    expect(setLink(e, "file:///etc/passwd")).toBe(false);
    expect(setLink(e, "https://frostyard.test")).toBe(true);
    expect(e.getHTML()).toContain('href="https://frostyard.test"');
    expect(setLink(e, "")).toBe(true);
    expect(e.getHTML()).not.toContain("href");
    expect(isAllowedLink("mailto:a@x.test")).toBe(true);
  });

  it("knows a blank body", () => {
    expect(isBlank(edit("<p><br></p>"))).toBe(true);
    expect(isBlank(edit("<p> </p>"))).toBe(true);
    expect(isBlank(edit('<p><img src="data:image/png;base64,AAAA"></p>'))).toBe(false);
  });
});
