import { describe, expect, it } from "vitest";

import { permalink } from "./permalink";

describe("permalink", () => {
  it("names a Message-ID as one segment", () => {
    expect(permalink("gen-1-200@mailgen.test")).toBe("/message/gen-1-200%40mailgen.test");
    expect(permalink("a/b?c#d e")).toBe("/message/a%2Fb%3Fc%23d%20e");
  });

  it("keeps maild's ID as it is", () => {
    expect(permalink("42")).toBe("/message/42");
  });
});
