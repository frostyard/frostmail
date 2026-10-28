//! File locations, mirroring internal/config: the overrides, then the XDG
//! base directories on Linux, or the user's local app data and per-user
//! pipe on Windows (ADR-0038). Each public function reads the process's
//! environment through a `_from` function that takes the lookup, so the
//! tests set their own.

use std::ffi::OsString;
use std::path::PathBuf;

/// The application ID, as internal/config.AppID.
const APP_ID: &str = "org.frostyard.Frostmail";

fn abs(value: Option<OsString>) -> Option<PathBuf> {
    value.map(PathBuf::from).filter(|p| p.is_absolute())
}

/// Whether this process runs in this application's Flatpak: FLATPAK_ID is
/// the app ID, as config.InFlatpak in internal/config. Another
/// application's Flatpak is not. Windows has no Flatpak, and sets no
/// FLATPAK_ID (the app is installed per user, ADR-0038).
pub fn in_flatpak() -> bool {
    in_flatpak_from(|key| std::env::var_os(key))
}

fn in_flatpak_from(getenv: impl Fn(&str) -> Option<OsString>) -> bool {
    getenv("FLATPAK_ID").is_some_and(|id| id == APP_ID)
}

/// maild's socket: $FROSTMAIL_SOCKET or $XDG_RUNTIME_DIR/frostmail/maild.sock.
/// In the Flatpak (FLATPAK_ID is the app ID) it is
/// $XDG_RUNTIME_DIR/app/org.frostyard.Frostmail/frostmail/maild.sock, the one
/// directory Flatpak shares between the app's sandboxes (ADR-0046). Keep in
/// step with config.DefaultSocket in internal/config.
#[cfg(not(windows))]
pub fn socket() -> Result<PathBuf, String> {
    socket_from(|key| std::env::var_os(key))
}

#[cfg(not(windows))]
fn socket_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = abs(getenv("FROSTMAIL_SOCKET")) {
        return Ok(p);
    }
    abs(getenv("XDG_RUNTIME_DIR"))
        .map(|d| {
            if in_flatpak_from(&getenv) {
                d.join("app").join(APP_ID).join("frostmail").join("maild.sock")
            } else {
                d.join("frostmail").join("maild.sock")
            }
        })
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

/// maild's data directory, where its database and blob store are:
/// $FROSTMAIL_DATA_DIR when it is absolute, else $XDG_DATA_HOME/frostmail,
/// else ~/.local/share/frostmail. Keep in step with config.Resolve and
/// defaultDataDir in internal/config. A relative FROSTMAIL_DATA_DIR names a
/// directory in maild's own working directory, which the app cannot know,
/// so the app reads the default instead.
#[cfg(not(windows))]
pub fn data_dir() -> Result<PathBuf, String> {
    data_dir_from(|key| std::env::var_os(key))
}

#[cfg(not(windows))]
fn data_dir_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = abs(getenv("FROSTMAIL_DATA_DIR")) {
        return Ok(p);
    }
    if let Some(p) = abs(getenv("XDG_DATA_HOME")) {
        return Ok(p.join("frostmail"));
    }
    abs(getenv("HOME"))
        .map(|h| h.join(".local").join("share").join("frostmail"))
        .ok_or_else(|| "HOME is not set".to_string())
}

/// maild's data directory: $FROSTMAIL_DATA_DIR when it is absolute, else
/// %LOCALAPPDATA%\Frostmail\Data (defaultDataDir in
/// internal/config/paths_windows.go).
#[cfg(windows)]
pub fn data_dir() -> Result<PathBuf, String> {
    if let Some(p) = abs(std::env::var_os("FROSTMAIL_DATA_DIR")) {
        return Ok(p);
    }
    abs(std::env::var_os("LOCALAPPDATA"))
        .map(|d| d.join("Frostmail").join("Data"))
        .ok_or_else(|| "LOCALAPPDATA is not set".to_string())
}

/// The cache maild writes decoded parts into:
/// $FROSTMAIL_CACHE_DIR or $XDG_CACHE_HOME/frostmail or ~/.cache/frostmail.
/// In a Flatpak, XDG_CACHE_HOME is the app's own, the directory maild's
/// instance uses too (ADR-0046); the development bundle's host maild is
/// reached through FROSTMAIL_CACHE_DIR, which install.sh sets.
#[cfg(not(windows))]
pub fn cache_dir() -> Result<PathBuf, String> {
    cache_dir_from(|key| std::env::var_os(key))
}

#[cfg(not(windows))]
fn cache_dir_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = abs(getenv("FROSTMAIL_CACHE_DIR")) {
        return Ok(p);
    }
    if let Some(p) = abs(getenv("XDG_CACHE_HOME")) {
        return Ok(p.join("frostmail"));
    }
    abs(getenv("HOME"))
        .map(|h| h.join(".cache").join("frostmail"))
        .ok_or_else(|| "HOME is not set".to_string())
}

/// The cache maild writes decoded parts into: $FROSTMAIL_CACHE_DIR or
/// %LOCALAPPDATA%\Frostmail\Cache.
#[cfg(windows)]
pub fn cache_dir() -> Result<PathBuf, String> {
    cache_dir_from(|key| std::env::var_os(key))
}

