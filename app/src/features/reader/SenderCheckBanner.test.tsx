import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MainWindow } from "../../app/MainWindow";
import { Session } from "../../data/session";
import { useUI } from "../../data/stores";
import type { SenderCheck } from "../../rpc/gen/api";
import { mockData } from "../../rpc/mock/fixture";
import { MockTransport } from "../../rpc/mock/mock";
import { SenderCheckBanner } from "./SenderCheckBanner";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("SenderCheckBanner", () => {
  it("names the sender's domain, lowercased", () => {
    render(<SenderCheckBanner address="Ann@Bank.Example" showing={false} onToggle={() => {}} />);
    const banner = screen.getByRole("status");
    expect(banner.textContent).toContain(
      "Sender not verified. This message says it is from bank.example, but bank.example did not vouch for it. Be careful with its links and attachments.",
    );
    expect(banner.querySelector("svg")?.getAttribute("class")).toContain("text-flag-1");
    expect(within(banner).getByRole("button", { name: "Show Details" }).className).toContain("print:hidden");
  });

  it("has its own wording with no sender address", () => {
    render(<SenderCheckBanner address="" showing onToggle={() => {}} />);
    const banner = screen.getByRole("status");
    expect(banner.textContent).toContain(
      "Sender not verified. Its sender did not pass the provider's checks. Be careful with its links and attachments.",
    );
    expect(within(banner).getByRole("button", { name: "Hide Details" })).toBeTruthy();
  });
});

async function open(senderCheck: SenderCheck | undefined) {
  const data = mockData({ inbox: 5, now: new Date("2026-10-08T12:00:00Z") });
  for (const m of data.messages) {
    m.senderCheck = senderCheck;
    m.summary.from = { name: "Bank", address: "Alerts@Bank.Example" };
  }
  const mock = new MockTransport(data);
  render(
    <Session connect={() => Promise.resolve(mock)} fallback={<div>connecting</div>}>
      <MainWindow />
    </Session>,
  );
  const list = await screen.findByRole("listbox", { name: "Messages" });
  const row = await waitFor(() => {
    const r = within(list).queryAllByRole("option")[0];
    if (!r) throw new Error("no row yet");
    return r;
  });
  fireEvent.click(row);
  const reader = screen.getByRole("region", { name: "Message" });
  await waitFor(() =>
    expect(within(reader).getAllByRole("button", { name: "More Actions" }).length).toBeGreaterThan(0),
  );
  return reader;
}

describe("the reader's sender check", () => {
  it("shows the banner on a fail, and Show Details toggles All Headers", async () => {
    const reader = await open("fail");
    const [banner] = await within(reader).findAllByText("Sender not verified.");
    expect(banner?.closest("[role=status]")?.textContent).toContain("from bank.example, but bank.example did");
    fireEvent.click(within(reader).getAllByRole("button", { name: "Show Details" })[0] as HTMLElement);
    await within(reader).findByLabelText("All Headers");
    fireEvent.click(within(reader).getAllByRole("button", { name: "Hide Details" })[0] as HTMLElement);
    await waitFor(() => expect(within(reader).queryByLabelText("All Headers")).toBeNull());
    expect(within(reader).getAllByRole("button", { name: "Show Details" })[0] as HTMLElement).toBeTruthy();
  });

  it("shows nothing on a pass", async () => {
    const reader = await open("pass");
    expect(within(reader).queryByText(/Sender not verified/)).toBeNull();
  });

  it("shows nothing when the check is absent", async () => {
    const reader = await open(undefined);
    expect(within(reader).queryByText(/Sender not verified/)).toBeNull();
  });
});
