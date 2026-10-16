//! The frostmail-toast: URIs a toast's click, Snooze and Dismiss hand to
//! the notifier. The installer registers the scheme for the user, so any
//! program the user runs can send one too: each does no more than the
//! user could with a click (open the app or a message, snooze or dismiss
//! a reminder whose id it would have to know).

/// The URI scheme, registered by the installer.
pub const SCHEME: &str = "frostmail-toast";

/// What a click asks for.
#[derive(Debug, PartialEq)]
pub enum Action {
    /// Open the message, or the app for 0.
    Open(i64),
    /// Snooze the reminder for ten minutes.
    Snooze(String),
    /// Dismiss the reminder.
    Dismiss(String),
}

impl Action {
    pub fn uri(&self) -> String {
        match self {
            Action::Open(0) => format!("{SCHEME}:open"),
            Action::Open(id) => format!("{SCHEME}:open?message={id}"),
            Action::Snooze(id) => format!("{SCHEME}:snooze?reminder={}", encode(id)),
            Action::Dismiss(id) => format!("{SCHEME}:dismiss?reminder={}", encode(id)),
        }
    }

    /// Read a URI made by uri. Anything else is refused.
    pub fn parse(uri: &str) -> Result<Action, String> {
        let bad = || format!("not a toast activation: {uri:?}");
        let (scheme, rest) = uri.split_once(':').ok_or_else(bad)?;
        if !scheme.eq_ignore_ascii_case(SCHEME) {
            return Err(bad());
        }
        let (verb, query) = rest.split_once('?').unwrap_or((rest, ""));
        // A shell may end the path with a slash.
        let verb = verb.strip_suffix('/').unwrap_or(verb);
        let (key, value) = if query.is_empty() { ("", "") } else { query.split_once('=').ok_or_else(bad)? };
        match (verb, key) {
            ("open", "") => Ok(Action::Open(0)),
            ("open", "message") => match value.parse::<i64>() {
                Ok(id) if id > 0 && value.bytes().all(|b| b.is_ascii_digit()) => Ok(Action::Open(id)),
                _ => Err(bad()),
            },
            ("snooze", "reminder") => Ok(Action::Snooze(reminder(value).ok_or_else(bad)?)),
            ("dismiss", "reminder") => Ok(Action::Dismiss(reminder(value).ok_or_else(bad)?)),
            _ => Err(bad()),
        }
    }
}

fn reminder(value: &str) -> Option<String> {
    decode(value).filter(|id| !id.is_empty())
}

/// Percent-encode everything but RFC 3986's unreserved characters.
fn encode(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for b in s.bytes() {
        if b.is_ascii_alphanumeric() || b"-._~".contains(&b) {
            out.push(b as char);
        } else {
            out.push_str(&format!("%{b:02X}"));
        }
    }
    out
}

/// Undo encode; None for a malformed escape or text that is not UTF-8.
fn decode(s: &str) -> Option<String> {
    let mut out = Vec::with_capacity(s.len());
    let mut bytes = s.bytes();
    while let Some(b) = bytes.next() {
        if b == b'%' {
            let hex = [bytes.next()?, bytes.next()?];
            out.push(u8::from_str_radix(std::str::from_utf8(&hex).ok()?, 16).ok()?);
        } else {
            out.push(b);
        }
    }
    String::from_utf8(out).ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trip() {
        for a in [
            Action::Open(0),
            Action::Open(42),
            Action::Snooze("17/20261016T090000Z".into()),
            Action::Dismiss("a b&c=d?é/%".into()),
        ] {
            assert_eq!(Action::parse(&a.uri()).unwrap(), a, "{}", a.uri());
        }
    }

    #[test]
    fn shells_may_vary_the_case_and_add_a_slash() {
        assert_eq!(Action::parse("FROSTMAIL-TOAST:open/?message=7").unwrap(), Action::Open(7));
        assert_eq!(Action::parse("frostmail-toast:open/").unwrap(), Action::Open(0));
    }

    #[test]
    fn others_are_refused() {
        for uri in [
            "",
            "open",
            "frostmail:open",
            "frostmail-toast:",
            "frostmail-toast:delete?message=1",
            "frostmail-toast:open?message=",
            "frostmail-toast:open?message=0",
            "frostmail-toast:open?message=-1",
            "frostmail-toast:open?message=+1",
            "frostmail-toast:open?message=1x",
            "frostmail-toast:open?account=1",
            "frostmail-toast:snooze",
            "frostmail-toast:snooze?reminder=",
            "frostmail-toast:snooze?reminder=%4",
            "frostmail-toast:snooze?reminder=%zz",
            "frostmail-toast:dismiss?reminder=%ff",
            "frostmail-toast:dismiss?message=1",
        ] {
            assert!(Action::parse(uri).is_err(), "{uri}");
        }
    }
}
