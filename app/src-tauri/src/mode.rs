//! Changing the mode (docs/specs/settings-ui.md, Server pane, Behavior
//! (container); ADR-0046, How duplicate engines are prevented): the app's
//! own choice of maild, app.json, and, in the Flatpak, this computer's
//! maild, in one fixed order. The order lives here, apart from the
//! processes it runs, so that a test drives it with steps that only record
//! their calls.

use std::process::{Command, Stdio};

use serde::Serialize;

use crate::paths;
use crate::remote::{AppConfig, Lodge};

/// The steps of a change of mode: the app's own configuration, and this
/// computer's maild. `Real` runs them; a test's steps record them.
pub trait Steps {
    /// Read the stored configuration.
    fn load(&self) -> Result<AppConfig, String>;
    /// Write the configuration.
    fn save(&self, cfg: &AppConfig) -> Result<(), String>;
    /// Stop this computer's maild, and return once it has stopped.
    fn stop_maild(&self) -> Result<(), String>;
    /// Start this computer's maild, and return once it answers.
    fn start_maild(&self) -> Result<(), String>;
}

/// A change to lodge whose maild did not stop: the previous choice went
/// back, and the windows stay on this computer's maild.
const DID_NOT_STOP: &str = "This computer's maild did not stop. Frostmail still uses it.";

/// A change to this computer whose maild did not start: the mode stays
/// local, and the launcher's line says why.
const DID_NOT_START: &str = "This computer's maild did not start: ";

/// What a change of mode did. `close` says whether every window's
/// connection must end, whatever the outcome: a change to this computer
/// starts maild or does not, and each window must try the mode's maild
/// either way. `error` is the words the Server pane shows.
#[derive(Debug, PartialEq, Eq)]
pub struct Outcome {
    pub close: bool,
    pub error: Option<String>,
}

impl Outcome {
    /// A change that succeeded, which needs the windows closed when close.
    fn done(close: bool) -> Self {
        Outcome { close, error: None }
    }

    /// A change that failed, with close as its steps left the windows.
    fn failed(close: bool, error: String) -> Self {
        Outcome { close, error: Some(error) }
    }
}

/// Change the app's choice of maild to next, in the order ADR-0046 fixes.
/// flatpak is paths::in_flatpak(): outside the Flatpak the app runs no
/// maild of its own, so it only writes the choice. In the Flatpak each mode
/// passes through this computer's maild:
/// - this computer to lodge: write the choice, then stop maild; when maild
///   does not stop, write the previous choice back, and leave the windows
///   on this computer's maild;
/// - lodge to this computer: write the choice, then start maild, and close
///   every window's connection either way;
/// - a new lodge address, or no change of mode: only write.
///
/// The new lodge address is checked before the first write, so a misspelled
/// one changes nothing.
pub fn change(steps: &impl Steps, flatpak: bool, next: AppConfig) -> Outcome {
    // The address first, before any write (docs/specs/settings-ui.md).
    if let Some(url) = &next.lodge {
        if let Err(e) = Lodge::parse(url) {
            return Outcome::failed(false, e);
        }
    }
    if !flatpak {
        return match steps.save(&next) {
            Ok(()) => Outcome::done(true),
            Err(e) => Outcome::failed(false, e),
        };
    }
    // The previous choice, before the first write: a failure here leaves
    // everything as it is, and nothing to write back.
    let prev = match steps.load() {
        Ok(cfg) => cfg,
        Err(e) => return Outcome::failed(false, e),
    };
    // Read the mode by presence: lodge is the mode whenever the key is set.
    match (prev.lodge.is_some(), next.lodge.is_some()) {
        (false, true) => {
            if let Err(e) = steps.save(&next) {
                return Outcome::failed(false, e);
            }
            match steps.stop_maild() {
                Ok(()) => Outcome::done(true),
                // mailctl's own words do not say what the pane needs to:
                // maild still runs, and the previous choice stands.
                Err(_) => match steps.save(&prev) {
                    Ok(()) => {
                        // A maild that took rpc.shutdown late can still
                        // exit: make sure this computer's maild runs, as
                        // the mode says. The launcher keeps one that
                        // answers, so this changes nothing when maild
                        // never stopped, and its failure adds nothing to
                        // the pane's words.
                        let _ = steps.start_maild();
                        Outcome::failed(false, DID_NOT_STOP.to_string())
                    }
                    Err(e) => Outcome::failed(false, format!("{DID_NOT_STOP} {e}")),
                },
            }
        }
        (true, false) => {
            if let Err(e) = steps.save(&next) {
                return Outcome::failed(false, e);
            }
            // The mode is local now: each window tries this computer's
            // maild, which starts or does not.
            match steps.start_maild() {
                Ok(()) => Outcome::done(true),
                Err(e) => Outcome::failed(true, format!("{DID_NOT_START}{e}")),
            }
        }
        // A new lodge address, or no change of mode: the file is all that
        // changes, and the windows reconnect to the maild now chosen.
        _ => match steps.save(&next) {
            Ok(()) => Outcome::done(true),
            Err(e) => Outcome::failed(false, e),
        },
    }
}

