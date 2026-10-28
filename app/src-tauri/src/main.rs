//! The Frostmail app shell: one instance with the main, compose, settings
//! and reminder windows, a line bridge per window to maild's socket, and the
//! mailpart:// protocol. All mail logic lives in maild; see
//! docs/design/overview.md and docs/specs/rpc-protocol.md.

// A release build is a GUI program on Windows: no console window opens
// with it.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod announce;
mod bridge;
mod commands;
mod mode;
mod paths;
mod portal;
mod protocol;
mod remote;

use tauri::{Manager, WebviewUrl, WebviewWindowBuilder};

fn main() {
    apply_webkit_workarounds();
    let args: Vec<String> = std::env::args().collect();
    tauri::Builder::default()
        // First, so a second launch hands over before anything else starts.
        .plugin(tauri_plugin_single_instance::init(|app, args, _cwd| commands::second_launch(app, &args)))
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .manage(bridge::Bridge::default())
        .manage(announce::Notifier::default())
        .manage(commands::Launch(std::sync::Mutex::new(commands::open_request(&args))))
        .register_asynchronous_uri_scheme_protocol("mailpart", protocol::mailpart)
        .invoke_handler(tauri::generate_handler![
            bridge::maild_connect,
            bridge::maild_send,
            commands::open_link,
            commands::open_part,
            commands::save_content,
            commands::in_flatpak,
            commands::portal_save_file,
            announce::notify_mail,
            commands::open_compose,
            commands::open_event,
            commands::open_settings,
            commands::open_reminders,
            commands::startup_message,
            commands::app_config_get,
            commands::app_config_set,
            commands::server_facts,
            commands::host_mail_found
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                let app = window.app_handle().clone();
                let label = window.label().to_string();
                tauri::async_runtime::spawn(async move { app.state::<bridge::Bridge>().forget(&label).await });
            }
        })
        .setup(|app| {
            let links = app.handle().clone();
            let main = window(app, "main")
                .title("Frostmail")
                .inner_size(1200.0, 780.0)
                .min_inner_size(800.0, 500.0)
                // The toolbar is the title bar (docs/specs/ui.md, Layout).
                .decorations(false)
                .on_navigation(|url| allowed_navigation(url))
                // Links clicked in the reader's message frame.
                .on_new_window(move |url, _| commands::open_new_window(&links, &url))
                .visible(false)
                .build()?;
            show_soon(main);
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("frostmail failed to start");
}

/// A builder for one of the app's windows, all of which load the app. On
/// Windows they share one WebView2 environment, which takes only windows
/// with the same browser arguments: wry's defaults, and the end-to-end
/// tests' DevTools port when FROSTMAIL_DEVTOOLS_PORT names one (as
/// TAURI_WEBVIEW_AUTOMATION allows automation on Linux). WebView2 lets an
/// app's arguments override WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS, which
/// wry always sets. The commands that open windows are async: on Windows a
/// window built in a synchronous command deadlocks the app (WebView2).
pub(crate) fn window<'a, R: tauri::Runtime, M: Manager<R>>(manager: &'a M, label: &str) -> WebviewWindowBuilder<'a, R, M> {
    let builder = WebviewWindowBuilder::new(manager, label, WebviewUrl::default());
    #[cfg(windows)]
    if let Some(args) = browser_args(std::env::var("FROSTMAIL_DEVTOOLS_PORT").ok().as_deref()) {
        return builder.additional_browser_args(&args);
    }
    builder
}

/// wry's default WebView2 arguments with a DevTools port, if port is one.
#[cfg(any(windows, test))]
fn browser_args(port: Option<&str>) -> Option<String> {
    let port: u16 = port?.parse().ok().filter(|&p| p != 0)?;
    Some(format!(
        "--disable-features=msWebOOUI,msPdfOOUI,msSmartScreenProtection --autoplay-policy=no-user-gesture-required --remote-debugging-port={port}"
    ))
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
/// open in the system browser instead (commands::open_new_window). WebKitGTK
/// serves the app at tauri://localhost, WebView2 at http://tauri.localhost
/// (a name that reaches the loopback interface on Linux).
pub(crate) fn allowed_navigation(url: &tauri::Url) -> bool {
    let at = |host: &str, port: Option<u16>| url.host_str() == Some(host) && url.port() == port;
    match url.scheme() {
        "tauri" | "about" => true,
        "http" => (cfg!(windows) && at("tauri.localhost", None)) || (cfg!(debug_assertions) && at("127.0.0.1", Some(5173))),
        _ => false,
    }
}

/// WebKitGTK 2.52+ has GPU rendering bugs on some AMD (radv) and NVIDIA
/// setups (docs/adr/0006-development-environment.md). FROSTMAIL_WEBKIT_SAFE=1
/// selects the CPU paths unless the variables are already set. Linux only:
/// Windows has WebView2.
#[cfg(target_os = "linux")]
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

#[cfg(not(target_os = "linux"))]
fn apply_webkit_workarounds() {}

#[cfg(test)]
mod tests {
    use super::{allowed_navigation, browser_args};

    #[test]
    fn a_devtools_port_only_when_one_is_named() {
        assert_eq!(browser_args(None), None);
        for bad in ["", "x", "0", "65536", "-1", "9222 --foo"] {
            assert_eq!(browser_args(Some(bad)), None, "{bad}");
        }
        let args = browser_args(Some("9222")).unwrap();
        assert!(args.starts_with("--disable-features=msWebOOUI,msPdfOOUI,msSmartScreenProtection "), "{args}");
        assert!(args.ends_with(" --remote-debugging-port=9222"), "{args}");
    }

    fn allowed(url: &str) -> bool {
        allowed_navigation(&tauri::Url::parse(url).unwrap())
    }

    #[test]
    fn only_the_app_loads() {
        assert!(allowed("tauri://localhost/"));
        assert!(allowed("about:blank"));
        assert_eq!(allowed("http://tauri.localhost/index.html"), cfg!(windows));
        for url in [
            "http://tauri.localhost:8080/",
            "http://evil.localhost/",
            "https://tauri.localhost/",
            "http://example.test/",
            "mailpart://localhost/m/1/2.png",
            "http://mailpart.localhost/m/1/2.png",
            "file:///C:/Windows/win.ini",
        ] {
            assert!(!allowed(url), "{url}");
        }
    }
}
