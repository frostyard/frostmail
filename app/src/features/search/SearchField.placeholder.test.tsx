// CONTRACT TEST for task card T-0064 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { SearchField } from "./SearchField";

describe("SearchField placeholder", () => {
  it("says Search by default", () => {
    render(<SearchField value="" onChange={vi.fn()} onSearch={vi.fn()} onClear={vi.fn()} />);
    expect(screen.getByRole("searchbox", { name: "Search" }).getAttribute("placeholder")).toBe("Search");
  });

  it("takes the module's placeholder", () => {
    render(
      <SearchField value="" placeholder="Search Contacts" onChange={vi.fn()} onSearch={vi.fn()} onClear={vi.fn()} />,
    );
    expect(screen.getByRole("searchbox", { name: "Search" }).getAttribute("placeholder")).toBe("Search Contacts");
  });
});
