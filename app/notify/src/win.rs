//! The Windows side: toasts through WinRT under Frostmail's AppUserModelID,
//! maild's pipe, and starting the app.

use std::fs::File;
use std::io::BufReader;
use std::os::windows::fs::OpenOptionsExt;
use std::os::windows::process::CommandExt;
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use serde_json::Value;
use windows::Data::Xml::Dom::XmlDocument;
use windows::UI::Notifications::{ToastNotification, ToastNotificationManager};
use windows::core::HSTRING;
use windows_sys::Win32::Storage::FileSystem::SECURITY_IDENTIFICATION;

/// The app's AppUserModelID: its bundle identifier, which the installer
/// sets on the Start Menu shortcut. Windows shows toasts only for an ID
/// a shortcut (or a package) declares.
const AUMID: &str = "org.frostyard.Frostmail";

const DETACHED_PROCESS: u32 = 0x0000_0008;
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

/// Show a toast, replacing one with the same tag and group.
pub fn show(tag: &str, group: &str, xml: &str) -> Result<(), String> {
    let toast = || -> windows::core::Result<()> {
        let doc = XmlDocument::new()?;
        doc.LoadXml(&HSTRING::from(xml))?;
        let toast = ToastNotification::CreateToastNotification(&doc)?;
        toast.SetTag(&HSTRING::from(tag))?;
        toast.SetGroup(&HSTRING::from(group))?;
        ToastNotificationManager::CreateToastNotifierWithId(&HSTRING::from(AUMID))?.Show(&toast)
    };
    toast().map_err(|e| format!("show the toast: {e}"))
}

/// Remove a toast, from the screen and the notification center.
pub fn remove(tag: &str, group: &str) -> Result<(), String> {
    ToastNotificationManager::History()
        .and_then(|h| h.RemoveGroupedTagWithId(&HSTRING::from(tag), &HSTRING::from(group), &HSTRING::from(AUMID)))
        .map_err(|e| format!("remove the toast: {e}"))
}

/// Open the message (or the app, for 0): run the app beside the notifier,
/// or $FROSTMAIL_APP, with --open-message. A running app takes the request
/// and the new one exits (docs/design/desktop.md, One app instance).
pub fn open(message: i64) -> Result<(), String> {
    let app = match std::env::var_os("FROSTMAIL_APP").filter(|a| !a.is_empty()) {
        Some(app) => PathBuf::from(app),
        None => beside("frostmail.exe")?,
    };
    let mut cmd = Command::new(&app);
    if message != 0 {
        cmd.args(["--open-message", &message.to_string()]);
    }
    cmd.spawn().map(drop).map_err(|e| format!("start {}: {e}", app.display()))
}

/// Make one call to maild, starting it if it is not running.
pub fn call(method: &str, params: Value) -> Result<Value, String> {
    let pipe = connect()?;
    crate::rpc::call(BufReader::new(&pipe), &pipe, method, params)
}

/// maild's named pipe: $FROSTMAIL_SOCKET or \\.\pipe\frostmail-<the
/// user's SID> (docs/specs/rpc-protocol.md).
fn pipe_path() -> Result<PathBuf, String> {
    if let Some(p) = std::env::var_os("FROSTMAIL_SOCKET").filter(|p| !p.is_empty()) {
        return Ok(PathBuf::from(p));
    }
    Ok(PathBuf::from(format!(r"\\.\pipe\frostmail-{}", sid::user()?)))
}

/// Open maild's pipe at the identification level, which lets maild check
/// who is calling but not act as them. Without maild (a toast outlives
/// it), start the one beside the notifier and wait for its pipe.
fn connect() -> Result<File, String> {
    const ERROR_PIPE_BUSY: i32 = 231;
    let path = pipe_path()?;
    let deadline = Instant::now() + Duration::from_secs(10);
    let mut started = false;
    loop {
        let open = std::fs::OpenOptions::new().read(true).write(true).security_qos_flags(SECURITY_IDENTIFICATION).open(&path);
        match open {
            Ok(pipe) => return Ok(pipe),
            // maild makes a new instance as it accepts each.
            Err(e) if e.raw_os_error() == Some(ERROR_PIPE_BUSY) && Instant::now() < deadline => {}
            Err(e) if e.kind() == std::io::ErrorKind::NotFound && Instant::now() < deadline => {
                if !started {
                    start_maild()?;
                    started = true;
                }
            }
            Err(e) => return Err(format!("connect to maild at {}: {e}", path.display())),
        }
        std::thread::sleep(Duration::from_millis(50));
    }
}

/// Start the maild.exe beside the notifier, without a console. A second
/// maild started meanwhile finds the pipe taken and exits.
fn start_maild() -> Result<(), String> {
    let maild = beside("maild.exe")?;
    Command::new(&maild)
        .creation_flags(DETACHED_PROCESS | CREATE_NO_WINDOW)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .map(drop)
        .map_err(|e| format!("start {}: {e}", maild.display()))
}

/// A file installed beside the notifier.
fn beside(name: &str) -> Result<PathBuf, String> {
    std::env::current_exe().map(|exe| exe.with_file_name(name)).map_err(|e| format!("the notifier's path: {e}"))
}

mod sid {
    use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, LocalFree};
    use windows_sys::Win32::Security::Authorization::ConvertSidToStringSidW;
    use windows_sys::Win32::Security::{GetTokenInformation, TOKEN_QUERY, TOKEN_USER, TokenUser};
    use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

    /// The SID of the user the notifier runs as, in string form.
    pub fn user() -> Result<String, String> {
        // SAFETY: plain Win32 calls on the process's own token; every
        // buffer is sized by the first GetTokenInformation call, and the
        // handle and string are freed below.
        unsafe {
            let mut token: HANDLE = std::ptr::null_mut();
            if OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) == 0 {
                return Err(format!("the process's token: {}", std::io::Error::last_os_error()));
            }
            let mut len = 0u32;
            GetTokenInformation(token, TokenUser, std::ptr::null_mut(), 0, &mut len);
            let mut buf = vec![0u64; (len as usize).div_ceil(8)];
            let ok = GetTokenInformation(token, TokenUser, buf.as_mut_ptr().cast(), len, &mut len);
            CloseHandle(token);
            if ok == 0 {
                return Err(format!("the process's user: {}", std::io::Error::last_os_error()));
            }
            let user = &*(buf.as_ptr() as *const TOKEN_USER);
            let mut text: *mut u16 = std::ptr::null_mut();
            if ConvertSidToStringSidW(user.User.Sid, &mut text) == 0 {
                return Err(format!("the user's SID: {}", std::io::Error::last_os_error()));
            }
            let n = (0..).take_while(|&i| *text.add(i) != 0).count();
            let sid = String::from_utf16_lossy(std::slice::from_raw_parts(text, n));
            LocalFree(text.cast());
            Ok(sid)
        }
    }
}
