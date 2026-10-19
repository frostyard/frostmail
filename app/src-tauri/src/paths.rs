//! File locations, mirroring internal/config: the overrides, then the XDG
//! base directories on Linux, or the user's local app data and per-user
//! pipe on Windows (ADR-0038).

use std::path::PathBuf;

fn env_abs(key: &str) -> Option<PathBuf> {
    std::env::var_os(key).map(PathBuf::from).filter(|p| p.is_absolute())
}

/// maild's socket: $FROSTMAIL_SOCKET or $XDG_RUNTIME_DIR/frostmail/maild.sock.
#[cfg(not(windows))]
pub fn socket() -> Result<PathBuf, String> {
    if let Some(p) = env_abs("FROSTMAIL_SOCKET") {
        return Ok(p);
    }
    env_abs("XDG_RUNTIME_DIR")
        .map(|d| d.join("frostmail").join("maild.sock"))
        .ok_or_else(|| "XDG_RUNTIME_DIR is not set; set FROSTMAIL_SOCKET".to_string())
}

/// maild's named pipe: $FROSTMAIL_SOCKET or \\.\pipe\frostmail-<the
/// user's SID>.
#[cfg(windows)]
pub fn socket() -> Result<PathBuf, String> {
    if let Some(p) = std::env::var_os("FROSTMAIL_SOCKET").filter(|p| !p.is_empty()) {
        return Ok(PathBuf::from(p));
    }
    Ok(PathBuf::from(format!(r"\\.\pipe\frostmail-{}", sid::user()?)))
}

/// The cache maild writes decoded parts into:
/// $FROSTMAIL_CACHE_DIR or $XDG_CACHE_HOME/frostmail or ~/.cache/frostmail.
/// In a Flatpak, XDG_CACHE_HOME is the app's own, so the host's
/// ($HOST_XDG_CACHE_HOME, else ~/.cache) is used instead (ADR-0013).
#[cfg(not(windows))]
pub fn cache_dir() -> Result<PathBuf, String> {
    if let Some(p) = env_abs("FROSTMAIL_CACHE_DIR") {
        return Ok(p);
    }
    let xdg = if std::env::var_os("FLATPAK_ID").is_some() { "HOST_XDG_CACHE_HOME" } else { "XDG_CACHE_HOME" };
    if let Some(p) = env_abs(xdg) {
        return Ok(p.join("frostmail"));
    }
    env_abs("HOME")
        .map(|h| h.join(".cache").join("frostmail"))
        .ok_or_else(|| "HOME is not set".to_string())
}

/// The cache maild writes decoded parts into: $FROSTMAIL_CACHE_DIR or
/// %LOCALAPPDATA%\Frostmail\Cache.
#[cfg(windows)]
pub fn cache_dir() -> Result<PathBuf, String> {
    if let Some(p) = env_abs("FROSTMAIL_CACHE_DIR") {
        return Ok(p);
    }
    env_abs("LOCALAPPDATA")
        .map(|d| d.join("Frostmail").join("Cache"))
        .ok_or_else(|| "LOCALAPPDATA is not set".to_string())
}

/// The app's own configuration, never maild's: which maild it asks
/// (docs/design/desktop.md, On a maild behind lodge). $XDG_CONFIG_HOME/frostmail
/// or ~/.config/frostmail; in a Flatpak, XDG_CONFIG_HOME is the app's own.
#[cfg(not(windows))]
pub fn config_dir() -> Result<PathBuf, String> {
    if let Some(p) = env_abs("XDG_CONFIG_HOME") {
        return Ok(p.join("frostmail"));
    }
    env_abs("HOME")
        .map(|h| h.join(".config").join("frostmail"))
        .ok_or_else(|| "HOME is not set".to_string())
}

/// The app's own configuration: %APPDATA%\Frostmail.
#[cfg(windows)]
pub fn config_dir() -> Result<PathBuf, String> {
    env_abs("APPDATA")
        .map(|d| d.join("Frostmail"))
        .ok_or_else(|| "APPDATA is not set".to_string())
}

/// Where the app keeps attachments it downloads from a maild behind lodge,
/// apart from maild's own cache: $XDG_CACHE_HOME/frostmail-app/attachments,
/// the Flatpak's own cache there.
#[cfg(not(windows))]
pub fn downloads_dir() -> Result<PathBuf, String> {
    let base = env_abs("XDG_CACHE_HOME")
        .or_else(|| env_abs("HOME").map(|h| h.join(".cache")))
        .ok_or_else(|| "HOME is not set".to_string())?;
    Ok(base.join("frostmail-app").join("attachments"))
}

/// Where the app keeps attachments it downloads from a maild behind lodge:
/// %LOCALAPPDATA%\Frostmail\Attachments.
#[cfg(windows)]
pub fn downloads_dir() -> Result<PathBuf, String> {
    env_abs("LOCALAPPDATA")
        .map(|d| d.join("Frostmail").join("Attachments"))
        .ok_or_else(|| "LOCALAPPDATA is not set".to_string())
}

#[cfg(windows)]
mod sid {
    use windows_sys::Win32::Foundation::{CloseHandle, HANDLE, LocalFree};
    use windows_sys::Win32::Security::Authorization::ConvertSidToStringSidW;
    use windows_sys::Win32::Security::{GetTokenInformation, TOKEN_QUERY, TOKEN_USER, TokenUser};
    use windows_sys::Win32::System::Threading::{GetCurrentProcess, OpenProcessToken};

    /// The SID of the user the app runs as, in string form.
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
