// CONTRACT TEST for task card T-0105 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { isVip, vipSet } from "./vips";

describe("VIP addresses", () => {
  it("match without case or surrounding space", () => {
    const set = vipSet([
      { address: "ann@x.test", name: "Ann" },
      { address: "bob@y.test", name: "" },
    ]);
    expect(isVip(" Ann@X.test ", set)).toBe(true);
    expect(isVip("bob@y.test", set)).toBe(true);
    expect(isVip("carol@z.test", set)).toBe(false);
    expect(isVip("ann@x.test", vipSet([]))).toBe(false);
  });
});
