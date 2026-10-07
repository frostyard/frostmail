//! The Frostmail app shell: one window, a line bridge to maild's socket and
//! the mailpart:// protocol. All mail logic lives in maild; see
//! docs/design/overview.md and docs/specs/rpc-protocol.md.

mod bridge;
mod commands;
mod paths;
mod protocol;

use tauri::{WebviewUrl, WebviewWindowBuilder};

fn main() {
    apply_webkit_workarounds();
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .manage(bridge::Bridge::default())
        .register_asynchronous_uri_scheme_protocol("mailpart", protocol::mailpart)
        .invoke_handler(tauri::generate_handler![
            bridge::maild_connect,
            bridge::maild_send,
            commands::open_link,
            commands::open_part
        ])
        .setup(|app| {
            WebviewWindowBuilder::new(app, "main", WebviewUrl::default())
                .title("Frostmail")
                .inner_size(1200.0, 780.0)
                .min_inner_size(800.0, 500.0)
                // The toolbar is the title bar (docs/specs/ui.md, Layout).
                .decorations(false)
                .on_navigation(|url| allowed_navigation(url))
                .build()?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("frostmail failed to start");
}

/// Only the app itself may load in the main frame; links in messages open in
/// the system browser through the open_link command instead.
fn allowed_navigation(url: &tauri::Url) -> bool {
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
