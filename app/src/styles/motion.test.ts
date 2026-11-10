// Reduced motion (docs/specs/ui.md, Rules): app.css stops every animation
// and transition when the system asks for reduced motion. happy-dom does not
// apply media queries, so the test reads the rule from the stylesheet.
import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

describe("app.css", () => {
  it("stops animations, transitions and smooth scrolling under prefers-reduced-motion", () => {
    // Tests run in app/.
    const css = readFileSync("src/styles/app.css", "utf8");
    const start = css.indexOf("@media (prefers-reduced-motion: reduce) {");
    expect(start).toBeGreaterThanOrEqual(0);
    const block = css.slice(start, css.indexOf("\n}\n", start));
    expect(block).toContain("*::before");
    expect(block).toContain("*::after");
    expect(block).toContain("animation-duration: 0.01ms;");
    expect(block).toContain("animation-iteration-count: 1;");
    expect(block).toContain("transition-duration: 0.01ms;");
    expect(block).toContain("scroll-behavior: auto;");
  });
});
