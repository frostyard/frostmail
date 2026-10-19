//! A line bridge between the webview and maild: maild's socket, a Unix
//! socket or on Windows a named pipe (ADR-0038), or lodge's /app/mail/api
//! WebSocket, one line per text frame, for a maild behind lodge
//! (docs/design/desktop.md, On a maild behind lodge). It never parses
//! frames: JSON-RPC multiplexing happens in the webview
//! (src/rpc/transport.ts), so the bridge cannot drift from the API.

use std::collections::HashMap;
use std::path::Path;
use std::sync::Arc;

use futures_util::stream::{SplitSink, SplitStream};
use futures_util::{SinkExt, StreamExt};
use serde::Serialize;
use tauri::{State, Window, ipc::Channel};
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::net::TcpStream;
use tokio::sync::{Mutex, Notify};
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::{MaybeTlsStream, WebSocketStream};

use crate::{paths, remote};

type Socket = WebSocketStream<MaybeTlsStream<TcpStream>>;

/// A connection's write half: maild's socket, or lodge's WebSocket.
enum Writer {
    Stream(Box<dyn AsyncWrite + Send + Unpin>),
    Lodge(SplitSink<Socket, Message>),
}

/// A window's connection: its write half, and the signal that ends its
/// read loop when the connection is replaced or the app switches maild.
struct Connection {
    writer: Writer,
    stop: Arc<Notify>,
}

impl Drop for Connection {
    fn drop(&mut self) {
        self.stop.notify_one();
    }
}

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
    connections: Mutex<HashMap<String, Connection>>,
}

impl Bridge {
    /// Drop a closed window's connection.
    pub async fn forget(&self, label: &str) {
        self.connections.lock().await.remove(label);
    }

    /// End every window's connection, which each window's session then
    /// reconnects: the app switched maild.
    pub async fn close_all(&self) {
        self.connections.lock().await.clear();
    }

    async fn keep(&self, label: &str, writer: Writer) -> Arc<Notify> {
        let stop = Arc::new(Notify::new());
        self.connections.lock().await.insert(label.to_string(), Connection { writer, stop: stop.clone() });
        stop
    }
}

/// What maild_connect tells the webview about the connection.
#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Connected {
    /// maild is behind lodge, which closes a connection that sends nothing
    /// for 90 s, so the webview keeps it alive.
    pub remote: bool,
}

/// Connect the calling window to maild, its local socket or lodge's
/// /app/mail/api as app.json says, and stream every received line to
/// `on_message`.
#[tauri::command]
pub async fn maild_connect(
    window: Window,
    bridge: State<'_, Bridge>,
    on_message: Channel<BridgeMessage>,
) -> Result<Connected, String> {
    if let Some(url) = remote::load()?.lodge {
        let lodge = remote::Lodge::parse(&url)?;
        let (write, read) = connect_lodge(&lodge).await?.split();
        let stop = bridge.keep(window.label(), Writer::Lodge(write)).await;
        tauri::async_runtime::spawn(read_frames(read, stop, on_message));
        return Ok(Connected { remote: true });
    }
    let path = paths::socket()?;
    let (read, write) = connect(&path)
        .await
        .map_err(|e| format!("connect to maild at {}: {e}", path.display()))?;
    let stop = bridge.keep(window.label(), Writer::Stream(write)).await;
    tauri::async_runtime::spawn(async move {
        let mut lines = BufReader::new(read).lines();
        let error = loop {
            tokio::select! {
                next = lines.next_line() => match next {
                    Ok(Some(line)) => {
                        if on_message.send(BridgeMessage::Line { line }).is_err() {
                            break "the webview stopped listening".to_string();
                        }
                    }
                    Ok(None) => break "maild closed the connection".to_string(),
                    Err(e) => break format!("read from maild: {e}"),
                },
                () = stop.notified() => break "the connection was replaced".to_string(),
            }
        };
        let _ = on_message.send(BridgeMessage::Closed { error });
    });
    Ok(Connected { remote: false })
}

