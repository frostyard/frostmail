//! A line bridge between the webview and maild: maild's socket, a Unix
//! socket or on Windows a named pipe (ADR-0038), or lodge's /app/mail/api
//! WebSocket, one line per text frame, for a maild behind lodge
//! (docs/design/desktop.md, On a maild behind lodge). It never parses
//! frames: JSON-RPC multiplexing happens in the webview
//! (src/rpc/transport.ts), so the bridge cannot drift from the API.

use std::collections::HashMap;
use std::fmt;
use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use frostyard_problems::Recorder;
use futures_util::stream::{SplitSink, SplitStream};
use futures_util::{SinkExt, StreamExt};
use serde::Serialize;
use tauri::{State, Window, ipc::Channel};
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::net::TcpStream;
use tokio::sync::{Mutex, Notify, RwLock, RwLockWriteGuard};
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::{MaybeTlsStream, WebSocketStream};

use crate::{paths, problems, remote};

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
///
/// A change of mode (src/mode.rs) holds `mode` for writing from its first
/// write of app.json to its last connection change, so that no connection
/// reads a choice the change may still write back, and two changes never
/// overlap. A connection reads the choice under `mode` for reading, with
/// the generation that `close_all` advances; one whose generation has
/// passed by the time it connected is refused, and the window connects
/// again to the maild now chosen.
#[derive(Default)]
pub struct Bridge {
    connections: Mutex<HashMap<String, Connection>>,
    mode: RwLock<()>,
    generation: AtomicU64,
}

impl Bridge {
    /// Drop a closed window's connection.
    pub async fn forget(&self, label: &str) {
        self.connections.lock().await.remove(label);
    }

    /// End every window's connection, which each window's session then
    /// reconnects: the app switched maild.
    pub async fn close_all(&self) {
        let mut connections = self.connections.lock().await;
        self.generation.fetch_add(1, Ordering::SeqCst);
        connections.clear();
    }

    /// Hold off every new connection, and every other change of mode, for
    /// as long as the guard lives: a change of mode, from its first write of
    /// app.json to its last connection change.
    pub async fn changing_mode(&self) -> RwLockWriteGuard<'_, ()> {
        self.mode.write().await
    }

    /// The app's choice of maild, read while no change of mode runs, and
    /// the generation of the connections it is good for.
    async fn choice(&self) -> Result<(remote::AppConfig, u64), String> {
        let _reading = self.mode.read().await;
        Ok((remote::load()?, self.generation.load(Ordering::SeqCst)))
    }

    /// Keep a window's new connection, unless close_all ran since the
    /// choice it followed was read: then the connection is dropped, and the
    /// window connects again.
    async fn keep(&self, label: &str, writer: Writer, generation: u64) -> Result<Arc<Notify>, String> {
        let mut connections = self.connections.lock().await;
        if self.generation.load(Ordering::SeqCst) != generation {
            return Err("the app changed its maild; connect again".to_string());
        }
        let stop = Arc::new(Notify::new());
        connections.insert(label.to_string(), Connection { writer, stop: stop.clone() });
        Ok(stop)
    }
}

/// What maild_connect tells the webview about the connection.
#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Connected {
    /// maild is behind lodge, which closes a connection that sends nothing
    /// for 90 s, so the webview keeps it alive.
    pub remote: bool,
    /// The lodge URL this connection reaches maild at, as app.json holds it;
    /// null for maild's local socket. Two connections with different values
    /// reach different mailds, whose ids are not each other's.
    pub lodge: Option<String>,
}

/// Connect the calling window to maild, its local socket or lodge's
/// /app/mail/api as app.json says, and stream every received line to
/// `on_message`.
#[tauri::command]
pub async fn maild_connect(
    window: Window,
    bridge: State<'_, Bridge>,
    rec: State<'_, Recorder>,
    on_message: Channel<BridgeMessage>,
) -> Result<Connected, String> {
    let (config, generation) = bridge.choice().await?;
    if let Some(url) = config.lodge {
        let lodge = remote::Lodge::parse(&url)?;
        let (write, read) = connect_lodge(&lodge).await?.split();
        let stop = bridge.keep(window.label(), Writer::Lodge(write), generation).await?;
        tauri::async_runtime::spawn(read_frames(read, stop, on_message));
        return Ok(Connected { remote: true, lodge: Some(url) });
    }
    let path = paths::socket()?;
    let (read, write) = connect(&path)
        .await
        .map_err(|e| format!("connect to maild at {}: {e}", path.display()))?;
    let stop = bridge.keep(window.label(), Writer::Stream(write), generation).await?;
    let rec = rec.inner().clone();
    tauri::async_runtime::spawn(async move {
        let end = read_lines(read, &stop, |line| on_message.send(BridgeMessage::Line { line }).is_ok()).await;
        record_end(&rec, &end);
        let _ = on_message.send(BridgeMessage::Closed { error: end.to_string() });
    });
    Ok(Connected { remote: false, lodge: None })
}

