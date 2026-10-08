//! The Frostmail app shell: the main window and compose windows, a line
//! bridge per window to maild's socket, and the mailpart:// protocol. All mail logic lives in maild; see
//! docs/design/overview.md and docs/specs/rpc-protocol.md.

mod bridge;
mod commands;
mod paths;
mod protocol;

use tauri::{Manager, WebviewUrl, WebviewWindowBuilder};

fn main() {
    apply_webkit_workarounds();
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .manage(bridge::Bridge::default())
        .register_asynchronous_uri_scheme_protocol("mailpart", protocol::mailpart)
        .invoke_handler(tauri::generate_handler![
            bridge::maild_connect,
            bridge::maild_send,
            commands::open_link,
            commands::open_part,
            commands::open_compose,
            commands::open_settings
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                let app = window.app_handle().clone();
                let label = window.label().to_string();
                tauri::async_runtime::spawn(async move { app.state::<bridge::Bridge>().forget(&label).await });
            }
        })
        .setup(|app| {
            let main = WebviewWindowBuilder::new(app, "main", WebviewUrl::default())
                .title("Frostmail")
                .inner_size(1200.0, 780.0)
                .min_inner_size(800.0, 500.0)
                // The toolbar is the title bar (docs/specs/ui.md, Layout).
                .decorations(false)
                .on_navigation(|url| allowed_navigation(url))
                .visible(false)
                .build()?;
            show_soon(main);
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("frostmail failed to start");
}

/// Windows start hidden and the page shows its own window once it has
/// rendered, so the first frame is never WebKit's blank white page; if the
/// page has not done so after 1.5 s, the window is shown anyway.
pub(crate) fn show_soon(window: tauri::WebviewWindow) {
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(std::time::Duration::from_millis(1500)).await;
        if window.is_visible().is_ok_and(|v| !v) {
            let _ = window.show();
        }
    });
}

/// Only the app itself may load in a window's main frame; links in messages
/// open in the system browser through the open_link command instead.
pub(crate) fn allowed_navigation(url: &tauri::Url) -> bool {
    match url.scheme() {
        "tauri" | "about" => true,
        "http" => cfg!(debug_assertions) && url.host_str() == Some("127.0.0.1") && url.port() == Some(5173),
        _ => false,
    }
}

/// WebKitGTK 2.52+ has GPU rendering bugs on some AMD (radv) and NVIDIA
/// setups (docs/adr/0006-development-environment.md). FROSTMAIL_WEBKIT_SAFE=1
/// selects the CPU paths unless the variables are already set.
fn apply_webkit_workarounds() {
    if std::env::var_os("FROSTMAIL_WEBKIT_SAFE").is_none_or(|v| v != "1") {
        return;
    }
    for key in ["WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_SKIA_ENABLE_CPU_RENDERING"] {
        if std::env::var_os(key).is_none() {
            // SAFETY: called first in main, before any other thread exists.
            unsafe { std::env::set_var(key, "1") };
        }
    }
}
