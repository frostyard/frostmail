//! The app's own notifications, for a maild with no desktop to notify on
//! (docs/design/desktop.md, Notifications): a maild behind lodge sends
//! message.announced, and the webview hands its notes here. On Linux they
//! go to org.freedesktop.Notifications, as maild's own do; on Windows to
//! frostmail-notify.exe beside the app, with the operation maild writes.
//! A click opens the message, as --open-message does.

use serde::Deserialize;
use tauri::{AppHandle, Emitter, Manager};

/// The notifier's state, which the app manages: on Linux, the session bus,
/// connected at the first announcement.
#[derive(Default)]
pub struct Notifier {
    #[cfg(target_os = "linux")]
    bus: std::sync::OnceLock<Result<platform::Bus, String>>,
}

/// One note of message.announced.
#[derive(Debug, Deserialize)]
pub struct Note {
    pub title: String,
    pub body: String,
    #[serde(rename = "messageId")]
    pub message_id: Option<i64>,
}

/// Show the notes of one announcement.
#[tauri::command]
pub async fn notify_mail(app: AppHandle, account_id: i64, notes: Vec<Note>) -> Result<(), String> {
    tauri::async_runtime::spawn_blocking(move || platform::show(&app, &app.state::<Notifier>(), account_id, &notes))
        .await
        .map_err(|e| e.to_string())?
}

/// Bring the main window forward at message, or as it is for 0.
pub fn open(app: &AppHandle, message: i64) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
    }
    if message > 0 {
        let _ = app.emit_to("main", "open-message", message);
    }
}

/// The operation frostmail-notify.exe takes for a note, as maild writes it
/// (docs/design/desktop.md, Windows): group mail, tag m<message id>, or
/// a<account id> for a group note, which the next one replaces.
#[cfg(any(windows, test))]
pub fn toast_op(account_id: i64, note: &Note) -> serde_json::Value {
    let (tag, open) = match note.message_id {
        Some(id) if id > 0 => (format!("m{id}"), id),
        _ => (format!("a{account_id}"), 0),
    };
    serde_json::json!({"op": "show", "tag": tag, "group": "mail", "title": note.title, "body": note.body, "open": open})
}

#[cfg(target_os = "linux")]
mod platform {
    use std::collections::HashMap;
    use std::sync::{Arc, Mutex};

    use tauri::AppHandle;
    use zbus::blocking::{Connection, MessageIterator};
    use zbus::zvariant::Value;

    use super::{Note, Notifier};

    const NAME: &str = "org.freedesktop.Notifications";
    const PATH: &str = "/org/freedesktop/Notifications";
    const APP_ID: &str = "org.frostyard.Frostmail";

    /// The session bus, with what the notifications shown open.
    pub struct Bus {
        conn: Connection,
        /// Each account's group note, which the next replaces.
        groups: Mutex<HashMap<i64, u32>>,
        /// The message each notification opens; 0 for a group.
        opens: Arc<Mutex<HashMap<u32, i64>>>,
    }

    impl Bus {
        /// Connect to the session bus; on_click gets the message a clicked
        /// notification opens, 0 for a group.
        pub fn connect(on_click: impl Fn(i64) + Send + 'static) -> zbus::Result<Bus> {
            let conn = Connection::session()?;
            let opens: Arc<Mutex<HashMap<u32, i64>>> = Arc::default();
            let rule = zbus::MatchRule::builder()
                .msg_type(zbus::message::Type::Signal)
                .interface(NAME)?
                .member("ActionInvoked")?
                .path(PATH)?
                .build();
            let signals = MessageIterator::for_match_rule(rule, &conn, None)?;
            let clicked = opens.clone();
            std::thread::spawn(move || {
                for msg in signals.flatten() {
                    let Ok((id, action)) = msg.body().deserialize::<(u32, String)>() else { continue };
                    let message = clicked.lock().ok().and_then(|m| m.get(&id).copied());
                    if let (Some(message), "default") = (message, action.as_str()) {
                        on_click(message);
                    }
                }
            });
            Ok(Bus { conn, groups: Mutex::default(), opens })
        }

