// CONTRACT TEST for task card T-0043 (docs/tasks). Do not edit.
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Address } from "../../rpc/gen/api";
import { RecipientField } from "./RecipientField";

const ann: Address = { name: "Ann Example", address: "ann@x.test" };
const bob: Address = { name: "", address: "bob@x.test" };
const carol: Address = { name: "Carol", address: "carol@x.test" };

function Harness(props: {
  initial?: Address[];
  suggest?: (prefix: string) => Promise<Address[]>;
  onChange?: (next: Address[]) => void;
}) {
  const [value, setValue] = useState<Address[]>(props.initial ?? []);
  return (
    <div>
      <RecipientField
        label="To"
        value={value}
        onChange={(next) => {
          props.onChange?.(next);
          setValue(next);
        }}
        suggest={props.suggest ?? (() => Promise.resolve([]))}
      />
      <input aria-label="Subject" />
    </div>
  );
}

function input(): HTMLInputElement {
  return screen.getByRole("combobox", { name: "To" }) as HTMLInputElement;
}

function token(text: string): HTMLElement {
  return screen.getByRole("button", { name: text });
}

describe("RecipientField", () => {
  it("shows a token per address, named by its name or address", () => {
    render(<Harness initial={[ann, bob]} />);
    expect(token("Ann Example").getAttribute("title")).toBe("Ann Example <ann@x.test>");
    expect(token("bob@x.test").getAttribute("title")).toBe("bob@x.test");
    expect(token("Ann Example").getAttribute("aria-pressed")).toBe("false");
    expect(token("Ann Example").getAttribute("tabindex")).toBe("-1");
    expect(input().getAttribute("aria-expanded")).toBe("false");
  });

  it("commits typed text on a comma or semicolon without inserting it", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await user.click(input());
    await user.type(input(), "ann@x.test,");
    expect(onChange).toHaveBeenLastCalledWith([{ name: "", address: "ann@x.test" }]);
    expect(input().value).toBe("");
    await user.type(input(), "Bob <bob@x.test>;");
    expect(onChange).toHaveBeenLastCalledWith([
      { name: "", address: "ann@x.test" },
      { name: "Bob", address: "bob@x.test" },
    ]);
    expect(input().value).toBe("");
  });

  it("commits on Enter and when focus leaves, skipping addresses it already has", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness initial={[bob]} onChange={onChange} />);
    await user.click(input());
    await user.type(input(), "BOB@x.test{Enter}");
    expect(onChange).not.toHaveBeenCalled();
    expect(input().value).toBe("");
    await user.type(input(), "carol@x.test");
    await user.click(screen.getByRole("textbox", { name: "Subject" }));
    expect(onChange).toHaveBeenLastCalledWith([bob, { name: "", address: "carol@x.test" }]);
    expect(input().value).toBe("");
  });

  it("lets Tab move focus, committing typed text on the way", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await user.click(input());
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Subject" }));
    expect(onChange).not.toHaveBeenCalled();
    await user.click(input());
    await user.type(input(), "dan@x.test");
    await user.tab();
    expect(onChange).toHaveBeenLastCalledWith([{ name: "", address: "dan@x.test" }]);
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Subject" }));
  });

  it("keeps what does not parse as an invalid token", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await user.click(input());
    await user.type(input(), "nope,");
    expect(onChange).toHaveBeenLastCalledWith([{ name: "", address: "nope" }]);
    expect(token("nope").getAttribute("data-invalid")).toBe("true");
    expect(token("nope").getAttribute("title")).toBe("nope is not a valid address");
  });

  it("commits pasted lists at once and pastes anything else as text", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await user.click(input());
    await user.paste("Ann Example <ann@x.test>, bob@x.test");
    expect(onChange).toHaveBeenLastCalledWith([ann, bob]);
    expect(input().value).toBe("");
    await user.paste("carol@");
    expect(input().value).toBe("carol@");
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("selects the last token on Backspace and removes it on the next", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness initial={[ann, bob]} onChange={onChange} />);
    await user.click(input());
    await user.keyboard("{Backspace}");
    expect(token("bob@x.test").getAttribute("aria-pressed")).toBe("true");
    expect(onChange).not.toHaveBeenCalled();
    await user.keyboard("{Backspace}");
    expect(onChange).toHaveBeenLastCalledWith([ann]);
    expect(screen.queryByRole("button", { name: "bob@x.test" })).toBeNull();
  });

  it("clears the selection when typing", async () => {
    const user = userEvent.setup();
    render(<Harness initial={[ann, bob]} />);
    await user.click(input());
    await user.keyboard("{Backspace}");
    await user.keyboard("c");
    expect(token("bob@x.test").getAttribute("aria-pressed")).toBe("false");
    expect(input().value).toBe("c");
  });

  it("selects a clicked token, keeps focus in the input, and removes it with Delete", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness initial={[ann, bob]} onChange={onChange} />);
    await user.click(token("Ann Example"));
    expect(token("Ann Example").getAttribute("aria-pressed")).toBe("true");
    expect(document.activeElement).toBe(input());
    await user.keyboard("{Delete}");
    expect(onChange).toHaveBeenLastCalledWith([bob]);
  });

  it("lists suggestions without addresses already present and adds the highlighted one on Enter", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const suggest = vi.fn((_prefix: string) => Promise.resolve([carol, ann, bob]));
    render(<Harness initial={[ann]} onChange={onChange} suggest={suggest} />);
    await user.click(input());
    await user.type(input(), "c");
    expect(suggest).toHaveBeenLastCalledWith("c");
    const list = await screen.findByRole("listbox");
    expect(input().getAttribute("aria-expanded")).toBe("true");
    expect(input().getAttribute("aria-controls")).toBe(list.id);
    const options = screen.getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Carolcarol@x.test", "bob@x.test"]);
    expect(options.every((o) => o.getAttribute("aria-selected") === "false")).toBe(true);

    await user.keyboard("{ArrowDown}");
    expect(options[0]?.getAttribute("aria-selected")).toBe("true");
    expect(input().getAttribute("aria-activedescendant")).toBe(options[0]?.id);
    await user.keyboard("{ArrowDown}{ArrowUp}{Enter}");
    expect(onChange).toHaveBeenLastCalledWith([ann, carol]);
    expect(input().value).toBe("");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input().getAttribute("aria-expanded")).toBe("false");
  });

  it("adds a suggestion on Tab and on click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} suggest={() => Promise.resolve([carol, bob])} />);
    await user.click(input());
    await user.type(input(), "x");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowDown}{ArrowDown}{Tab}");
    expect(onChange).toHaveBeenLastCalledWith([bob]);
    expect(document.activeElement).toBe(input());

    await user.type(input(), "c");
    const options = await screen.findAllByRole("option");
    const first = options[0];
    if (!first) throw new Error("no option");
    await user.click(first);
    expect(onChange).toHaveBeenLastCalledWith([bob, carol]);
    expect(document.activeElement).toBe(input());
  });

  it("closes the list on Escape and asks nothing for blank text", async () => {
    const user = userEvent.setup();
    const suggest = vi.fn((_prefix: string) => Promise.resolve([carol]));
    render(<Harness suggest={suggest} />);
    await user.click(input());
    await user.type(input(), "   ");
    expect(suggest).not.toHaveBeenCalled();
    await user.clear(input());
    await user.type(input(), "ca");
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input().value).toBe("ca");
  });

  it("drops suggestions for text that has since changed", async () => {
    const user = userEvent.setup();
    const pending = new Map<string, (list: Address[]) => void>();
    const suggest = (prefix: string) =>
      new Promise<Address[]>((resolve) => {
        pending.set(prefix, resolve);
      });
    render(<Harness suggest={suggest} />);
    await user.click(input());
    await user.type(input(), "ca");
    await act(async () => {
      pending.get("ca")?.([carol]);
    });
    await act(async () => {
      pending.get("c")?.([bob]);
    });
    const options = screen.getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Carolcarol@x.test"]);
  });
});
