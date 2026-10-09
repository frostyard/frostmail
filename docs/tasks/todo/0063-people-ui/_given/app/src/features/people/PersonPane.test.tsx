// CONTRACT TEST for task card T-0063 (docs/tasks). Do not edit.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { formatListDate } from "../../lib/format";
import type { Contact, MessageSummary, Person } from "../../rpc/gen/api";
import { PersonPane, type PersonPaneProps } from "./PersonPane";

const now = new Date(2026, 9, 8, 15, 0);

function contact(over: Partial<Contact>): Contact {
  return {
    id: 1,
    collectionId: 10,
    accountId: 1,
    displayName: "",
    givenName: "",
    familyName: "",
    nickname: "",
    organization: "",
    title: "",
    emails: [],
    phones: [],
    addresses: [],
    urls: [],
    birthday: "",
    note: "",
    readOnly: false,
    ...over,
  };
}

const ada: Person = {
  id: 7,
  displayName: "Ada Lovelace",
  organization: "Analytical Engines",
  hasPhoto: true,
  contacts: [
    contact({
      id: 7,
      displayName: "Ada Lovelace",
      title: "Countess",
      organization: "Analytical Engines",
      emails: [
        { label: "home", value: "ada@example.com" },
        { label: "", value: "ada@work.example" },
      ],
      phones: [{ label: "mobile", value: "+44 20 7946 0000" }],
      addresses: [
        {
          label: "home",
          street: "12 St James's Square",
          locality: "London",
          region: "",
          postcode: "SW1Y 4JH",
          country: "United Kingdom",
        },
      ],
      urls: [
        { label: "", value: "https://ada.example/" },
        { label: "", value: "javascript:alert(1)" },
      ],
      birthday: "1815-12-10",
      note: "Met at the\nExhibition",
    }),
    contact({
      id: 9,
      collectionId: 21,
      accountId: 2,
      displayName: "Ada",
      nickname: "Countess of Lovelace",
      emails: [{ label: "", value: "ada@example.com" }],
      birthday: "--12-10",
      readOnly: true,
    }),
  ],
};

const books = { 10: { name: "Contacts", account: "Gmail" }, 21: { name: "Shared", account: "iCloud" } };

