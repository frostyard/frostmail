// CONTRACT TEST for task card T-0063 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { avatarTone } from "../../lib/format";
import { Avatar } from "./Avatar";

describe("Avatar", () => {
  it("shows initials on the email's tone", () => {
    render(<Avatar name="Grace Brewster Hopper" email="grace@navy.example" size={64} />);
    const el = screen.getByText("GH");
    expect(el.className).toContain(`bg-avatar-${avatarTone("grace@navy.example")}`);
    expect(el.className).toContain("size-16");
    expect(el.getAttribute("aria-hidden")).toBe("true");
  });

  it("uses the email when there is no name", () => {
    render(<Avatar name="" email="zed@example.com" size={28} />);
    expect(screen.getByText("Z").className).toContain("size-7");
  });

  it("shows the photo when there is one", () => {
    const { container } = render(<Avatar name="Ada" email="ada@example.com" size={48} photo="data:image/png;base64,AA==" />);
    const img = container.querySelector("img");
    expect(img?.getAttribute("src")).toBe("data:image/png;base64,AA==");
    expect(img?.getAttribute("alt")).toBe("");
    expect(img?.className).toContain("size-12");
    expect(img?.className).toContain("object-cover");
    expect(screen.queryByText("A")).toBeNull();
  });
});
