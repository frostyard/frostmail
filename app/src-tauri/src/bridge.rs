//! A line bridge between the webview and maild's socket: a Unix socket, or
//! on Windows a named pipe (ADR-0038). It never parses frames: JSON-RPC
//! multiplexing happens in the webview (src/rpc/transport.ts), so the
//! bridge cannot drift from the API.

use std::collections::HashMap;
use std::path::Path;

use serde::Serialize;
use tauri::{State, Window, ipc::Channel};
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::sync::Mutex;

use crate::paths;

/// A connection's write half.
type Writer = Box<dyn AsyncWrite + Send + Unpin>;

/// What the webview receives on its channel, in order.
#[derive(Clone, Serialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum BridgeMessage {
    /// One newline-delimited JSON-RPC message from maild.
    Line { line: String },
    /// The connection ended; no more lines follow.
    Closed { error: String },
}

/// Each window's connection write half, by window label: every window has
/// its own connection (docs/design/send.md, Compose windows). Connecting
/// again replaces the window's connection, which shuts the old one down;
/// closing the window drops it.
#[derive(Default)]
pub struct Bridge {
    writers: Mutex<HashMap<String, Writer>>,
}

impl Bridge {
    /// Drop a closed window's connection.
    pub async fn forget(&self, label: &str) {
        self.writers.lock().await.remove(label);
    }
}

/// Connect the calling window to maild and stream every received line to
/// `on_message`.
#[tauri::command]
pub async fn maild_connect(
    window: Window,
    bridge: State<'_, Bridge>,
    on_message: Channel<BridgeMessage>,
) -> Result<(), String> {
    let path = paths::socket()?;
    let (read, write) = connect(&path)
        .await
        .map_err(|e| format!("connect to maild at {}: {e}", path.display()))?;
    bridge.writers.lock().await.insert(window.label().to_string(), write);
    tauri::async_runtime::spawn(async move {
        let mut lines = BufReader::new(read).lines();
        let error = loop {
            match lines.next_line().await {
                Ok(Some(line)) => {
                    if on_message.send(BridgeMessage::Line { line }).is_err() {
                        break "the webview stopped listening".to_string();
                    }
                }
                Ok(None) => break "maild closed the connection".to_string(),
                Err(e) => break format!("read from maild: {e}"),
            }
        };
        let _ = on_message.send(BridgeMessage::Closed { error });
    });
    Ok(())
}

/// Send one JSON-RPC message on the calling window's connection; it must not
/// contain a newline.
#[tauri::command]
pub async fn maild_send(window: Window, bridge: State<'_, Bridge>, line: String) -> Result<(), String> {
    if line.contains('\n') {
        return Err("a frame must be a single line".into());
    }
    let mut guard = bridge.writers.lock().await;
    let writer = guard.get_mut(window.label()).ok_or("not connected to maild")?;
    let mut frame = line.into_bytes();
    frame.push(b'\n');
    writer.write_all(&frame).await.map_err(|e| format!("write to maild: {e}"))
}

/// Connect to maild's Unix socket; systemd starts maild on Linux.
#[cfg(not(windows))]
async fn connect(path: &Path) -> std::io::Result<(Box<dyn AsyncRead + Send + Unpin>, Writer)> {
    let (read, write) = tokio::net::UnixStream::connect(path).await?.into_split();
    Ok((Box::new(read), Box::new(write)))
}

/// Connect to maild's named pipe, at the identification level (tokio's
/// default), which maild's check of the client's SID needs. With no maild
/// yet (the app started before the user's sign-in started it, or it
/// stopped), the app starts the maild.exe beside it and waits for its pipe
/// (docs/design/desktop.md, Windows).
#[cfg(windows)]
async fn connect(path: &Path) -> std::io::Result<(Box<dyn AsyncRead + Send + Unpin>, Writer)> {
    use std::time::{Duration, Instant};

    let mut started = false;
    let deadline = Instant::now() + Duration::from_secs(10);
    loop {
        match open_pipe(path).await {
            Ok(pipe) => {
                let (read, write) = tokio::io::split(pipe);
                return Ok((Box::new(read), Box::new(write)));
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound && Instant::now() < deadline => {
                if !started {
                    start_maild()?;
                    started = true;
                }
                tokio::time::sleep(Duration::from_millis(100)).await;
            }
            Err(e) => return Err(e),
        }
    }
}

/// Open the pipe, waiting while all its instances are busy (maild makes a
/// new one as it accepts each).
#[cfg(windows)]
async fn open_pipe(path: &Path) -> std::io::Result<tokio::net::windows::named_pipe::NamedPipeClient> {
    const ERROR_PIPE_BUSY: i32 = 231;
    let mut tries = 0;
    loop {
        match tokio::net::windows::named_pipe::ClientOptions::new().open(path) {
            Err(e) if e.raw_os_error() == Some(ERROR_PIPE_BUSY) && tries < 50 => {
                tries += 1;
                tokio::time::sleep(std::time::Duration::from_millis(20)).await;
            }
            r => return r,
        }
    }
}

/// Start the maild.exe installed beside the app, without a console and
/// outliving the app. A second maild started meanwhile finds the pipe
/// taken and exits.
#[cfg(windows)]
fn start_maild() -> std::io::Result<()> {
    use std::os::windows::process::CommandExt;
    const DETACHED_PROCESS: u32 = 0x0000_0008;
    const CREATE_NO_WINDOW: u32 = 0x0800_0000;
    let maild = std::env::current_exe()?.with_file_name("maild.exe");
    std::process::Command::new(&maild)
        .creation_flags(DETACHED_PROCESS | CREATE_NO_WINDOW)
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .spawn()
        .map_err(|e| std::io::Error::new(e.kind(), format!("start {}: {e}", maild.display())))?;
    Ok(())
}
