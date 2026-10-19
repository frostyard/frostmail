// Notifications in a browser (docs/specs/lodge-app.md, The web build): a
// maild with no session bus announces new mail to connected clients
// (message.announced), and the web build shows each note with the
// Notification API, as maild words them on a desktop. Calendar reminders
// raise the reminder window, which a hidden page cannot show, so a hidden
// page notifies them too. Nothing notifies while no page is open: that
// waits on Web Push, lodge's decision.
import { isTauri } from "@tauri-apps/api/core";

import { useMail, useUI } from "../data/stores";
import { lodge } from "../lodge/lodge";
import { routeOf } from "../lodge/routes";
import type { Client } from "../rpc/gen/api";
import { revealMessage } from "./openMessage";
import { openReminders } from "./reminders";

/** browserNotifications reports whether this page can ask for and show notifications. */
export function browserNotifications(): boolean {
  return !isTauri() && typeof Notification !== "undefined";
}

/**
 * notify shows one notification, when the browser allows them, and runs
 * onClick when it is clicked. A browser with no Notification constructor
 * (Android's Chrome) shows it through a service worker's registration,
 * when the page has one; lodge's shell has none yet.
 */
export async function notify(title: string, body: string, tag: string, onClick: () => void): Promise<void> {
  if (!browserNotifications() || Notification.permission !== "granted") return;
  try {
    const n = new Notification(title, { body, tag });
    n.onclick = () => {
      window.focus();
      n.close();
      onClick();
    };
  } catch {
    const registration = await navigator.serviceWorker?.getRegistration();
    await registration?.showNotification(title, { body, tag });
  }
}

/** show brings mail to the front in lodge's shell, at what the window now shows. */
function show(): void {
  const ui = useUI.getState();
  ui.setModule("mail");
  lodge()?.open(`/mail${routeOf(ui.source, ui.selected)}`);
}

/** watchAnnouncements shows new mail and, while the page is hidden, due reminders; it returns the cleanup. */
export function watchAnnouncements(client: Client): () => void {
  if (!browserNotifications()) return () => {};
  const reminded = new Set<string>();
  const openMessage = (id: number) => {
    void revealMessage(client, id)
      .then(show)
      .catch((err: unknown) => console.warn("open announced message", err));
  };
  const openInbox = (accountId: number) => {
    const inbox = useMail.getState().mailboxes.find((mb) => mb.accountId === accountId && mb.role === "inbox");
    useUI.getState().setSource(inbox ? { kind: "mailbox", mailboxId: inbox.id } : { kind: "allInboxes" });
    show();
  };
  const remind = () => {
    void client.calendar
      .reminders({})
      .then((rows) => {
        for (const r of rows) {
          if (reminded.has(r.id)) continue;
          reminded.add(r.id);
          void notify(
            r.summary || "(no title)",
            r.location ? `Reminder · ${r.location}` : "Reminder",
            `reminder:${r.id}`,
            () => {
              show();
              void openReminders();
            },
          );
        }
      })
      .catch((err: unknown) => console.warn("reminders", err));
  };
  return client.transport.onEvent((event) => {
    if (event.event === "message.announced") {
      const { accountId, notes } = event.data;
      for (const n of notes) {
        const id = n.messageId;
        const tag = id === undefined ? `mail:${accountId}` : `mail:${accountId}:${id}`;
        void notify(n.title, n.body, tag, () => (id === undefined ? openInbox(accountId) : openMessage(id)));
      }
    } else if (event.event === "calendar.reminders" && event.data.count > 0 && document.visibilityState === "hidden") {
      remind();
    }
  });
}