/// Why the read loop of maild's local socket ended. Its text is what the
/// webview's Closed says.
#[derive(Debug)]
enum End {
    /// The window connected again, closed, or the app switched maild.
    Replaced,
    /// The webview no longer takes lines.
    WebviewGone,
    /// maild closed the socket.
    MaildClosed,
    /// A read from the socket failed.
    ReadError(std::io::Error),
}

impl fmt::Display for End {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            End::Replaced => f.write_str("the connection was replaced"),
            End::WebviewGone => f.write_str("the webview stopped listening"),
            End::MaildClosed => f.write_str("maild closed the connection"),
            End::ReadError(e) => write!(f, "read from maild: {e}"),
        }
    }
}

impl End {
    /// Whether the shell lost maild's socket under a window, which records
    /// [`problems::SOCKET_LOST`]: maild closed it, or a read failed. The
    /// app's own ends (a replaced connection, a webview that went away)
    /// are no problem. Lodge's WebSocket never comes here: its losses are
    /// the network's.
    fn socket_lost(&self) -> bool {
        matches!(self, End::MaildClosed | End::ReadError(_))
    }
}

/// Records [`problems::SOCKET_LOST`] when `end` lost maild's socket, with
/// the end's text as the record's message.
fn record_end(rec: &Recorder, end: &End) {
    if end.socket_lost() {
        rec.error(problems::SOCKET_LOST, Some(end));
    }
}