/// Open lodge's /app/mail/api from this machine's tailnet node, with
/// lodge's own Origin and no fetch metadata, as lodge-http's native
/// clients do (D7): lodge knows the node by WhoIs.
async fn connect_lodge(lodge: &remote::Lodge) -> Result<Socket, String> {
    let mut req = lodge.api_url().into_client_request().map_err(|e| e.to_string())?;
    let origin = lodge.origin().parse().map_err(|_| format!("lodge's origin {} is not a header", lodge.origin()))?;
    req.headers_mut().insert("Origin", origin);
    let (ws, _) = tokio_tungstenite::connect_async(req)
        .await
        .map_err(|e| format!("connect to maild through lodge at {}: {e}", lodge.origin()))?;
    Ok(ws)
}

/// Stream each text frame from lodge to the webview as a line.
async fn read_frames(mut read: SplitStream<Socket>, stop: Arc<Notify>, on_message: Channel<BridgeMessage>) {
    let error = loop {
        tokio::select! {
            next = read.next() => match next {
                Some(Ok(Message::Text(text))) => {
                    let line = text.as_str().strip_suffix('\n').unwrap_or(text.as_str()).to_string();
                    if line.contains('\n') {
                        break "lodge sent a frame of several lines".to_string();
                    }
                    if !line.is_empty() && on_message.send(BridgeMessage::Line { line }).is_err() {
                        break "the webview stopped listening".to_string();
                    }
                }
                Some(Ok(Message::Close(frame))) => {
                    break match frame {
                        Some(f) if !f.reason.is_empty() => format!("lodge closed the connection: {}", f.reason),
                        _ => "lodge closed the connection".to_string(),
                    };
                }
                Some(Ok(Message::Binary(_))) => break "lodge sent a binary frame".to_string(),
                // Pings are answered by the library; pongs and raw frames carry nothing.
                Some(Ok(_)) => {}
                Some(Err(e)) => break format!("read from lodge: {e}"),
                None => break "lodge closed the connection".to_string(),
            },
            () = stop.notified() => break "the connection was replaced".to_string(),
        }
    };
    let _ = on_message.send(BridgeMessage::Closed { error });
}

/// Send one JSON-RPC message on the calling window's connection; it must not
/// contain a newline.
#[tauri::command]
pub async fn maild_send(window: Window, bridge: State<'_, Bridge>, line: String) -> Result<(), String> {
    if line.contains('\n') {
        return Err("a frame must be a single line".into());
    }
    let mut guard = bridge.connections.lock().await;
    let conn = guard.get_mut(window.label()).ok_or("not connected to maild")?;
    match &mut conn.writer {
        Writer::Stream(w) => {
            let mut frame = line.into_bytes();
            frame.push(b'\n');
            w.write_all(&frame).await.map_err(|e| format!("write to maild: {e}"))
        }
        Writer::Lodge(w) => w.send(Message::text(line)).await.map_err(|e| format!("write to lodge: {e}")),
    }
}

/// Connect to maild's Unix socket; systemd starts maild on Linux.
#[cfg(not(windows))]
async fn connect(path: &Path) -> std::io::Result<(Box<dyn AsyncRead + Send + Unpin>, Box<dyn AsyncWrite + Send + Unpin>)> {
    let (read, write) = tokio::net::UnixStream::connect(path).await?.into_split();
    Ok((Box::new(read), Box::new(write)))
}

/// Connect to maild's named pipe, at the identification level (tokio's
/// default), which maild's check of the client's SID needs. With no maild
/// yet (the app started before the user's sign-in started it, or it
/// stopped), the app starts the maild.exe beside it and waits for its pipe
/// (docs/design/desktop.md, Windows).
#[cfg(windows)]
async fn connect(path: &Path) -> std::io::Result<(Box<dyn AsyncRead + Send + Unpin>, Box<dyn AsyncWrite + Send + Unpin>)> {
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