#[cfg(windows)]
fn cache_dir_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = abs(getenv("FROSTMAIL_CACHE_DIR")) {
        return Ok(p);
    }
    abs(getenv("LOCALAPPDATA"))
        .map(|d| d.join("Frostmail").join("Cache"))
        .ok_or_else(|| "LOCALAPPDATA is not set".to_string())
}

/// The app's own configuration, never maild's: which maild it asks
/// (docs/design/desktop.md, On a maild behind lodge). $XDG_CONFIG_HOME/frostmail
/// or ~/.config/frostmail; in a Flatpak, XDG_CONFIG_HOME is the app's own.
#[cfg(not(windows))]
pub fn config_dir() -> Result<PathBuf, String> {
    config_dir_from(|key| std::env::var_os(key))
}

#[cfg(not(windows))]
fn config_dir_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = abs(getenv("XDG_CONFIG_HOME")) {
        return Ok(p.join("frostmail"));
    }
    abs(getenv("HOME"))
        .map(|h| h.join(".config").join("frostmail"))
        .ok_or_else(|| "HOME is not set".to_string())
}

/// The app's own configuration: %APPDATA%\Frostmail.
#[cfg(windows)]
pub fn config_dir() -> Result<PathBuf, String> {
    abs(std::env::var_os("APPDATA"))
        .map(|d| d.join("Frostmail"))
        .ok_or_else(|| "APPDATA is not set".to_string())
}

/// Where the app keeps attachments it downloads from a maild behind lodge,
/// apart from maild's own cache: $XDG_CACHE_HOME/frostmail-app/attachments,
/// the Flatpak's own cache there.
#[cfg(not(windows))]
pub fn downloads_dir() -> Result<PathBuf, String> {
    downloads_dir_from(|key| std::env::var_os(key))
}

#[cfg(not(windows))]
fn downloads_dir_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    let base = abs(getenv("XDG_CACHE_HOME"))
        .or_else(|| abs(getenv("HOME")).map(|h| h.join(".cache")))
        .ok_or_else(|| "HOME is not set".to_string())?;
    Ok(base.join("frostmail-app").join("attachments"))
}

/// Where the app keeps attachments it downloads from a maild behind lodge:
/// %LOCALAPPDATA%\Frostmail\Attachments.
#[cfg(windows)]
pub fn downloads_dir() -> Result<PathBuf, String> {
    abs(std::env::var_os("LOCALAPPDATA"))
        .map(|d| d.join("Frostmail").join("Attachments"))
        .ok_or_else(|| "LOCALAPPDATA is not set".to_string())
}

#[cfg(all(test, not(windows)))]
mod tests {
    use super::*;