/// Passes each line maild's local socket sends to `deliver`, until the
/// socket ends, `deliver` returns false (the webview stopped listening), or
/// `stop` is notified. `stop` is polled first: a connection that was
/// replaced ends as replaced even when its socket is at its end too, so it
/// records no problem.
async fn read_lines(read: impl AsyncRead + Unpin, stop: &Notify, mut deliver: impl FnMut(String) -> bool) -> End {
    let mut lines = BufReader::new(read).lines();
    loop {
        tokio::select! {
            biased;
            () = stop.notified() => return End::Replaced,
            next = lines.next_line() => match next {
                Ok(Some(line)) => {
                    if !deliver(line) {
                        return End::WebviewGone;
                    }
                }
                Ok(None) => return End::MaildClosed,
                Err(e) => return End::ReadError(e),
            },
        }
    }
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

#[cfg(test)]
mod tests {
    use super::{Bridge, End, Writer};

    #[test]
    fn only_a_lost_socket_is_a_problem() {
        assert!(End::MaildClosed.socket_lost());
        assert!(End::ReadError(std::io::Error::other("reset")).socket_lost());
        assert!(!End::Replaced.socket_lost());
        assert!(!End::WebviewGone.socket_lost());
        // The webview's Closed keeps its words.
        assert_eq!(End::MaildClosed.to_string(), "maild closed the connection");
        assert_eq!(End::Replaced.to_string(), "the connection was replaced");
        assert_eq!(End::WebviewGone.to_string(), "the webview stopped listening");
        assert_eq!(End::ReadError(std::io::Error::other("reset")).to_string(), "read from maild: reset");
    }

    /// maild closing its socket under a window records bridge.socket_lost;
    /// the window's own ends record nothing.
    #[cfg(target_os = "linux")]
    #[test]
    fn a_socket_maild_closes_records_socket_lost() {
        use super::{read_lines, record_end};
        use crate::problems::{self, testing::Journal};
        use tokio::io::AsyncWriteExt;
        use tokio::net::UnixStream;
        use tokio::sync::Notify;

        tauri::async_runtime::block_on(async {
            let journal = Journal::new();
            let rec = problems::recorder("1.2.3").with_socket(&journal.path);
            let stop = Notify::new();

            // maild sends a line, then closes the socket.
            let (shell, mut maild) = UnixStream::pair().unwrap();
            maild.write_all(b"{\"jsonrpc\":\"2.0\"}\n").await.unwrap();
            drop(maild);
            let mut lines = Vec::new();
            let end = read_lines(shell, &stop, |line| {
                lines.push(line);
                true
            })
            .await;
            assert!(matches!(end, End::MaildClosed), "{end:?}");
            assert_eq!(lines, ["{\"jsonrpc\":\"2.0\"}"]);
            record_end(&rec, &end);
            let records = journal.records();
            assert_eq!(records.len(), 1, "records after maild closed the socket");
            let (fields, detail) = &records[0];
            assert_eq!(fields["MESSAGE"], "maild closed the connection");
            assert_eq!(
                (detail.app_id.as_str(), detail.app_version.as_str(), detail.runtime.as_str()),
                ("org.frostyard.Frostmail", "1.2.3", "rust")
            );
            assert_eq!((detail.component.as_str(), detail.class.as_str()), ("shell", "error"));
            assert_eq!(detail.code.as_deref(), Some(problems::SOCKET_LOST));

            // The connection is replaced while maild is still there.
            let (shell, _maild) = UnixStream::pair().unwrap();
            stop.notify_one();
            let end = read_lines(shell, &stop, |_| true).await;
            assert!(matches!(end, End::Replaced), "{end:?}");
            record_end(&rec, &end);

            // The webview stops listening.
            let (shell, mut maild) = UnixStream::pair().unwrap();
            maild.write_all(b"{}\n").await.unwrap();
            let end = read_lines(shell, &stop, |_| false).await;
            assert!(matches!(end, End::WebviewGone), "{end:?}");
            record_end(&rec, &end);
            assert!(journal.records().is_empty(), "a window's own end recorded a problem");

            // An unstamped build records nothing, even for a lost socket.
            problems::recorder("0.0.0").with_socket(&journal.path).error(problems::SOCKET_LOST, None);
            record_end(&problems::recorder("0.0.0").with_socket(&journal.path), &End::MaildClosed);
            assert!(journal.records().is_empty(), "an unstamped build recorded a problem");
        });
    }

    /// A connection that is replaced while maild's end is closed too ends
    /// as replaced, and records nothing: the stop wins over the socket's end.
    /// Without a biased select, either branch could win; 50 rounds catch it.
    #[cfg(target_os = "linux")]
    #[test]
    fn a_replaced_connection_at_eof_records_nothing() {
        use super::{read_lines, record_end};
        use crate::problems::{self, testing::Journal};
        use tokio::net::UnixStream;
        use tokio::sync::Notify;

        tauri::async_runtime::block_on(async {
            let journal = Journal::new();
            let rec = problems::recorder("1.2.3").with_socket(&journal.path);
            for round in 0..50 {
                let (shell, maild) = UnixStream::pair().unwrap();
                drop(maild);
                let stop = Notify::new();
                stop.notify_one();
                let end = read_lines(shell, &stop, |_| true).await;
                assert!(matches!(end, End::Replaced), "round {round}: {end:?}");
                record_end(&rec, &end);
            }
            assert!(journal.records().is_empty(), "a replaced connection recorded a problem");
        });
    }

    fn writer() -> Writer {
        Writer::Stream(Box::new(tokio::io::sink()))
    }

    #[test]
    fn a_connection_made_across_a_change_of_maild_is_refused() {
        tauri::async_runtime::block_on(async {
            let bridge = Bridge::default();
            let before = bridge.generation.load(std::sync::atomic::Ordering::SeqCst);
            assert!(bridge.keep("main", writer(), before).await.is_ok());
            // The app switched maild after this window read its choice.
            bridge.close_all().await;
            assert!(bridge.connections.lock().await.is_empty());
            assert!(bridge.keep("main", writer(), before).await.is_err());
            assert!(bridge.connections.lock().await.is_empty());
            let now = bridge.generation.load(std::sync::atomic::Ordering::SeqCst);
            assert!(bridge.keep("main", writer(), now).await.is_ok());
        });
    }

    #[test]
    fn no_connection_reads_the_choice_during_a_change_of_mode() {
        tauri::async_runtime::block_on(async {
            let bridge = Bridge::default();
            let changing = bridge.changing_mode().await;
            assert!(bridge.mode.try_read().is_err(), "a reader got in during a change of mode");
            assert!(bridge.mode.try_write().is_err(), "a second change of mode got in");
            drop(changing);
            assert!(bridge.mode.try_read().is_ok());
        });
    }
}
