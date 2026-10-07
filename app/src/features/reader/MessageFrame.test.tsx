import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { frameDocument, linkTarget, MessageFrame } from "./MessageFrame";

describe("frameDocument", () => {
  it("wraps the HTML in a light page", () => {
    const doc = frameDocument("<p>x</p>", false);
    expect(doc.startsWith('<!doctype html><html><head><meta charset="utf-8">')).toBe(true);
    expect(doc).toContain('<meta name="color-scheme" content="light">');
    expect(doc).toContain("<body><p>x</p></body>");
  });

  it("rewrites mailpart URLs to the dev server only in dev", () => {
    const html = '<img src="mailpart://localhost/m/1/2.png">';
    expect(frameDocument(html, true)).toContain('src="/mailpart/m/1/2.png"');
    expect(frameDocument(html, false)).toContain('src="mailpart://localhost/m/1/2.png"');
  });
});

describe("linkTarget", () => {
  const link = (href: string) => {
    const a = document.createElement("a");
    a.setAttribute("href", href);
    const span = document.createElement("span");
    a.appendChild(span);
    return span;
  };

  it("opens http, https and mailto links from anywhere inside them", () => {
    expect(linkTarget(link("https://example.test/a?b=1"))).toBe("https://example.test/a?b=1");
    expect(linkTarget(link("http://example.test"))).toBe("http://example.test/");
    expect(linkTarget(link("mailto:a@example.test"))).toBe("mailto:a@example.test");
  });

  it("opens nothing else", () => {
    for (const href of ["javascript:alert(1)", "file:///etc/passwd", "#top", "/relative", "data:text/html,x"]) {
      expect(linkTarget(link(href))).toBeNull();
    }
    expect(linkTarget(document.createElement("p"))).toBeNull();
    expect(linkTarget(null)).toBeNull();
  });
});

describe("MessageFrame", () => {
  it("is a sandboxed srcdoc frame without scripts", () => {
    const { container } = render(<MessageFrame html="<p>hello</p>" onOpenLink={() => {}} />);
    const frame = container.querySelector("iframe");
    expect(frame?.getAttribute("sandbox")).toBe("allow-same-origin");
    expect(frame?.getAttribute("srcdoc")).toContain("<p>hello</p>");
    expect(frame?.getAttribute("referrerpolicy")).toBe("no-referrer");
  });
});
