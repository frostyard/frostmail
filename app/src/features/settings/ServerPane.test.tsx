import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { ServerPane, type ServerPaneProps, STARTING, STOPPING, saveStatus } from "./ServerPane";

const TO_LODGE =
  "This computer's maild stops. Its accounts and mail stay on this computer, not synced, until you switch back. Nothing moves to lodge.";
const TO_LOCAL_DATA =
  "This computer's maild starts again with the accounts it had. It runs their rules, Send Later and reminders here. If lodge's maild has the same accounts, both run them.";
const TO_LOCAL_EMPTY =
  "Mail on lodge keeps running there. If you add the same accounts here, both run their rules, Send Later and reminders.";

/** renderPane renders the pane with every prop but the ones given. */
function renderPane({
  lodge = null,
  flatpak = false,
  localData = false,
  busy = false,
  status = null,
  error = null,
}: Partial<ServerPaneProps> = {}) {
  const onSave = vi.fn();
  render(
    <ServerPane
      lodge={lodge}
      flatpak={flatpak}
      localData={localData}
      busy={busy}
      status={status}
      error={error}
      onSave={onSave}
    />,
  );
  return { onSave };
}

const note = () => screen.queryByRole("note")?.textContent ?? null;
const status = () => screen.queryByRole("status")?.textContent ?? null;

describe("ServerPane", () => {
  it("chooses lodge at an address", () => {
    const { onSave } = renderPane();
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
    const { onSave } = renderPane({ lodge: "https://lodge.example.ts.net", error: "refused" });
    expect(screen.getByRole("alert").textContent).toBe("refused");
    expect((screen.getByRole("textbox", { name: "lodge:" }) as HTMLInputElement).value).toBe(
      "https://lodge.example.ts.net",
    );
    fireEvent.click(screen.getByRole("radio", { name: "maild on this computer" }));
    expect(screen.queryByRole("textbox", { name: "lodge:" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSave).toHaveBeenCalledWith(null);
  });

  it("says what a change to lodge does", () => {
    renderPane({ flatpak: true, localData: true });
    fireEvent.click(screen.getByRole("radio", { name: "maild behind lodge" }));
    fireEvent.change(screen.getByRole("textbox", { name: "lodge:" }), {
      target: { value: "https://lodge.example.ts.net" },
    });
    expect(note()).toBe(TO_LODGE);
  });

  it("says what a change to this computer does with its data", () => {
    renderPane({ lodge: "https://lodge.example.ts.net", flatpak: true, localData: true });
    fireEvent.click(screen.getByRole("radio", { name: "maild on this computer" }));
    expect(note()).toBe(TO_LOCAL_DATA);
  });

  it("says what a change to this computer does without its data", () => {
    renderPane({ lodge: "https://lodge.example.ts.net", flatpak: true });
    fireEvent.click(screen.getByRole("radio", { name: "maild on this computer" }));
    expect(note()).toBe(TO_LOCAL_EMPTY);
  });

  it("says nothing outside the Flatpak", () => {
    renderPane();
    fireEvent.click(screen.getByRole("radio", { name: "maild behind lodge" }));
    fireEvent.change(screen.getByRole("textbox", { name: "lodge:" }), {
      target: { value: "https://lodge.example.ts.net" },
    });
    expect(note()).toBeNull();
  });

  it("says nothing when the mode stays the same", () => {
    renderPane({ lodge: "https://lodge.example.ts.net", flatpak: true });
    expect(note()).toBeNull();
    fireEvent.change(screen.getByRole("textbox", { name: "lodge:" }), {
      target: { value: "https://other.example.ts.net" },
    });
    expect(note()).toBeNull();
    fireEvent.click(screen.getByRole("radio", { name: "maild on this computer" }));
    fireEvent.click(screen.getByRole("radio", { name: "maild behind lodge" }));
    expect(note()).toBeNull();
  });

  it("shows a running save's status", () => {
    renderPane({ flatpak: true, busy: true, status: STOPPING });
    expect(status()).toBe("Stopping this computer's maild…");
  });

  it("shows no status once the save ends", () => {
    renderPane({ flatpak: true, busy: false, status: STARTING });
    expect(status()).toBeNull();
  });

  it("fixes the status when a save starts", () => {
    const lodge = "https://lodge.example.ts.net";
    expect(saveStatus(true, null, lodge)).toBe("Stopping this computer's maild…");
    expect(saveStatus(true, lodge, null)).toBe("Starting this computer's maild…");
    // A new lodge address keeps the mode, and no maild of this computer changes.
    expect(saveStatus(true, lodge, "https://other.example.ts.net")).toBeNull();
    expect(saveStatus(true, null, null)).toBeNull();
    // Outside the Flatpak the app neither stops nor starts a maild.
    expect(saveStatus(false, null, lodge)).toBeNull();
    expect(saveStatus(false, lodge, null)).toBeNull();
  });
});
