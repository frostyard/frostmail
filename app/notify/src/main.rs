//! frostmail-notify, Frostmail's toast notifier on Windows
//! (docs/design/desktop.md, Windows). maild runs it once per change with
//! one JSON operation on stdin, which shows or removes a toast. Windows
//! runs it with --activate <uri> when a toast, or its Snooze or Dismiss,
//! is clicked: it opens the app or the message, or calls maild.
//!
//! It is a GUI program, so neither run opens a console; maild reads what
//! it writes to stderr.
#![cfg_attr(windows, windows_subsystem = "windows")]

mod activate;
mod rpc;
mod toast;
#[cfg(windows)]
mod win;

use std::io::Read;
use std::process::ExitCode;
use std::time::Duration;

/// A run that takes longer (a toast platform or maild that never answers)
/// is stopped.
const LIMIT: Duration = Duration::from_secs(30);

fn main() -> ExitCode {
    std::thread::spawn(|| {
        std::thread::sleep(LIMIT);
        eprintln!("frostmail-notify: gave up after {LIMIT:?}");
        std::process::exit(2);
    });
    let args: Vec<String> = std::env::args().skip(1).collect();
    let result = match args.as_slice() {
        [] => from_maild(),
        [flag, uri] if flag == "--activate" => activate::Action::parse(uri).and_then(act),
        _ => Err("usage: frostmail-notify < operation.json | frostmail-notify --activate URI".into()),
    };
    match result {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("frostmail-notify: {e}");
            ExitCode::FAILURE
        }
    }
}

/// Show or remove the toast maild asks for.
fn from_maild() -> Result<(), String> {
    let mut input = Vec::new();
    std::io::stdin().read_to_end(&mut input).map_err(|e| format!("read the operation: {e}"))?;
    match toast::parse(&input)? {
        toast::Command::Show { tag, group, xml } => platform::show(&tag, &group, &xml),
        toast::Command::Remove { tag, group } => platform::remove(&tag, &group),
    }
}

/// Do what a click asked for.
fn act(action: activate::Action) -> Result<(), String> {
    use activate::Action;
    match action {
        Action::Open(message) => platform::open(message),
        Action::Snooze(id) => platform::call("calendar.snooze", rpc::snooze(&id, std::time::SystemTime::now())).map(drop),
        Action::Dismiss(id) => platform::call("calendar.dismiss", rpc::dismiss(&id)).map(drop),
    }
}

#[cfg(windows)]
use win as platform;

/// Elsewhere there are no toasts; the notifier exists to be built and
/// tested with the app.
#[cfg(not(windows))]
mod platform {
    const ONLY: &str = "toasts are Windows's";

    pub fn show(_tag: &str, _group: &str, _xml: &str) -> Result<(), String> {
        Err(ONLY.into())
    }

    pub fn remove(_tag: &str, _group: &str) -> Result<(), String> {
        Err(ONLY.into())
    }

    pub fn open(_message: i64) -> Result<(), String> {
        Err(ONLY.into())
    }

    pub fn call(_method: &str, _params: serde_json::Value) -> Result<serde_json::Value, String> {
        Err(ONLY.into())
    }
}