        /// Show one announcement's notes, as maild's desktop notifications.
        pub fn show(&self, account_id: i64, notes: &[Note]) -> Result<(), String> {
            for note in notes {
                let message = note.message_id.filter(|&id| id > 0).unwrap_or(0);
                let replaces = if message == 0 {
                    self.groups.lock().map_err(|e| e.to_string())?.get(&account_id).copied().unwrap_or(0)
                } else {
                    0
                };
                let hints =
                    HashMap::from([("desktop-entry", Value::from(APP_ID)), ("category", Value::from("email.arrived"))]);
                let reply = self
                    .conn
                    .call_method(
                        Some(NAME),
                        PATH,
                        Some(NAME),
                        "Notify",
                        &("Frostmail", replaces, APP_ID, &note.title, &note.body, vec!["default", "Open"], hints, -1i32),
                    )
                    .map_err(|e| format!("notify: {e}"))?;
                let id: u32 = reply.body().deserialize().map_err(|e| format!("notify: {e}"))?;
                if message == 0 {
                    self.groups.lock().map_err(|e| e.to_string())?.insert(account_id, id);
                }
                self.opens.lock().map_err(|e| e.to_string())?.insert(id, message);
            }
            Ok(())
        }
    }

    pub fn show(app: &AppHandle, notifier: &Notifier, account_id: i64, notes: &[Note]) -> Result<(), String> {
        let app = app.clone();
        notifier
            .bus
            .get_or_init(|| {
                Bus::connect(move |message| super::open(&app, message)).map_err(|e| format!("the session bus: {e}"))
            })
            .as_ref()
            .map_err(Clone::clone)?
            .show(account_id, notes)
    }

    #[cfg(test)]
    mod tests {
        use std::collections::HashMap;
        use std::sync::{Arc, Mutex, mpsc};
        use std::time::Duration;

        use zbus::zvariant::OwnedValue;

        use super::{Bus, Note};

        /// A notification server recording each Notify, with ids from 1.
        struct Server {
            calls: Arc<Mutex<Vec<(u32, String, String, Vec<String>, String)>>>,
        }

        #[zbus::interface(name = "org.freedesktop.Notifications")]
        impl Server {
            #[allow(clippy::too_many_arguments)]
            fn notify(
                &self,
                _app_name: &str,
                replaces_id: u32,
                _app_icon: &str,
                summary: &str,
                body: &str,
                actions: Vec<String>,
                hints: HashMap<String, OwnedValue>,
                _expire_timeout: i32,
            ) -> u32 {
                let mut calls = self.calls.lock().unwrap();
                let entry = hints.get("desktop-entry").and_then(|v| String::try_from(v.clone()).ok()).unwrap_or_default();
                calls.push((replaces_id, summary.into(), body.into(), actions, entry));
                if replaces_id != 0 { replaces_id } else { calls.len() as u32 }
            }
        }