/// The app's own steps: remote::load and remote::save, and the launcher and
/// mailctl as bounded processes.
pub struct Real;

impl Steps for Real {
    fn load(&self) -> Result<AppConfig, String> {
        crate::remote::load()
    }

    fn save(&self, cfg: &AppConfig) -> Result<(), String> {
        crate::remote::save(cfg)
    }

    fn stop_maild(&self) -> Result<(), String> {
        // The launcher's own bound. mailctl's limit is 30 s in all: it
        // calls rpc.shutdown, asks again while a maild answers, and returns
        // once the socket is silent and the data directory's lock is free.
        run("35", "mailctl", &["shutdown"])
    }

    fn start_maild(&self) -> Result<(), String> {
        // The launcher starts maild through the Flatpak portal as a normal
        // start does, and exits 0 once maild answers.
        run("60", "frostmail", &["--start-maild"])
    }
}

/// run runs program with args for at most secs seconds, with no input, and
/// holds its output. A failure's full output goes to the app's stderr, and
/// the error is the last non-empty line of its stderr, else of its stdout,
/// else its exit status.
fn run(secs: &str, program: &str, args: &[&str]) -> Result<(), String> {
    let out = Command::new("timeout")
        // --kill-after: a command that catches SIGTERM, or a flatpak-spawn
        // that forwards it to a portal call that hangs, gets SIGKILL 1 s on.
        .args(["--kill-after=1", secs])
        .arg(program)
        .args(args)
        .stdin(Stdio::null())
        .output()
        .map_err(|e| format!("running {program}: {e}"))?;
    if out.status.success() {
        return Ok(());
    }
    let stdout = String::from_utf8_lossy(&out.stdout);
    let stderr = String::from_utf8_lossy(&out.stderr);
    eprintln!("{program} {}: {}; output:\n{stdout}{stderr}", args.join(" "), out.status);
    match last_line(&stderr).or_else(|| last_line(&stdout)) {
        Some(line) => Err(line.to_string()),
        None => Err(match out.status.code() {
            Some(code) => format!("exit status {code}"),
            None => out.status.to_string(),
        }),
    }
}

/// The last line of text that holds more than spaces.
fn last_line(text: &str) -> Option<&str> {
    text.lines().map(str::trim).rev().find(|line| !line.is_empty())
}

/// The Server pane's local facts (docs/specs/settings-ui.md, Server pane,
/// Behavior (container)): whether this is the Flatpak, and whether maild's
/// data directory on this computer holds a database.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ServerFacts {
    pub flatpak: bool,
    pub local_data: bool,
}

/// The Server pane's local facts, from the window's own environment: no
/// request to maild, and none to lodge, so the pane says the same while
/// either is unreachable.
pub fn server_facts() -> ServerFacts {
    ServerFacts { flatpak: paths::in_flatpak(), local_data: db_exists(paths::data_dir()) }
}

