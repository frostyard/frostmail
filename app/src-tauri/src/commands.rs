//! Commands the webview calls: links open in the system browser and
//! attachments in their default application (docs/design/app.md, Reader),
//! and drafts open in compose windows (docs/design/send.md).

use tauri::{AppHandle, Manager, WebviewUrl, WebviewWindowBuilder};
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

/// Open a draft in its compose window, labeled compose-<id>, or focus the
/// window that already shows it. The window learns its draft, and whether
/// it was just created (fresh: discarded when closed untouched), from an
/// initialization script, so the app's URL stays the same.
#[tauri::command]
pub fn open_compose(app: AppHandle, draft_id: i64, fresh: bool) -> Result<(), String> {
    let label = format!("compose-{draft_id}");
    if let Some(w) = app.get_webview_window(&label) {
        let _ = w.unminimize();
        return w.set_focus().map_err(|e| e.to_string());
    }
    WebviewWindowBuilder::new(&app, &label, WebviewUrl::default())
        .title("New Message")
        .inner_size(720.0, 560.0)
        .min_inner_size(480.0, 360.0)
        // The compose toolbar is the title bar (docs/specs/compose-ui.md).
        .decorations(false)
        .initialization_script(format!(
            "window.__frostmailCompose = {draft_id}; window.__frostmailComposeFresh = {fresh};"
        ))
        .on_navigation(crate::allowed_navigation)
        .visible(false)
        .build()
        .map(crate::show_soon)
        .map_err(|e| e.to_string())
}
