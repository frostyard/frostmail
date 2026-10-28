//! Commands the webview calls: links open in the system browser and
//! attachments in their default application (docs/design/app.md, Reader),
//! drafts open in compose windows (docs/design/send.md), and the settings
//! window opens (docs/specs/settings-ui.md).

use std::sync::Mutex;

use tauri::webview::NewWindowResponse;
use tauri::{AppHandle, Manager, State};
use tauri_plugin_opener::OpenerExt;

use crate::{bridge, paths, portal, protocol, remote};

/// File types that run code when opened, on Linux or Windows; Frostmail
/// never opens them. Windows' list follows the types its shell runs or
/// hands to a script host: links and shortcuts, installers, scripts,
/// Control Panel and console snap-ins, HTML applications and help.
const UNSAFE_EXTENSIONS: &[&str] = &[
    "desktop", "sh", "bash", "zsh", "fish", "csh", "run", "bin", "appimage", "flatpakref", "flatpakrepo",
    "deb", "rpm", "exe", "msi", "bat", "cmd", "com", "scr", "ps1", "vbs", "jar", "py", "pl", "rb", "js",
    "so", "elf", "x86_64", "apk", "dmg", "pkg", //
    "lnk", "url", "pif", "scf", "appref-ms", "application", "settingcontent-ms", "library-ms", "msix",
    "msixbundle", "appx", "appxbundle", "msp", "mst", "reg", "inf", "cpl", "msc", "hta", "chm", "hlp",
    "psm1", "psd1", "ps1xml", "vbe", "jse", "wsf", "wsh", "ws", "wsc", "sct", "gadget", "xll", "jnlp",
    "dll", "sys", "drv", "ocx",
];

/// Open an http, https or mailto URL in the system's default handler.
#[tauri::command]
pub fn open_link(app: AppHandle, url: String) -> Result<(), String> {
    let parsed = tauri::Url::parse(&url).map_err(|e| format!("not a URL: {e}"))?;
    open_url(&app, &parsed)
}

/// Open a link with the system's default handler: in the Flatpak, xdg-open
/// asks the OpenURI portal. Only http, https and mailto URLs open.
pub(crate) fn open_url(app: &AppHandle, url: &tauri::Url) -> Result<(), String> {
    openable(url)?;
    if logged(url.as_str())? {
        return Ok(());
    }
    app.opener().open_url(url.as_str(), None::<&str>).map_err(|e| e.to_string())
}

/// With FROSTMAIL_OPEN_LOG set, what the app would open is appended to
/// that file instead, a line each: the end-to-end tests' stand-in on
/// Windows, whose handlers are the shell's rather than programs on PATH
/// (docs/design/testing.md).
fn logged(target: &str) -> Result<bool, String> {
    use std::io::Write;
    let Some(log) = std::env::var_os("FROSTMAIL_OPEN_LOG").filter(|l| !l.is_empty()) else {
        return Ok(false);
    };
    std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(&log)
        .and_then(|mut f| writeln!(f, "{target}"))
        .map_err(|e| format!("FROSTMAIL_OPEN_LOG: {e}"))?;
    Ok(true)
}

fn openable(url: &tauri::Url) -> Result<(), String> {
    match url.scheme() {
        "http" | "https" | "mailto" => Ok(()),
        scheme => Err(format!("{scheme} links do not open")),
    }
}

/// A link clicked in the reader's message frame arrives as a request for a
/// new window: the frame's links target _blank, and WebKit runs none of the
/// page's listeners inside a frame sandboxed without scripts (ADR-0036). The
/// link opens in the system browser; no window is ever created.
pub(crate) fn open_new_window(app: &AppHandle, url: &tauri::Url) -> NewWindowResponse<tauri::Wry> {
    if let Err(e) = open_url(app, url) {
        eprintln!("open link {url}: {e}");
    }
    NewWindowResponse::Deny
}

/// Open a part maild decoded into the parts cache (message.part) with the
/// system's default application. For a maild behind lodge, whose cache is
/// on lodge's machine, the part is first downloaded from lodge's content
/// node by its contentPath (docs/design/desktop.md, On a maild behind
/// lodge). Paths outside the cache, and file types that run code, are
/// refused.
#[tauri::command]
pub async fn open_part(
    app: AppHandle,
    path: String,
    content_path: Option<String>,
    filename: Option<String>,
) -> Result<(), String> {
    let full = match remote::load()?.lodge {
        Some(url) => {
            let lodge = remote::Lodge::parse(&url)?;
            let content_path = content_path.ok_or("maild did not name the part on lodge's content node")?;
            let name = filename.unwrap_or_else(|| path.rsplit('/').next().unwrap_or("attachment").to_string());
            refuse_unsafe(&name)?;
            tauri::async_runtime::spawn_blocking(move || remote::download(&lodge, &content_path, &name))
                .await
                .map_err(|e| e.to_string())??
        }
        None => protocol::resolve(&path, false).await.map_err(|status| format!("cannot open {path}: {status}"))?,
    };
    refuse_unsafe(&full.to_string_lossy())?;
    if logged(&full.to_string_lossy())? {
        return Ok(());
    }
    app.opener().open_path(full.to_string_lossy(), None::<&str>).map_err(|e| e.to_string())
}

