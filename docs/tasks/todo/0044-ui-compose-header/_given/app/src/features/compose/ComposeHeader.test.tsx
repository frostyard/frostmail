// CONTRACT TEST for task card T-0044 (docs/tasks). Do not edit.
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Address, DraftContent, Identity } from "../../rpc/gen/api";
import { ComposeHeader } from "./ComposeHeader";

const ann: Identity = {
  id: 1,
  accountId: 1,
  name: "Ann Example",
  email: "ann@x.test",
  replyTo: "",
  signatureHtml: "",
  isDefault: true,
};
const work: Identity = { ...ann, id: 2, name: "", email: "ann@work.test", isDefault: false };

const empty: DraftContent = { identityId: 1, to: [], cc: [], bcc: [], subject: "", html: "" };

function Harness(props: {
  initial?: DraftContent;
  identities?: Identity[];
  showBcc?: boolean;
  onChange?: (patch: Partial<DraftContent>) => void;
  suggest?: (prefix: string) => Promise<Address[]>;
  autoFocusTo?: boolean;
}) {
  const [content, setContent] = useState<DraftContent>(props.initial ?? empty);
  return (
    <ComposeHeader
      content={content}
      identities={props.identities ?? [ann]}
      showBcc={props.showBcc ?? false}
      onChange={(patch) => {
        props.onChange?.(patch);
        setContent((c) => ({ ...c, ...patch }));
      }}
      suggest={props.suggest ?? (() => Promise.resolve([]))}
      autoFocusTo={props.autoFocusTo}
    />
  );
}

const labels = () => Array.from(document.querySelectorAll("[data-header-label]")).map((el) => el.textContent);

describe("ComposeHeader", () => {
  it("shows To, Cc and Subject for a single identity", () => {
    render(<Harness />);
    expect(labels()).toEqual(["To:", "Cc:", "Subject:"]);
    expect(screen.getByRole("combobox", { name: "To" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Cc" })).toBeTruthy();
    expect(screen.queryByRole("combobox", { name: "Bcc" })).toBeNull();
    expect(screen.queryByRole("combobox", { name: "From" })).toBeNull();
  });

  it("adds Bcc when shown and From for several identities", () => {
    render(<Harness showBcc identities={[ann, work]} />);
    expect(labels()).toEqual(["To:", "Cc:", "Bcc:", "Subject:", "From:"]);
    const from = screen.getByRole("combobox", { name: "From" }) as HTMLSelectElement;
    expect(from.tagName).toBe("SELECT");
    expect(Array.from(from.options).map((o) => o.textContent)).toEqual(["Ann Example <ann@x.test>", "ann@work.test"]);
    expect(from.value).toBe("1");
  });

  it("lays out each row with a label column", () => {
    render(<Harness />);
    const label = document.querySelector("[data-header-label]");
    expect(label?.className).toContain("w-16");
    expect(label?.className).toContain("text-right");
    expect(label?.className).toContain("text-secondary");
    expect(label?.getAttribute("aria-hidden")).toBe("true");
    const row = label?.parentElement;
    expect(row?.className).toContain("min-h-[30px]");
    expect(row?.className).toContain("border-b");
    expect(row?.className).toContain("border-separator");
  });

  it("reports recipients per field", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness showBcc onChange={onChange} />);
    await user.type(screen.getByRole("combobox", { name: "To" }), "bob@x.test,");
    expect(onChange).toHaveBeenLastCalledWith({ to: [{ name: "", address: "bob@x.test" }] });
    await user.type(screen.getByRole("combobox", { name: "Cc" }), "carol@x.test,");
    expect(onChange).toHaveBeenLastCalledWith({ cc: [{ name: "", address: "carol@x.test" }] });
    await user.type(screen.getByRole("combobox", { name: "Bcc" }), "dan@x.test,");
    expect(onChange).toHaveBeenLastCalledWith({ bcc: [{ name: "", address: "dan@x.test" }] });
  });

  it("reports the subject as it is typed", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    const subject = screen.getByRole("textbox", { name: "Subject" }) as HTMLInputElement;
    await user.type(subject, "Hi");
    expect(onChange).toHaveBeenLastCalledWith({ subject: "Hi" });
    expect(subject.value).toBe("Hi");
  });

  it("reports the chosen identity as a number", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness identities={[ann, work]} onChange={onChange} />);
    await user.selectOptions(screen.getByRole("combobox", { name: "From" }), "2");
    expect(onChange).toHaveBeenLastCalledWith({ identityId: 2 });
  });

  it("passes suggestions to the recipient fields", async () => {
    const user = userEvent.setup();
    const suggest = vi.fn((_prefix: string) =>
      Promise.resolve<Address[]>([{ name: "Carol", address: "carol@x.test" }]),
    );
    render(<Harness suggest={suggest} />);
    await user.type(screen.getByRole("combobox", { name: "Cc" }), "ca");
    expect(suggest).toHaveBeenLastCalledWith("ca");
    expect(await screen.findByRole("option")).toBeTruthy();
  });

  it("focuses To when asked", () => {
    render(<Harness autoFocusTo />);
    expect(document.activeElement).toBe(screen.getByRole("combobox", { name: "To" }));
  });
});
