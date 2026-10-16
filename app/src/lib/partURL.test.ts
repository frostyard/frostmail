import { describe, expect, it } from "vitest";

import { isPartURL, toMaildParts, toPageParts } from "./partURL";

describe("parts URLs", () => {
  const html = '<p><img src="mailpart://localhost/41/2.png"></p>';

  it("stay maild's on Linux", () => {
    expect(toPageParts(html, false)).toBe(html);
    expect(toMaildParts(html, false)).toBe(html);
    expect(isPartURL("http://mailpart.localhost/41/2.png", false)).toBe(false);
  });

  it("take WebView2's form on Windows, and go back for maild", () => {
    const page = toPageParts(html, true);
    expect(page).toBe('<p><img src="http://mailpart.localhost/41/2.png"></p>');
    expect(toMaildParts(page, true)).toBe(html);
    expect(isPartURL("http://mailpart.localhost/41/2.png", true)).toBe(true);
    expect(isPartURL("http://example.com/x.png", true)).toBe(false);
    expect(isPartURL("mailpart://localhost/41/2.png", true)).toBe(true);
  });
});