    fn env<'a>(vars: &'a [(&'a str, &'a str)]) -> impl Fn(&str) -> Option<OsString> + 'a {
        move |key| vars.iter().find(|(k, _)| *k == key).map(|(_, v)| OsString::from(v))
    }

    #[test]
    fn socket_defaults_to_the_runtime_dir() {
        let vars = [("HOME", "/home/u"), ("XDG_RUNTIME_DIR", "/run/user/1000")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/run/user/1000/frostmail/maild.sock")));
    }

    #[test]
    fn socket_prefers_an_absolute_override() {
        let vars = [("FROSTMAIL_SOCKET", "/s/maild.sock"), ("XDG_RUNTIME_DIR", "/run/user/1000")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/s/maild.sock")));
        let vars = [("FROSTMAIL_SOCKET", "relative.sock"), ("XDG_RUNTIME_DIR", "/run/user/1000")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/run/user/1000/frostmail/maild.sock")));
    }

    #[test]
    fn socket_in_the_flatpak_is_in_the_shared_app_directory() {
        let vars = [("XDG_RUNTIME_DIR", "/run/user/1000"), ("FLATPAK_ID", "org.frostyard.Frostmail")];
        assert_eq!(
            socket_from(env(&vars)),
            Ok(PathBuf::from("/run/user/1000/app/org.frostyard.Frostmail/frostmail/maild.sock"))
        );
        // Another app's Flatpak, and no Flatpak, keep the plain path.
        let vars = [("XDG_RUNTIME_DIR", "/run/user/1000"), ("FLATPAK_ID", "org.example.Editor")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/run/user/1000/frostmail/maild.sock")));
        let vars = [("XDG_RUNTIME_DIR", "/run/user/1000"), ("FLATPAK_ID", "")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/run/user/1000/frostmail/maild.sock")));
        // FROSTMAIL_SOCKET still wins in the Flatpak (the development bundle).
        let vars = [
            ("FROSTMAIL_SOCKET", "/s/m.sock"),
            ("XDG_RUNTIME_DIR", "/run/user/1000"),
            ("FLATPAK_ID", "org.frostyard.Frostmail"),
        ];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/s/m.sock")));
    }

    #[test]
    fn socket_needs_an_absolute_runtime_dir() {
        assert!(socket_from(env(&[("HOME", "/home/u")])).is_err());
        assert!(socket_from(env(&[("XDG_RUNTIME_DIR", "run")])).is_err());
    }

    #[test]
    fn cache_dir_follows_internal_config() {
        let vars = [("HOME", "/home/u")];
        assert_eq!(cache_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.cache/frostmail")));
        let vars = [("HOME", "/home/u"), ("XDG_CACHE_HOME", "/c")];
        assert_eq!(cache_dir_from(env(&vars)), Ok(PathBuf::from("/c/frostmail")));
        let vars = [("HOME", "/home/u"), ("XDG_CACHE_HOME", "relative/ignored")];
        assert_eq!(cache_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.cache/frostmail")));
        let vars = [("HOME", "/home/u"), ("XDG_CACHE_HOME", "/c"), ("FROSTMAIL_CACHE_DIR", "/fc")];
        assert_eq!(cache_dir_from(env(&vars)), Ok(PathBuf::from("/fc")));
        assert!(cache_dir_from(env(&[])).is_err());
    }

    #[test]
    fn cache_dir_in_the_flatpak_is_the_apps_own() {
        // The window reads maild's cache from its own XDG_CACHE_HOME; the
        // host's (HOST_XDG_CACHE_HOME) is not used any more.
        let vars = [
            ("HOME", "/home/u"),
            ("FLATPAK_ID", "org.frostyard.Frostmail"),
            ("XDG_CACHE_HOME", "/home/u/.var/app/org.frostyard.Frostmail/cache"),
            ("HOST_XDG_CACHE_HOME", "/home/u/.cache"),
        ];
        assert_eq!(
            cache_dir_from(env(&vars)),
            Ok(PathBuf::from("/home/u/.var/app/org.frostyard.Frostmail/cache/frostmail"))
        );
        // The development bundle names the host maild's cache.
        let vars = [
            ("FLATPAK_ID", "org.frostyard.Frostmail"),
            ("XDG_CACHE_HOME", "/home/u/.var/app/org.frostyard.Frostmail/cache"),
            ("FROSTMAIL_CACHE_DIR", "/home/u/.cache/frostmail"),
        ];
        assert_eq!(cache_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.cache/frostmail")));
    }

    #[test]
    fn data_dir_follows_internal_config() {
        let vars = [("HOME", "/home/u")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.local/share/frostmail")));
        let vars = [("HOME", "/home/u"), ("XDG_DATA_HOME", "/d")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/d/frostmail")));
        let vars = [("HOME", "/home/u"), ("XDG_DATA_HOME", "relative/ignored")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.local/share/frostmail")));
        let vars = [("HOME", "/home/u"), ("XDG_DATA_HOME", "/d"), ("FROSTMAIL_DATA_DIR", "/fd")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/fd")));
        // A relative override names a directory in maild's own, so it is not
        // the app's answer (internal/config.Resolve would pass it on).
        let vars = [("HOME", "/home/u"), ("FROSTMAIL_DATA_DIR", "data")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.local/share/frostmail")));
        assert!(data_dir_from(env(&[])).is_err());
    }

    #[test]
    fn only_this_apps_flatpak_is_one() {
        for (id, want) in [("org.frostyard.Frostmail", true), ("org.example.Editor", false), ("", false)] {
            assert_eq!(in_flatpak_from(env(&[("FLATPAK_ID", id)])), want, "{id}");
        }
        assert!(!in_flatpak_from(env(&[])));
        // In the Flatpak the data directory is the app's own, from the
        // XDG base directory the sandbox sets.
        let vars = [
            ("FLATPAK_ID", "org.frostyard.Frostmail"),
            ("XDG_DATA_HOME", "/home/u/.var/app/org.frostyard.Frostmail/data"),
            ("HOME", "/home/u"),
        ];
        assert_eq!(
            data_dir_from(env(&vars)),
            Ok(PathBuf::from("/home/u/.var/app/org.frostyard.Frostmail/data/frostmail"))
        );
    }

    #[test]
    fn config_dir_follows_internal_config() {
        let vars = [("HOME", "/home/u"), ("XDG_CONFIG_HOME", "/cfg")];
        assert_eq!(config_dir_from(env(&vars)), Ok(PathBuf::from("/cfg/frostmail")));
        let vars = [("HOME", "/home/u")];
        assert_eq!(config_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.config/frostmail")));
        let vars = [("HOME", "/home/u"), ("XDG_CONFIG_HOME", "rel")];
        assert_eq!(config_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.config/frostmail")));
        assert!(config_dir_from(env(&[])).is_err());
    }

    #[test]
    fn downloads_dir_is_under_the_apps_cache() {
        let vars = [("HOME", "/home/u"), ("XDG_CACHE_HOME", "/c")];
        assert_eq!(downloads_dir_from(env(&vars)), Ok(PathBuf::from("/c/frostmail-app/attachments")));
        let vars = [("HOME", "/home/u")];
        assert_eq!(downloads_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.cache/frostmail-app/attachments")));
        assert!(downloads_dir_from(env(&[])).is_err());
    }
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
