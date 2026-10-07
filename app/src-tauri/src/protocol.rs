//! mailpart://localhost/<path> serves files maild has decoded into
//! <cache>/parts, so message images and attachments never travel as JSON and
//! the webview never touches the network for them (docs/specs/rpc-protocol.md).

use std::path::{Component, Path};

use tauri::http::{Request, Response, StatusCode, header};
use tauri::{Runtime, UriSchemeContext, UriSchemeResponder};

use crate::paths;

pub fn mailpart<R: Runtime>(_ctx: UriSchemeContext<'_, R>, request: Request<Vec<u8>>, responder: UriSchemeResponder) {
    let path = request.uri().path().to_string();
    if cfg!(debug_assertions) {
        eprintln!("mailpart: request {}", request.uri());
    }
    tauri::async_runtime::spawn(async move {
        let response = serve(&path).await;
        if cfg!(debug_assertions) {
            eprintln!("mailpart: {path} -> {}", response.status());
        }
        responder.respond(response)
    });
}

async fn serve(uri_path: &str) -> Response<Vec<u8>> {
    match read_part(uri_path).await {
        Ok((body, mime)) => Response::builder()
            .status(StatusCode::OK)
            .header(header::CONTENT_TYPE, mime)
            .header("X-Content-Type-Options", "nosniff")
            // Parts are passive content: an SVG or HTML part opened directly
            // must not run script or load anything.
            .header("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox")
            .header(header::CACHE_CONTROL, "private, max-age=3600")
            .body(body)
            .unwrap_or_else(|_| error(StatusCode::INTERNAL_SERVER_ERROR)),
        Err(status) => error(status),
    }
}

fn error(status: StatusCode) -> Response<Vec<u8>> {
    let mut r = Response::new(Vec::new());
    *r.status_mut() = status;
    r
}

/// Resolve a request path to a file under <cache>/parts. Paths are plain
/// relative names maild generates, so anything else is rejected rather than
/// decoded: no "..", no absolute paths, no percent-escapes, no symlink exits.
async fn read_part(uri_path: &str) -> Result<(Vec<u8>, &'static str), StatusCode> {
    let rel = uri_path.trim_start_matches('/');
    let valid_chars = rel.bytes().all(|b| b.is_ascii_alphanumeric() || b"._-/".contains(&b));
    let rel_path = Path::new(rel);
    if rel.is_empty() || !valid_chars || !rel_path.components().all(|c| matches!(c, Component::Normal(_))) {
        return Err(StatusCode::BAD_REQUEST);
    }
    let base = paths::cache_dir().map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?.join("parts");
    let base = tokio::fs::canonicalize(&base).await.map_err(|_| StatusCode::NOT_FOUND)?;
    let full = tokio::fs::canonicalize(base.join(rel_path)).await.map_err(|_| StatusCode::NOT_FOUND)?;
    if !full.starts_with(&base) {
        return Err(StatusCode::FORBIDDEN);
    }
    let body = tokio::fs::read(&full).await.map_err(|_| StatusCode::NOT_FOUND)?;
    Ok((body, mime_for(&full)))
}

fn mime_for(path: &Path) -> &'static str {
    match path.extension().and_then(|e| e.to_str()).map(str::to_ascii_lowercase).as_deref() {
        Some("png") => "image/png",
        Some("jpg" | "jpeg") => "image/jpeg",
        Some("gif") => "image/gif",
        Some("webp") => "image/webp",
        Some("svg") => "image/svg+xml",
        Some("pdf") => "application/pdf",
        Some("txt") => "text/plain; charset=utf-8",
        _ => "application/octet-stream",
    }
}
