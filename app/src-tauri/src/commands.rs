//! Commands the reader calls to leave the app: links open in the system
//! browser and attachments in their default application
//! (docs/design/app.md, Reader).

use tauri::AppHandle;
use tauri_plugin_opener::OpenerExt;

use crate::protocol;

/// File types that run code when opened; Frostmail never opens them.
const UNSAFE_EXTENSIONS: &[&str] = &[
    "desktop", "sh", "bash", "zsh", "fish", "csh", "run", "bin", "appimage", "flatpakref", "flatpakrepo",
    "deb", "rpm", "exe", "msi", "bat", "cmd", "com", "scr", "ps1", "vbs", "jar", "py", "pl", "rb", "js",
    "so", "elf", "x86_64", "apk", "dmg", "pkg",
];

/// Open an http, https or mailto URL in the system's default handler.
#[tauri::command]
pub fn open_link(app: AppHandle, url: String) -> Result<(), String> {
    let parsed = tauri::Url::parse(&url).map_err(|e| format!("not a URL: {e}"))?;
    if !matches!(parsed.scheme(), "http" | "https" | "mailto") {
        return Err(format!("{} links do not open", parsed.scheme()));
    }
    app.opener().open_url(parsed.as_str(), None::<&str>).map_err(|e| e.to_string())
}

/// Open a part maild decoded into the parts cache (message.part) with the
/// system's default application. Paths outside the cache, and file types
/// that run code, are refused.
#[tauri::command]
pub async fn open_part(app: AppHandle, path: String) -> Result<(), String> {
    let full = protocol::resolve(&path, false).await.map_err(|status| format!("cannot open {path}: {status}"))?;
    let ext = full.extension().and_then(|e| e.to_str()).unwrap_or("").to_ascii_lowercase();
    if UNSAFE_EXTENSIONS.contains(&ext.as_str()) {
        return Err(format!("Frostmail does not open .{ext} files"));
    }
    app.opener().open_path(full.to_string_lossy(), None::<&str>).map_err(|e| e.to_string())
}