/// Save As on a maild behind lodge (docs/design/desktop.md): download the
/// message maild named on lodge's content node into the file the user
/// chose. The maild on this machine writes it there itself (message.save).
#[tauri::command]
pub async fn save_content(content_path: String, path: String) -> Result<(), String> {
    let url = remote::load()?.lodge.ok_or("Save As downloads only from a maild behind lodge")?;
    let lodge = remote::Lodge::parse(&url)?;
    let path = std::path::PathBuf::from(path);
    tauri::async_runtime::spawn_blocking(move || remote::save_to(&lodge, &content_path, &path))
        .await
        .map_err(|e| e.to_string())?
}

/// Whether the app runs in its Flatpak (docs/adr/0046-the-desktop-flatpak-
/// carries-its-own-maild.md, Files in the sandbox): the webview then asks
/// the file chooser portal for Save As, and attaches by bytes.
#[tauri::command]
pub fn in_flatpak() -> bool {
    paths::in_flatpak()
}

/// Save As in the Flatpak: ask the file chooser portal where the file goes,
/// and answer the path, or None when the user cancels (src/portal.rs). The
/// portal opens a window and waits on the user, so this runs on a blocking
/// thread. Outside the Flatpak, and on Windows, Save As keeps Tauri's
/// dialog, and this refuses.
#[tauri::command]
pub async fn portal_save_file(
    title: String,
    current_name: String,
    filter_name: String,
    extension: String,
) -> Result<Option<String>, String> {
    if !paths::in_flatpak() {
        return Err("Save As through the file chooser portal is only in the Flatpak".to_string());
    }
    let path = tauri::async_runtime::spawn_blocking(move || {
        portal::save_file(&title, &current_name, &filter_name, &extension)
    })
    .await
    .map_err(|e| e.to_string())??;
    Ok(path.map(|p| p.to_string_lossy().into_owned()))
}

/// Refuse a file whose type runs code when opened.
fn refuse_unsafe(name: &str) -> Result<(), String> {
    let ext = std::path::Path::new(name).extension().and_then(|e| e.to_str()).unwrap_or("").to_ascii_lowercase();
    if UNSAFE_EXTENSIONS.contains(&ext.as_str()) {
        return Err(format!("Frostmail does not open .{ext} files"));
    }
    Ok(())
}

/// The app's choice of maild (docs/design/desktop.md, On a maild behind
/// lodge): lodge's address, or none for the maild on this machine.
#[tauri::command]
pub fn app_config_get() -> Result<remote::AppConfig, String> {
    remote::load()
}

/// Change the app's choice of maild, then end every window's connection, so
/// each reconnects to the maild now chosen.
#[tauri::command]
pub async fn app_config_set(bridge: State<'_, bridge::Bridge>, config: remote::AppConfig) -> Result<(), String> {
    let config = remote::AppConfig { lodge: config.lodge.map(|l| l.trim().to_string()).filter(|l| !l.is_empty()) };
    remote::save(&config)?;
    bridge.close_all().await;
    Ok(())
}

/// Open a draft in its compose window, labeled compose-<id>, or focus the
/// window that already shows it. The window learns its draft, and whether
/// it was just created (fresh: discarded when closed untouched), from an
/// initialization script, so the app's URL stays the same.
#[tauri::command]
pub async fn open_compose(app: AppHandle, draft_id: i64, fresh: bool) -> Result<(), String> {
    let label = format!("compose-{draft_id}");
    if let Some(w) = app.get_webview_window(&label) {
        let _ = w.unminimize();
        return w.set_focus().map_err(|e| e.to_string());
    }
    let mut window = crate::window(&app, &label)
        .title("New Message")
        .inner_size(720.0, 560.0)
        .min_inner_size(480.0, 360.0)
        // The compose toolbar is the title bar (docs/specs/compose-ui.md).
        .decorations(false)
        .initialization_script(format!(
            "window.__frostmailCompose = {draft_id}; window.__frostmailComposeFresh = {fresh};"
        ))
        .on_navigation(crate::allowed_navigation)
        .visible(false);
    // Files dropped on the window reach the webview as files, whose bytes it
    // sends (draft.attachData), as in a browser: a maild behind lodge cannot
    // read this machine's paths, and the Flatpak's maild cannot read the
    // paths the window sees outside its sandbox.
    if remote::load().is_ok_and(|c| c.lodge.is_some()) || paths::in_flatpak() {
        window = window.disable_drag_drop_handler();
    }
    window.build().map(crate::show_soon).map_err(|e| e.to_string())
}

