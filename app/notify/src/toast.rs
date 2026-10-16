//! The operation maild writes on the notifier's stdin, and the toast XML it
//! becomes (docs/design/desktop.md, Windows).

use serde::Deserialize;

use crate::activate::Action;

/// A tag or group holds at most 64 characters.
const MAX_NAME: usize = 64;

/// One operation from maild: show a toast, replacing one with the same tag
/// and group, or remove one. Unknown members are ignored, since maild's
/// side may grow (additive, like the RPC protocol).
#[derive(Debug, Deserialize)]
struct Op {
    op: String,
    tag: String,
    group: String,
    #[serde(default)]
    title: String,
    #[serde(default)]
    body: String,
    /// The message a click opens; 0 opens the app.
    #[serde(default)]
    open: i64,
    /// The calendar.reminders id the toast's Snooze and Dismiss act on.
    #[serde(default)]
    reminder: String,
}

/// What the notifier does with an operation.
#[derive(Debug, PartialEq)]
pub enum Command {
    Show { tag: String, group: String, xml: String },
    Remove { tag: String, group: String },
}

/// Read one operation.
pub fn parse(input: &[u8]) -> Result<Command, String> {
    let op: Op = serde_json::from_slice(input).map_err(|e| format!("the operation: {e}"))?;
    for (what, name) in [("tag", &op.tag), ("group", &op.group)] {
        if name.is_empty() || name.chars().count() > MAX_NAME {
            return Err(format!("the {what} must have 1 to {MAX_NAME} characters"));
        }
    }
    match op.op.as_str() {
        "show" => {
            if op.open < 0 {
                return Err("open must be a message id".into());
            }
            let xml = xml(&op);
            Ok(Command::Show { tag: op.tag, group: op.group, xml })
        }
        "remove" => Ok(Command::Remove { tag: op.tag, group: op.group }),
        other => Err(format!("unknown op {other:?}")),
    }
}

/// The toast's XML. Every activation is a frostmail-toast: URI, which
/// Windows hands to the notifier (--activate) whether or not anything of
/// Frostmail's is running. A reminder stays on screen until acted on, like
/// its Linux notification, with Snooze and Dismiss; clicking it opens the
/// app, whose reminder window shows it.
fn xml(op: &Op) -> String {
    let reminder = !op.reminder.is_empty();
    let launch = if reminder { Action::Open(0) } else { Action::Open(op.open) };
    let mut x = format!(r#"<toast launch="{}" activationType="protocol""#, escape(&launch.uri()));
    if reminder {
        x.push_str(r#" scenario="reminder""#);
    }
    x.push_str(r#"><visual><binding template="ToastGeneric">"#);
    x.push_str(&format!("<text>{}</text>", escape(&op.title)));
    if !op.body.is_empty() {
        x.push_str(&format!("<text>{}</text>", escape(&op.body)));
    }
    x.push_str("</binding></visual>");
    if reminder {
        x.push_str("<actions>");
        for (label, action) in [
            ("Snooze", Action::Snooze(op.reminder.clone())),
            ("Dismiss", Action::Dismiss(op.reminder.clone())),
        ] {
            x.push_str(&format!(
                r#"<action content="{label}" activationType="protocol" arguments="{}"/>"#,
                escape(&action.uri())
            ));
        }
        x.push_str("</actions>");
    }
    x.push_str("</toast>");
    x
}

/// Escape text for an XML element or attribute, dropping the characters
/// XML 1.0 cannot hold (control characters a subject may carry), which
/// would make Windows refuse the whole toast.
fn escape(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '&' => out.push_str("&amp;"),
            '<' => out.push_str("&lt;"),
            '>' => out.push_str("&gt;"),
            '"' => out.push_str("&quot;"),
            '\'' => out.push_str("&apos;"),
            '\t' | '\n' | '\r' => out.push(c),
            c if c < ' ' || c == '\u{FFFE}' || c == '\u{FFFF}' => {}
            c => out.push(c),
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn show(input: &str) -> String {
        match parse(input.as_bytes()).unwrap() {
            Command::Show { xml, .. } => xml,
            other => panic!("not a show: {other:?}"),
        }
    }

    #[test]
    fn mail_opens_its_message() {
        let xml = show(r#"{"op":"show","tag":"m42","group":"mail","title":"Ann","body":"Lunch?","open":42}"#);
        assert_eq!(
            xml,
            r#"<toast launch="frostmail-toast:open?message=42" activationType="protocol"><visual><binding template="ToastGeneric"><text>Ann</text><text>Lunch?</text></binding></visual></toast>"#
        );
    }

    #[test]
    fn a_group_note_opens_the_app() {
        let xml = show(r#"{"op":"show","tag":"a3","group":"mail","title":"5 new messages"}"#);
        assert!(xml.starts_with(r#"<toast launch="frostmail-toast:open" "#), "{xml}");
        assert!(!xml.contains("<text></text>"), "{xml}");
    }

    #[test]
    fn a_reminder_has_snooze_and_dismiss() {
        let xml = show(r#"{"op":"show","tag":"r00","group":"reminders","title":"Standup","body":"10:00","reminder":"7/a b&c"}"#);
        assert!(xml.contains(r#"launch="frostmail-toast:open" activationType="protocol" scenario="reminder">"#), "{xml}");
        assert!(xml.contains(r#"<action content="Snooze" activationType="protocol" arguments="frostmail-toast:snooze?reminder=7%2Fa%20b%26c"/>"#), "{xml}");
        assert!(xml.contains(r#"<action content="Dismiss" activationType="protocol" arguments="frostmail-toast:dismiss?reminder=7%2Fa%20b%26c"/>"#), "{xml}");
    }

    #[test]
    fn text_is_escaped() {
        let xml = show(r#"{"op":"show","tag":"m1","group":"mail","title":"<b>\"A&B\" 'x'</b>","body":"a\u0001b\u0000c\td","open":1}"#);
        assert!(xml.contains("<text>&lt;b&gt;&quot;A&amp;B&quot; &apos;x&apos;&lt;/b&gt;</text>"), "{xml}");
        assert!(xml.contains("<text>abc\td</text>"), "{xml}");
    }

    #[test]
    fn remove() {
        assert_eq!(
            parse(br#"{"op":"remove","tag":"r1","group":"reminders"}"#).unwrap(),
            Command::Remove { tag: "r1".into(), group: "reminders".into() }
        );
    }

    #[test]
    fn unknown_members_are_ignored() {
        show(r#"{"op":"show","tag":"m1","group":"mail","title":"t","open":1,"later":true}"#);
    }

    #[test]
    fn bad_operations() {
        for input in [
            "",
            "{}",
            r#"{"op":"blink","tag":"m1","group":"mail"}"#,
            r#"{"op":"show","tag":"","group":"mail"}"#,
            r#"{"op":"show","tag":"m1","group":""}"#,
            r#"{"op":"show","tag":"m1","group":"mail","open":-1}"#,
            &format!(r#"{{"op":"remove","tag":"{}","group":"mail"}}"#, "x".repeat(65)),
        ] {
            assert!(parse(input.as_bytes()).is_err(), "{input}");
        }
    }
}