function summary(id: number, subject: string, date: Date): MessageSummary {
  return {
    id,
    accountId: 1,
    mailboxIds: [1],
    threadId: id,
    subject,
    from: { name: "Ada", address: "ada@example.com" },
    date: date.toISOString(),
    preview: "",
    flags: { seen: true, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
    hasAttachments: false,
    size: 1,
    threadCount: 1,
  };
}

function pane(over: Partial<PersonPaneProps> = {}) {
  const props: PersonPaneProps = {
    person: ada,
    books,
    recent: [],
    now,
    onCompose: vi.fn(),
    onOpenMessage: vi.fn(),
    onOpenURL: vi.fn(),
    ...over,
  };
  render(<PersonPane {...props} />);
  return props;
}

// pairs reads a contact block's dl as [label, value] rows.
function pairs(block: HTMLElement): [string, string][] {
  return [...block.querySelectorAll("dt")].map((dt) => [dt.textContent ?? "", dt.nextElementSibling?.textContent ?? ""]);
}

const longDate = new Intl.DateTimeFormat(undefined, { dateStyle: "long" }).format(new Date(1815, 11, 10));
const monthDay = new Intl.DateTimeFormat(undefined, { month: "long", day: "numeric" }).format(new Date(2000, 11, 10));

describe("PersonPane", () => {
  it("says so when no one is selected", () => {
    pane({ person: null });
    expect(screen.getByText("No Contact Selected").className).toContain("text-secondary");
  });

  it("heads the pane with the photo, name, title and organization", () => {
    const { container } = render(
      <PersonPane
        person={ada}
        books={books}
        photo="data:image/png;base64,AA=="
        recent={[]}
        now={now}
        onCompose={vi.fn()}
        onOpenMessage={vi.fn()}
        onOpenURL={vi.fn()}
      />,
    );
    expect(screen.getByRole("heading", { level: 2, name: "Ada Lovelace" })).toBeTruthy();
    expect(screen.getByText("Countess · Analytical Engines").className).toContain("text-secondary");
    expect(container.querySelector("img")?.getAttribute("src")).toBe("data:image/png;base64,AA==");
  });

  it("starts a message to the first email", () => {
    const props = pane();
    const button = screen.getByRole("button", { name: "Message" });
    expect(button.className).toContain("bg-accent");
    fireEvent.click(button);
    expect(props.onCompose).toHaveBeenCalledWith("ada@example.com");
  });

  it("disables Message for a person without email", () => {
    pane({ person: { ...ada, contacts: [contact({ displayName: "No Mail" })] } });
    expect((screen.getByRole("button", { name: "Message" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("shows a block per contact, named by its book and account", () => {
    pane();
    const first = screen.getByRole("region", { name: "Contacts — Gmail" });
    const second = screen.getByRole("region", { name: "Shared — iCloud · Read-only" });
    const heading = within(first).getByRole("heading", { level: 3 });
    expect(heading.textContent).toBe("Contacts — Gmail");
    expect(heading.className).toContain("uppercase");
    expect(pairs(first)).toEqual([
      ["home", "ada@example.com"],
      ["email", "ada@work.example"],
      ["mobile", "+44 20 7946 0000"],
      ["home", "12 St James's SquareLondon SW1Y 4JHUnited Kingdom"],
      ["homepage", "https://ada.example/"],
      ["homepage", "javascript:alert(1)"],
      ["birthday", longDate],
      ["note", "Met at the\nExhibition"],
    ]);
    expect(pairs(second)).toEqual([
      ["email", "ada@example.com"],
      ["birthday", monthDay],
      ["nickname", "Countess of Lovelace"],
    ]);
  });

  it("puts each address line on its own line", () => {
    pane();
    const block = screen.getByRole("region", { name: "Contacts — Gmail" });
    const dd = [...block.querySelectorAll("dt")].find((dt) => dt.textContent === "home" && dt.nextElementSibling?.querySelector("div"))?.nextElementSibling;
    expect([...(dd?.querySelectorAll("div") ?? [])].map((d) => d.textContent)).toEqual([
      "12 St James's Square",
      "London SW1Y 4JH",
      "United Kingdom",
    ]);
  });

  it("composes to an email and opens web links, not other URLs", () => {
    const props = pane();
    fireEvent.click(screen.getByRole("button", { name: "ada@work.example" }));
    expect(props.onCompose).toHaveBeenCalledWith("ada@work.example");
    const link = screen.getByRole("link", { name: "https://ada.example/" });
    const ev = fireEvent.click(link);
    expect(ev).toBe(false);
    expect(props.onOpenURL).toHaveBeenCalledWith("https://ada.example/");
    expect(screen.queryByRole("link", { name: "javascript:alert(1)" })).toBeNull();
    expect(screen.getByText("javascript:alert(1)")).toBeTruthy();
  });

  it("lists recent mail, which opens in Mail", () => {
    const props = pane({
      recent: [summary(41, "Engines", new Date(2026, 9, 1, 9, 0)), summary(40, "  ", new Date(2026, 9, 8, 9, 30))],
    });
    const region = screen.getByRole("region", { name: "Recent Mail" });
    const rows = within(region).getAllByRole("button");
    expect(rows).toHaveLength(2);
    expect(rows[0]?.textContent).toBe(`Engines${formatListDate(new Date(2026, 9, 1, 9, 0), now)}`);
    expect(within(rows[1] as HTMLElement).getByText("(No Subject)").className).toContain("text-tertiary");
    fireEvent.click(rows[0] as HTMLElement);
    expect(props.onOpenMessage).toHaveBeenCalledWith(41);
  });

  it("has no Recent Mail without recent mail", () => {
    pane();
    expect(screen.queryByRole("region", { name: "Recent Mail" })).toBeNull();
  });
});
