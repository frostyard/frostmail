//! A line bridge between the webview and maild's Unix socket. It never parses
//! frames: JSON-RPC multiplexing happens in the webview
//! (src/rpc/transport.ts), so the bridge cannot drift from the API.

use serde::Serialize;
use tauri::{State, ipc::Channel};
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::UnixStream;
use tokio::net::unix::OwnedWriteHalf;
use tokio::sync::Mutex;

use crate::paths;

/// What the webview receives on its channel, in order.
#[derive(Clone, Serialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum BridgeMessage {
    /// One newline-delimited JSON-RPC message from maild.
    Line { line: String },
    /// The connection ended; no more lines follow.
    Closed { error: String },
}

/// The current connection's write half. Connecting again replaces it, which
/// shuts the old connection down.
#[derive(Default)]
pub struct Bridge {
    writer: Mutex<Option<OwnedWriteHalf>>,
}

/// Connect to maild and stream every received line to `on_message`.
#[tauri::command]
pub async fn maild_connect(bridge: State<'_, Bridge>, on_message: Channel<BridgeMessage>) -> Result<(), String> {
    let path = paths::socket()?;
    let stream = UnixStream::connect(&path)
        .await
        .map_err(|e| format!("connect to maild at {}: {e}", path.display()))?;
    let (read, write) = stream.into_split();
    *bridge.writer.lock().await = Some(write);
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

/// Send one JSON-RPC message, which must not contain a newline.
#[tauri::command]
pub async fn maild_send(bridge: State<'_, Bridge>, line: String) -> Result<(), String> {
    if line.contains('\n') {
        return Err("a frame must be a single line".into());
    }
    let mut guard = bridge.writer.lock().await;
    let writer = guard.as_mut().ok_or("not connected to maild")?;
    let mut frame = line.into_bytes();
    frame.push(b'\n');
    writer.write_all(&frame).await.map_err(|e| format!("write to maild: {e}"))
}
