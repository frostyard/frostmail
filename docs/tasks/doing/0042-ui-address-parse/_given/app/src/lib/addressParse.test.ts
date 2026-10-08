// CONTRACT TEST for task card T-0042 (docs/tasks). Do not edit.
import { describe, expect, it } from "vitest";

import { isValidAddress, parseAddresses } from "./addressParse";

describe("isValidAddress", () => {
  it.each([
    ["ann@x.test", true],
    ["ann.smith+tag@mail.example.co.uk", true],
    ["a@b.c", true],
    ["ann@x", false],
    ["ann@", false],
    ["@x.test", false],
    ["ann x@x.test", false],
    ["ann@@x.test", false],
    ["ann@x..test", false],
    ["", false],
  ])("%s is %s", (text, want) => {
    expect(isValidAddress(text)).toBe(want);
  });
});

describe("parseAddresses", () => {
  it("splits on commas, semicolons and newlines", () => {
    expect(parseAddresses("ann@x.test, bob@x.test;carol@x.test\ndan@x.test")).toEqual({
      addresses: [
        { name: "", address: "ann@x.test" },
        { name: "", address: "bob@x.test" },
        { name: "", address: "carol@x.test" },
        { name: "", address: "dan@x.test" },
      ],
      invalid: [],
    });
  });

  it("reads names in the Name <address> form, quoted or not", () => {
    expect(parseAddresses('Ann Smith <ann@x.test>, "Smith, Bob" <bob@x.test>, <carol@x.test>')).toEqual({
      addresses: [
        { name: "Ann Smith", address: "ann@x.test" },
        { name: "Smith, Bob", address: "bob@x.test" },
        { name: "", address: "carol@x.test" },
      ],
      invalid: [],
    });
  });

  it("separates what does not parse and skips empty pieces", () => {
    expect(parseAddresses(" ann@x.test ,, not an address ; Bob <bob@> ,")).toEqual({
      addresses: [{ name: "", address: "ann@x.test" }],
      invalid: ["not an address", "Bob <bob@>"],
    });
  });

  it("drops repeated addresses, case-insensitively, keeping the first", () => {
    expect(parseAddresses("Ann <ann@x.test>, ANN@x.test")).toEqual({
      addresses: [{ name: "Ann", address: "ann@x.test" }],
      invalid: [],
    });
  });

  it("returns nothing for blank text", () => {
    expect(parseAddresses("   ")).toEqual({ addresses: [], invalid: [] });
  });
});
