import { describe, expect, it } from "vitest";

import { appLocale } from "./calendarDates";

describe("appLocale", () => {
  it("keeps a language tag Intl takes", () => {
    expect(appLocale("de-DE")).toBe("de-DE");
    expect(appLocale("en-us")).toBe("en-US");
  });

  it("falls back to the runtime's locale for POSIX names and nothing", () => {
    const fallback = Intl.DateTimeFormat().resolvedOptions().locale;
    for (const language of ["C", "", "C.UTF-8"]) {
      const locale = appLocale(language);
      expect(locale).toBe(fallback);
      expect(() => new Intl.DateTimeFormat(locale)).not.toThrow();
    }
  });
});
