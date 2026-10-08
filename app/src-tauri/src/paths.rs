//! File locations, mirroring internal/config/paths.go: the overrides, then
//! the XDG base directories.

use std::path::PathBuf;

fn env_abs(key: &str) -> Option<PathBuf> {
    std::env::var_os(key).map(PathBuf::from).filter(|p| p.is_absolute())
}

/// maild's socket: $FROSTMAIL_SOCKET or $XDG_RUNTIME_DIR/frostmail/maild.sock.
pub fn socket() -> Result<PathBuf, String> {
    if let Some(p) = env_abs("FROSTMAIL_SOCKET") {
        return Ok(p);
    }
    env_abs("XDG_RUNTIME_DIR")
        .map(|d| d.join("frostmail").join("maild.sock"))
        .ok_or_else(|| "XDG_RUNTIME_DIR is not set; set FROSTMAIL_SOCKET".to_string())
}

/// The cache maild writes decoded parts into:
/// $FROSTMAIL_CACHE_DIR or $XDG_CACHE_HOME/frostmail or ~/.cache/frostmail.
/// In a Flatpak, XDG_CACHE_HOME is the app's own, so the host's
/// ($HOST_XDG_CACHE_HOME, else ~/.cache) is used instead (ADR-0013).
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
