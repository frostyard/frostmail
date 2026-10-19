import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ServerPane } from "./ServerPane";

describe("ServerPane", () => {
  it("chooses lodge at an address", () => {
    const onSave = vi.fn();
    render(<ServerPane lodge={null} busy={false} error={null} onSave={onSave} />);
    const save = screen.getByRole("button", { name: "Save" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.click(screen.getByRole("radio", { name: "maild behind lodge" }));
    expect(save.disabled).toBe(true);
    fireEvent.change(screen.getByRole("textbox", { name: "lodge:" }), {
      target: { value: " https://lodge.example.ts.net " },
    });
    fireEvent.click(save);
    expect(onSave).toHaveBeenCalledWith("https://lodge.example.ts.net");
  });

  it("goes back to this computer's maild", () => {
    const onSave = vi.fn();
    render(<ServerPane lodge="https://lodge.example.ts.net" busy={false} error="refused" onSave={onSave} />);
    expect(screen.getByRole("alert").textContent).toBe("refused");
    expect((screen.getByRole("textbox", { name: "lodge:" }) as HTMLInputElement).value).toBe(
      "https://lodge.example.ts.net",
    );
    fireEvent.click(screen.getByRole("radio", { name: "maild on this computer" }));
    expect(screen.queryByRole("textbox", { name: "lodge:" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSave).toHaveBeenCalledWith(null);
  });
});