/// Whether maild has made its database. Only the file's metadata is read:
/// the database belongs to maild, which holds it, and the window never
/// opens it. False on any error, such as a missing HOME.
fn db_exists(dir: Result<std::path::PathBuf, String>) -> bool {
    dir.is_ok_and(|dir| dir.join("frostmail.db").exists())
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::collections::VecDeque;

    use super::*;

    /// The steps a test records: what load answers, what each step answers,
    /// and the calls in order, with the configurations they wrote.
    #[derive(Default)]
    struct Fake {
        prev: AppConfig,
        /// The error stop_maild returns, when it fails.
        stop: Option<String>,
        /// The error start_maild returns, when it fails.
        start: Option<String>,
        /// Each save's error, in order; no entry writes.
        saves: RefCell<VecDeque<Option<String>>>,
        calls: RefCell<Vec<&'static str>>,
        saved: RefCell<Vec<AppConfig>>,
    }

    impl Fake {
        fn calls(&self) -> Vec<&'static str> {
            self.calls.borrow().clone()
        }

        fn saved(&self) -> Vec<AppConfig> {
            self.saved.borrow().clone()
        }
    }

    impl Steps for Fake {
        fn load(&self) -> Result<AppConfig, String> {
            self.calls.borrow_mut().push("load");
            Ok(self.prev.clone())
        }

        fn save(&self, cfg: &AppConfig) -> Result<(), String> {
            self.calls.borrow_mut().push("save");
            self.saved.borrow_mut().push(cfg.clone());
            match self.saves.borrow_mut().pop_front().flatten() {
                Some(e) => Err(e),
                None => Ok(()),
            }
        }

        fn stop_maild(&self) -> Result<(), String> {
            self.calls.borrow_mut().push("stop");
            match &self.stop {
                Some(e) => Err(e.clone()),
                None => Ok(()),
            }
        }

        fn start_maild(&self) -> Result<(), String> {
            self.calls.borrow_mut().push("start");
            match &self.start {
                Some(e) => Err(e.clone()),
                None => Ok(()),
            }
        }
    }

    const LODGE: &str = "https://lodge.example.ts.net";

    fn lodge(url: &str) -> AppConfig {
        AppConfig { lodge: Some(url.to_string()) }
    }

    /// This computer's maild, the default configuration.
    fn local() -> AppConfig {
        AppConfig::default()
    }

    /// The answers each save gives, in order: none writes.
    fn saves(answers: Vec<Option<&str>>) -> RefCell<VecDeque<Option<String>>> {
        RefCell::new(answers.into_iter().map(|answer| answer.map(str::to_string)).collect())
    }

    #[test]
    fn local_to_lodge_writes_then_stops_maild() {
        let fake = Fake::default();
        let out = change(&fake, true, lodge(LODGE));
        assert_eq!(out.error, None);
        assert!(out.close);
        // The previous choice comes first: it is read before the write.
        assert_eq!(fake.calls(), ["load", "save", "stop"]);
        assert_eq!(fake.saved(), [lodge(LODGE)]);
    }

    #[test]
    fn a_maild_that_does_not_stop_keeps_this_computer() {
        let fake = Fake { stop: Some("maild still holds the lock".to_string()), ..Fake::default() };
        let out = change(&fake, true, lodge(LODGE));
        assert_eq!(out.error.as_deref(), Some(DID_NOT_STOP));
        // The windows stay on this computer's maild: nothing to reconnect to.
        assert!(!out.close);
        // The choice went back, and this computer's maild is made sure of.
        assert_eq!(fake.calls(), ["load", "save", "stop", "save", "start"]);
        assert_eq!(fake.saved(), [lodge(LODGE), local()]);
    }

    #[test]
    fn a_write_back_that_fails_is_added_to_the_words() {
        let fake = Fake {
            stop: Some("maild still holds the lock".to_string()),
            saves: saves(vec![None, Some("app.json: read-only file system")]),
            ..Fake::default()
        };
        let out = change(&fake, true, lodge(LODGE));
        assert_eq!(
            out.error.as_deref(),
            Some("This computer's maild did not stop. Frostmail still uses it. app.json: read-only file system")
        );
        assert!(!out.close);
        assert_eq!(fake.calls(), ["load", "save", "stop", "save"]);
    }

    #[test]
    fn lodge_to_local_writes_then_starts_maild() {
        let fake = Fake { prev: lodge(LODGE), ..Fake::default() };
        let out = change(&fake, true, local());
        assert_eq!(out.error, None);
        assert!(out.close);
        assert_eq!(fake.calls(), ["load", "save", "start"]);
        assert_eq!(fake.saved(), [local()]);
    }

    #[test]
    fn a_maild_that_does_not_start_closes_the_windows_and_says_why() {
        let line = "frostmail: maild did not start through the Flatpak portal (flatpak-spawn); see /state/maild.log";
        let fake = Fake { prev: lodge(LODGE), start: Some(line.to_string()), ..Fake::default() };
        let out = change(&fake, true, local());
        // The mode is local now: every window must try this computer's maild.
        assert!(out.close);
        let want = format!("This computer's maild did not start: {line}");
        assert_eq!(out.error.as_deref(), Some(want.as_str()));
        assert_eq!(fake.calls(), ["load", "save", "start"]);
    }

    #[test]
    fn a_new_lodge_address_only_writes() {
        let fake = Fake { prev: lodge(LODGE), ..Fake::default() };
        let out = change(&fake, true, lodge("https://lodge.example.test.ts.net"));
        assert_eq!(out.error, None);
        assert!(out.close);
        // No stop and no start: no maild of this computer changes.
        assert_eq!(fake.calls(), ["load", "save"]);
        assert_eq!(fake.saved(), [lodge("https://lodge.example.test.ts.net")]);
    }

    #[test]
    fn no_change_of_mode_only_writes() {
        let fake = Fake::default();
        let out = change(&fake, true, local());
        assert_eq!(out.error, None);
        assert!(out.close);
        assert_eq!(fake.calls(), ["load", "save"]);
        assert_eq!(fake.saved(), [local()]);
    }

    #[test]
    fn outside_the_flatpak_it_only_writes() {
        // No mode to change: the app runs no maild of its own, so it does
        // not even read the previous choice.
        for next in [lodge(LODGE), local()] {
            let fake = Fake { prev: lodge(LODGE), ..Fake::default() };
            let out = change(&fake, false, next.clone());
            assert_eq!(out.error, None);
            assert!(out.close);
            assert_eq!(fake.calls(), ["save"]);
            assert_eq!(fake.saved(), [next]);
        }
    }

    #[test]
    fn a_bad_address_changes_nothing() {
        for flatpak in [true, false] {
            let fake = Fake::default();
            let out = change(&fake, flatpak, lodge("ftp://lodge.example.ts.net"));
            assert!(!out.close);
            assert!(out.error.as_deref().is_some_and(|e| e.contains("lodge's address")), "{:?}", out.error);
            assert!(fake.calls().is_empty(), "{:?}", fake.calls());
        }
    }

    #[test]
    fn a_failed_write_changes_nothing() {
        let fake = Fake {
            saves: saves(vec![Some("app.json: read-only file system")]),
            ..Fake::default()
        };
        let out = change(&fake, true, lodge(LODGE));
        assert_eq!(out.error.as_deref(), Some("app.json: read-only file system"));
        assert!(!out.close);
        // No stop: the mode never changes half-way.
        assert_eq!(fake.calls(), ["load", "save"]);
    }

    #[test]
    fn a_change_that_cannot_read_the_choice_changes_nothing() {
        // load fails: there is no previous choice to write back, so the
        // change must not start.
        struct NoLoad;
        impl Steps for NoLoad {
            fn load(&self) -> Result<AppConfig, String> {
                Err("app.json: permission denied".to_string())
            }
            fn save(&self, _cfg: &AppConfig) -> Result<(), String> {
                panic!("a change that cannot read the choice wrote one")
            }
            fn stop_maild(&self) -> Result<(), String> {
                panic!("a change that cannot read the choice stopped maild")
            }
            fn start_maild(&self) -> Result<(), String> {
                panic!("a change that cannot read the choice started maild")
            }
        }
        let out = change(&NoLoad, true, lodge(LODGE));
        assert_eq!(out.error.as_deref(), Some("app.json: permission denied"));
        assert!(!out.close);
    }

    #[test]
    fn the_facts_say_what_the_pane_can_show() {
        // The names the container reads (docs/specs/settings-ui.md).
        let facts = ServerFacts { flatpak: true, local_data: false };
        assert_eq!(serde_json::to_string(&facts).unwrap(), r#"{"flatpak":true,"localData":false}"#);
    }

    #[test]
    fn the_database_is_a_file_in_the_data_directory() {
        let dir = std::env::temp_dir().join(format!("frostmail-facts-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        assert!(!db_exists(Ok(dir.clone())));
        std::fs::write(dir.join("frostmail.db"), b"").unwrap();
        assert!(db_exists(Ok(dir.clone())));
        assert!(!db_exists(Err("HOME is not set".to_string())));
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn the_last_line_of_the_output_is_the_reason() {
        assert_eq!(last_line(""), None);
        assert_eq!(last_line("\n  \n"), None);
        assert_eq!(last_line("first\n\n  last  \n"), Some("last"));
        assert_eq!(last_line("only"), Some("only"));
    }
}
