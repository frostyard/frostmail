// What a window forgets when its new connection reaches another maild
// (docs/specs/settings-ui.md, Server pane): the old maild's ids.
import { beforeEach, describe, expect, it } from "vitest";

import { useUI } from "./stores";

beforeEach(() => {
  localStorage.clear();
  useUI.setState(useUI.getInitialState());
});

describe("forgetMaild", () => {
  it("drops the ids of the maild it showed", () => {
    const ui = useUI.getState();
    ui.setSource({ kind: "mailbox", mailboxId: 7 });
    ui.commitSearch("offsite");
    ui.select([41], 41);
    ui.setRevealing({
      id: 41,
      accountId: 1,
      mailboxIds: [7],
      threadId: 41,
      subject: "Offsite plan",
      from: { name: "Ann", address: "ann@x.test" },
      date: "2026-10-01T00:00:00Z",
      preview: "",
      flags: { seen: false, flagged: false, answered: false, forwarded: false, draft: false, flagColor: 0 },
      hasAttachments: false,
      size: 1,
      threadCount: 1,
    });
    ui.openMailboxes(true);
    ui.openSmartSheet({ mode: "edit", id: 3 });
    ui.selectOccurrence({ eventId: 12, recurrenceId: "" });
    ui.setTasksSource(4);
    ui.selectTask(9);
    ui.setPeopleBook(5);
    ui.selectPerson(6);
    ui.editContact(6);

    ui.forgetMaild();

    const now = useUI.getState();
    const initial = useUI.getInitialState();
    expect(now.source).toEqual(initial.source);
    expect(now.search).toBe("");
    expect(now.searchDraft).toBe("");
    expect(now.selected).toEqual([]);
    expect(now.anchor).toBeNull();
    expect(now.revealing).toBeNull();
    expect(now.mailboxesOpen).toBe(false);
    expect(now.smartSheet).toBeNull();
    expect(now.calendarSelected).toBeNull();
    expect(now.tasksSource).toEqual(initial.tasksSource);
    expect(now.tasksSelected).toBeNull();
    expect(now.peopleBook).toBe("all");
    expect(now.peopleSelected).toBeNull();
    expect(now.peopleEditing).toBeNull();
  });

  it("keeps the window's own settings", () => {
    const ui = useUI.getState();
    ui.setModule("calendar");
    ui.setWidths({ sidebarWidth: 240 });
    ui.setConversations(false);
    ui.forgetMaild();
    const now = useUI.getState();
    expect(now.module).toBe("calendar");
    expect(now.sidebarWidth).toBe(240);
    expect(now.conversations).toBe(false);
  });
});