/// Open an event in its editor window (docs/specs/pim-ui.md, Event editor
/// window), labeled event-<id>, event-<id>-<occurrence> or event-new-<n>,
/// or focus the window that already shows it. The window learns what it
/// edits from an initialization script, so the app's URL stays the same.
#[tauri::command]
pub async fn open_event(app: AppHandle, label: String, target: serde_json::Value, title: String) -> Result<(), String> {
    let valid = label
        .strip_prefix("event-")
        .is_some_and(|rest| !rest.is_empty() && rest.chars().all(|c| c.is_ascii_alphanumeric() || c == '-'));
    if !valid {
        return Err(format!("{label} is not an event window"));
    }
    if let Some(w) = app.get_webview_window(&label) {
        let _ = w.unminimize();
        let _ = w.show();
        return w.set_focus().map_err(|e| e.to_string());
    }
    let target = serde_json::to_string(&target).map_err(|e| e.to_string())?;
    crate::window(&app, &label)
        .title(title)
        .inner_size(480.0, 600.0)
        .min_inner_size(420.0, 480.0)
        // The editor draws its own title strip.
        .decorations(false)
        .initialization_script(format!("window.__frostmailEvent = {target};"))
        .on_navigation(crate::allowed_navigation)
        .visible(false)
        .build()
        .map(crate::show_soon)
        .map_err(|e| e.to_string())
}

/// Open the settings window, labeled settings, or focus it when it is open.
/// It learns that it is the settings window from an initialization script.
#[tauri::command]
pub async fn open_settings(app: AppHandle) -> Result<(), String> {
    if let Some(w) = app.get_webview_window("settings") {
        let _ = w.unminimize();
        let _ = w.show();
        return w.set_focus().map_err(|e| e.to_string());
    }
    crate::window(&app, "settings")
        .title("Settings")
        .inner_size(760.0, 560.0)
        .min_inner_size(640.0, 440.0)
        // The settings toolbar is the title bar (docs/specs/settings-ui.md).
        .decorations(false)
        .initialization_script("window.__frostmailSettings = true;")
        .on_navigation(crate::allowed_navigation)
        .visible(false)
        .build()
        .map(crate::show_soon)
        .map_err(|e| e.to_string())
}

/// Open the reminder window, labeled reminders, or raise it when it is open
/// (docs/specs/pim-ui.md, Reminder window). It floats over the other
/// windows and learns its role from an initialization script.
#[tauri::command]
pub async fn open_reminders(app: AppHandle) -> Result<(), String> {
    if let Some(w) = app.get_webview_window("reminders") {
        let _ = w.unminimize();
        let _ = w.show();
        return w.set_focus().map_err(|e| e.to_string());
    }
    crate::window(&app, "reminders")
        .title("Reminders")
        .inner_size(380.0, 300.0)
        .min_inner_size(320.0, 160.0)
        .always_on_top(true)
        // The reminder window draws its own title strip.
        .decorations(false)
        .initialization_script("window.__frostmailReminders = true;")
        .on_navigation(crate::allowed_navigation)
        .visible(false)
        .build()
        .map(crate::show_soon)
        .map_err(|e| e.to_string())
}

/// The message a launch asked to open with --open-message <id>.
pub fn open_request(args: &[String]) -> Option<i64> {
    let i = args.iter().position(|a| a == "--open-message")?;
    args.get(i + 1)?.parse().ok()
}

/// The first launch's --open-message, until the main window takes it.
#[derive(Default)]
pub struct Launch(pub Mutex<Option<i64>>);

/// Return the message the app was started to open, once.
#[tauri::command]
pub fn startup_message(launch: State<'_, Launch>) -> Option<i64> {
    launch.0.lock().ok()?.take()
}

/// A second launch (single instance): show the main window and pass on
/// its --open-message as the open-message event (docs/design/desktop.md).
pub fn second_launch(app: &AppHandle, args: &[String]) {
    crate::announce::open(app, open_request(args).unwrap_or(0));
}

#[cfg(test)]
mod tests {
    use super::{open_request, openable};

    fn args(list: &[&str]) -> Vec<String> {
        list.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn opens_only_web_and_mail_links() {
        for url in ["https://example.test/a?b=1", "http://example.test", "mailto:a@example.test"] {
            assert!(openable(&tauri::Url::parse(url).unwrap()).is_ok(), "{url}");
        }
        for url in [
            "javascript:alert(1)",
            "file:///etc/passwd",
            "data:text/html,x",
            "mailpart://localhost/m/1/2.png",
            "tauri://localhost/",
        ] {
            assert!(openable(&tauri::Url::parse(url).unwrap()).is_err(), "{url}");
        }
    }

    #[test]
    fn reads_open_message() {
        assert_eq!(open_request(&args(&["frostmail", "--open-message", "42"])), Some(42));
        assert_eq!(open_request(&args(&["frostmail"])), None);
        assert_eq!(open_request(&args(&["frostmail", "--open-message"])), None);
        assert_eq!(open_request(&args(&["frostmail", "--open-message", "x"])), None);
    }
}
