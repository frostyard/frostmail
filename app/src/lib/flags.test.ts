import { describe, expect, it } from "vitest";

import { flagLabel, flagName } from "./flags";

describe("flag names", () => {
  it("names colors 1-7 and nothing else", () => {
    expect(flagName(1)).toBe("Red");
    expect(flagName(7)).toBe("Gray");
    expect(flagName(0)).toBe("");
    expect(flagName(8)).toBe("");
  });

  it("prefers the user's name for a color", () => {
    const names = [" Urgent ", "", "", "", "", "", "Someday"];
    expect(flagLabel(1, names)).toBe("Urgent");
    expect(flagLabel(2, names)).toBe("Orange");
    expect(flagLabel(7, names)).toBe("Someday");
    expect(flagLabel(3)).toBe("Yellow");
  });
});
