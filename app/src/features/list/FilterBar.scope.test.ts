import { describe, expect, it } from "vitest";

import { emptyTextIn } from "./FilterBar";

describe("emptyTextIn", () => {
  it("names the source a filter narrows", () => {
    expect(emptyTextIn("flagged", "All Inboxes")).toBe("No Flagged Messages in All Inboxes");
    expect(emptyTextIn("ccMe", "Work")).toBe("No Messages Cc'd to You in Work");
  });

  it("names none without a filter or a source", () => {
    expect(emptyTextIn("all", "All Inboxes")).toBe("No Messages");
    expect(emptyTextIn("unread", "")).toBe("No Unread Messages");
  });
});