        /// Run with a session bus, as `dbus-run-session -- cargo test`; it
        /// skips without one.
        #[test]
        fn shows_and_opens_through_the_session_bus() {
            if std::env::var_os("DBUS_SESSION_BUS_ADDRESS").is_none() {
                eprintln!("no session bus: skipped");
                return;
            }
            let calls = Arc::default();
            let server = zbus::blocking::connection::Builder::session()
                .unwrap()
                .name("org.freedesktop.Notifications")
                .unwrap()
                .serve_at("/org/freedesktop/Notifications", Server { calls: Arc::clone(&calls) })
                .unwrap()
                .build()
                .unwrap();
            let (tx, clicked) = mpsc::channel();
            let bus = Bus::connect(move |message| tx.send(message).unwrap()).unwrap();
            let note = |title: &str, id: Option<i64>| Note { title: title.into(), body: "Plan".into(), message_id: id };
            bus.show(3, &[note("Ann", Some(42)), note("4 new messages", None)]).unwrap();
            bus.show(3, &[note("5 new messages", None)]).unwrap();
            {
                let calls = calls.lock().unwrap();
                assert_eq!(calls.len(), 3);
                assert_eq!(calls[0], (0, "Ann".into(), "Plan".into(), vec!["default".into(), "Open".into()], "org.frostyard.Frostmail".into()));
                assert_eq!(calls[1].0, 0);
                assert_eq!(calls[2].0, 2, "the account's group note replaces the last");
            }
            server
                .emit_signal(None::<()>, "/org/freedesktop/Notifications", "org.freedesktop.Notifications", "ActionInvoked", &(1u32, "default"))
                .unwrap();
            assert_eq!(clicked.recv_timeout(Duration::from_secs(5)).unwrap(), 42);
            server
                .emit_signal(None::<()>, "/org/freedesktop/Notifications", "org.freedesktop.Notifications", "ActionInvoked", &(2u32, "default"))
                .unwrap();
            assert_eq!(clicked.recv_timeout(Duration::from_secs(5)).unwrap(), 0);
        }
    }
}

#[cfg(windows)]
mod platform {
    use std::io::Write;
    use std::os::windows::process::CommandExt;
    use std::process::{Command, Stdio};

    use tauri::AppHandle;

    use super::{Note, Notifier, toast_op};

    /// No console window for the notifier.
    const CREATE_NO_WINDOW: u32 = 0x0800_0000;

    /// frostmail-notify.exe shows each toast; a click runs the app with
    /// --open-message, which the single instance hands to this one.
    pub fn show(_app: &AppHandle, _state: &Notifier, account_id: i64, notes: &[Note]) -> Result<(), String> {
        let notifier = std::env::current_exe()
            .map_err(|e| e.to_string())?
            .with_file_name("frostmail-notify.exe");
        for note in notes {
            let mut child = Command::new(&notifier)
                .stdin(Stdio::piped())
                .creation_flags(CREATE_NO_WINDOW)
                .spawn()
                .map_err(|e| format!("start {}: {e}", notifier.display()))?;
            let op = toast_op(account_id, note).to_string();
            child.stdin.take().ok_or("no stdin")?.write_all(op.as_bytes()).map_err(|e| e.to_string())?;
            let status = child.wait().map_err(|e| e.to_string())?;
            if !status.success() {
                return Err(format!("{} exited with {status}", notifier.display()));
            }
        }
        Ok(())
    }
}

/// Elsewhere the app shows no notifications yet.
#[cfg(not(any(target_os = "linux", windows)))]
mod platform {
    use tauri::AppHandle;

    use super::{Note, Notifier};

    pub fn show(_app: &AppHandle, _state: &Notifier, _account_id: i64, _notes: &[Note]) -> Result<(), String> {
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn toasts_take_maild_tags() {
        let one = Note { title: "Ann".into(), body: "Plan".into(), message_id: Some(42) };
        assert_eq!(
            toast_op(3, &one),
            serde_json::json!({"op": "show", "tag": "m42", "group": "mail", "title": "Ann", "body": "Plan", "open": 42})
        );
        let group = Note { title: "4 new messages".into(), body: "From Ann".into(), message_id: None };
        assert_eq!(toast_op(3, &group)["tag"], "a3");
        assert_eq!(toast_op(3, &group)["open"], 0);
    }

    #[test]
    fn reads_the_announced_notes() {
        let notes: Vec<Note> =
            serde_json::from_str(r#"[{"title":"Ann","body":"Plan","messageId":7},{"title":"4 new messages","body":"x"}]"#).unwrap();
        assert_eq!(notes[0].message_id, Some(7));
        assert_eq!(notes[1].message_id, None);
    }
}
