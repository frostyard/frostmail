//! A maild behind lodge (docs/design/desktop.md, On a maild behind lodge;
//! ADR-0041): the app's own choice of maild, lodge's address and the URLs
//! made from it, and what the Rust layer fetches from lodge itself, since
//! the webview fetches nothing.

use std::io::Read;
use std::path::{Path, PathBuf};
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tauri::Url;

use crate::paths;

/// The app's own configuration, app.json in paths::config_dir(). Without
/// the file, or without `lodge`, the app asks the maild on its own machine.
#[derive(Clone, Debug, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AppConfig {
    /// lodge's address, such as https://lodge.<tailnet>.ts.net.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub lodge: Option<String>,
}

fn config_file() -> Result<PathBuf, String> {
    Ok(paths::config_dir()?.join("app.json"))
}

/// Read the configuration; a missing file is the default.
pub fn load() -> Result<AppConfig, String> {
    let path = config_file()?;
    match std::fs::read(&path) {
        Ok(data) => serde_json::from_slice(&data).map_err(|e| format!("{}: {e}", path.display())),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(AppConfig::default()),
        Err(e) => Err(format!("{}: {e}", path.display())),
    }
}

/// Write the configuration, after checking its lodge address.
pub fn save(cfg: &AppConfig) -> Result<(), String> {
    if let Some(url) = &cfg.lodge {
        Lodge::parse(url)?;
    }
    let path = config_file()?;
    if let Some(dir) = path.parent() {
        std::fs::create_dir_all(dir).map_err(|e| format!("{}: {e}", dir.display()))?;
    }
    let data = serde_json::to_vec_pretty(cfg).map_err(|e| e.to_string())?;
    let tmp = path.with_extension("json.tmp");
    std::fs::write(&tmp, data).map_err(|e| format!("{}: {e}", tmp.display()))?;
    std::fs::rename(&tmp, &path).map_err(|e| format!("{}: {e}", path.display()))
}

/// lodge's address: https on the tailnet, or http on loopback for lodge's
/// development build (docs/specs/lodge-http.md, The development build).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Lodge {
    origin: String,
    secure: bool,
}

impl Lodge {
    /// Parse lodge's address: a scheme and a host, with an optional port
    /// and no path, query, fragment or credentials. Plain http is for a
    /// loopback address only.
    pub fn parse(s: &str) -> Result<Lodge, String> {
        let url = Url::parse(s.trim()).map_err(|e| format!("lodge's address {s:?}: {e}"))?;
        let host = url.host_str().ok_or_else(|| format!("lodge's address {s:?} has no host"))?;
        if !url.username().is_empty() || url.password().is_some() || url.query().is_some() || url.fragment().is_some()
        {
            return Err(format!("lodge's address {s:?} must be a scheme and a host"));
        }
        if url.path() != "/" && !url.path().is_empty() {
            return Err(format!("lodge's address {s:?} must have no path"));
        }
        let secure = match url.scheme() {
            "https" => true,
            "http" if loopback(host) => false,
            "http" => return Err(format!("lodge's address {s:?} must be https off this machine")),
            scheme => return Err(format!("lodge's address {s:?} must be https, not {scheme}")),
        };
        Ok(Lodge { origin: url.origin().ascii_serialization(), secure })
    }

    /// lodge's origin, which the bridge sends as its Origin (D7).
    pub fn origin(&self) -> &str {
        &self.origin
    }

    /// The mail app's /api WebSocket.
    pub fn api_url(&self) -> String {
        let rest = self.origin.split_once("://").map_or("", |(_, r)| r);
        format!("{}://{rest}/app/mail/api", if self.secure { "wss" } else { "ws" })
    }

    /// lodge's content origin: contentOrigin from GET /lodge/apps, the JSON
    /// lodge's shell reads, asked with lodge's Origin; derived from lodge's
    /// own name only when that request fails, since lodge can rename its
    /// content node (lodge-http, Native clients).
    pub fn content_origin(&self) -> Result<String, String> {
        #[derive(Deserialize)]
        struct Apps {
            #[serde(rename = "contentOrigin")]
            content_origin: Option<String>,
        }
        let asked = agent()
            .get(&format!("{}/lodge/apps", self.origin))
            .header("Origin", &self.origin)
            .call()
            .map_err(|e| e.to_string())
            .and_then(|mut r| r.body_mut().read_to_string().map_err(|e| e.to_string()))
            .and_then(|body| serde_json::from_str::<Apps>(&body).map_err(|e| e.to_string()));
        match asked {
            Ok(Apps { content_origin: Some(o) }) => Lodge::parse(&o).map(|l| l.origin),
            Ok(Apps { content_origin: None }) => Err("lodge serves no content origin for mail".into()),
            Err(e) => {
                eprintln!("lodge's /lodge/apps: {e}; deriving the content origin");
                self.derived_content_origin()
            }
        }
    }

    /// The content node's usual name: lodge's, with lodge-content for its
    /// first label. On loopback there is no name to derive.
    fn derived_content_origin(&self) -> Result<String, String> {
        let (scheme, rest) = self.origin.split_once("://").unwrap_or(("https", ""));
        match rest.split_once('.') {
            Some((_, domain)) if self.secure => Ok(format!("{scheme}://lodge-content.{domain}")),
            _ => Err(format!("cannot tell {}'s content origin", self.origin)),
        }
    }
}

fn loopback(host: &str) -> bool {
    let host = host.trim_start_matches('[').trim_end_matches(']');
    host == "localhost" || host.parse::<std::net::IpAddr>().is_ok_and(|ip| ip.is_loopback())
}

/// Attachments downloaded at once are at most this big.
const MAX_DOWNLOAD: u64 = 128 << 20;

fn agent() -> ureq::Agent {
    let config = ureq::Agent::config_builder()
        .timeout_global(Some(Duration::from_secs(120)))
        .max_redirects(0)
        .user_agent(concat!("Frostmail/", env!("CARGO_PKG_VERSION")));
    // Windows has the system's TLS (Cargo.toml); elsewhere ureq's rustls.
    #[cfg(windows)]
    let config = config.tls_config(ureq::tls::TlsConfig::builder().provider(ureq::tls::TlsProvider::NativeTls).build());
    config.build().into()
}

/// Download a part from lodge's content node (`<contentOrigin>/mail/<contentPath>`,
/// docs/specs/lodge-app.md) into the app's downloads, as filename, and
/// return the file. contentPath is maild's, already percent-encoded.
pub fn download(lodge: &Lodge, content_path: &str, filename: &str) -> Result<PathBuf, String> {
    download_into(&paths::downloads_dir()?, lodge, content_path, filename)
}

fn download_into(dir: &Path, lodge: &Lodge, content_path: &str, filename: &str) -> Result<PathBuf, String> {
    if content_path.starts_with('/') || content_path.contains("..") || content_path.contains(['?', '#']) {
        return Err(format!("not a content path: {content_path}"));
    }
    let url = format!("{}/mail/{content_path}", lodge.content_origin()?);
    let mut resp = agent().get(&url).call().map_err(|e| format!("download {url}: {e}"))?;
    let mut data = Vec::new();
    resp.body_mut()
        .as_reader()
        .take(MAX_DOWNLOAD + 1)
        .read_to_end(&mut data)
        .map_err(|e| format!("download {url}: {e}"))?;
    if data.len() as u64 > MAX_DOWNLOAD {
        return Err(format!("{filename} is over {} MiB", MAX_DOWNLOAD >> 20));
    }
    std::fs::create_dir_all(dir).map_err(|e| format!("{}: {e}", dir.display()))?;
    let path = dir.join(safe_name(filename));
    write_new(&path, &data)?;
    Ok(path)
}

/// A file name that stays in its directory: no separators, no leading dot,
/// no control characters.
fn safe_name(name: &str) -> String {
    let cleaned: String = name
        .chars()
        .map(|c| if c.is_control() || matches!(c, '/' | '\\' | ':') { '_' } else { c })
        .collect();
    let trimmed = cleaned.trim().trim_start_matches('.');
    if trimmed.is_empty() { "attachment".to_string() } else { trimmed.to_string() }
}

fn write_new(path: &Path, data: &[u8]) -> Result<(), String> {
    let tmp = path.with_extension("download");
    std::fs::write(&tmp, data).map_err(|e| format!("{}: {e}", tmp.display()))?;
    std::fs::rename(&tmp, path).map_err(|e| format!("{}: {e}", path.display()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_lodge_addresses() {
        let l = Lodge::parse("https://lodge.example.ts.net").unwrap();
        assert_eq!(l.origin(), "https://lodge.example.ts.net");
        assert_eq!(l.api_url(), "wss://lodge.example.ts.net/app/mail/api");
        assert_eq!(l.derived_content_origin().unwrap(), "https://lodge-content.example.ts.net");
        let l = Lodge::parse("https://lodge.example.ts.net/").unwrap();
        assert_eq!(l.origin(), "https://lodge.example.ts.net");
        let dev = Lodge::parse("http://127.0.0.1:8480").unwrap();
        assert_eq!(dev.origin(), "http://127.0.0.1:8480");
        assert_eq!(dev.api_url(), "ws://127.0.0.1:8480/app/mail/api");
        assert!(dev.derived_content_origin().is_err());
        assert!(Lodge::parse("http://[::1]:8480").is_ok());
        for bad in [
            "http://lodge.example.ts.net",
            "ftp://lodge.example.ts.net",
            "https://lodge.example.ts.net/app",
            "https://u:p@lodge.example.ts.net",
            "https://lodge.example.ts.net/?x=1",
            "lodge.example.ts.net",
            "",
        ] {
            assert!(Lodge::parse(bad).is_err(), "{bad} parsed");
        }
    }

    #[test]
    fn config_round_trips_and_refuses_unknown_fields() {
        let cfg: AppConfig = serde_json::from_str(r#"{"lodge":"https://lodge.example.ts.net"}"#).unwrap();
        assert_eq!(cfg.lodge.as_deref(), Some("https://lodge.example.ts.net"));
        assert_eq!(serde_json::to_string(&AppConfig::default()).unwrap(), "{}");
        assert!(serde_json::from_str::<AppConfig>(r#"{"lodg":"x"}"#).is_err());
    }

    /// A loopback server answering lodge's /lodge/apps and one content path,
    /// recording each request's head.
    fn fake_lodge(apps: &'static str, part: &'static [u8]) -> (String, std::sync::mpsc::Receiver<String>) {
        use std::io::{BufRead, BufReader, Write};
        let ln = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let origin = format!("http://{}", ln.local_addr().unwrap());
        let (tx, rx) = std::sync::mpsc::channel();
        let apps = apps.replace("ORIGIN", &origin);
        std::thread::spawn(move || {
            for conn in ln.incoming() {
                let mut conn = conn.unwrap();
                let mut head = String::new();
                let mut reader = BufReader::new(conn.try_clone().unwrap());
                loop {
                    let mut line = String::new();
                    if reader.read_line(&mut line).unwrap() == 0 || line == "\r\n" {
                        break;
                    }
                    head.push_str(&line);
                }
                let (status, body): (&str, Vec<u8>) = if head.starts_with("GET /lodge/apps ") {
                    ("200 OK", apps.clone().into_bytes())
                } else if head.starts_with("GET /mail/p/mac/m/1/2/notes.txt ") {
                    ("200 OK", part.to_vec())
                } else {
                    ("404 Not Found", b"not found".to_vec())
                };
                tx.send(head).unwrap();
                let _ = write!(conn, "HTTP/1.1 {status}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", body.len());
                let _ = conn.write_all(&body);
            }
        });
        (origin, rx)
    }

    #[test]
    fn downloads_from_the_content_origin_lodge_names() {
        let (origin, heads) = fake_lodge(r#"{"contract":1,"contentOrigin":"ORIGIN","apps":[]}"#, b"the attachment");
        let lodge = Lodge::parse(&origin).unwrap();
        let dir = std::env::temp_dir().join(format!("frostmail-download-{}", std::process::id()));
        let path = download_into(&dir, &lodge, "p/mac/m/1/2/notes.txt", "notes.txt").unwrap();
        assert_eq!(std::fs::read(&path).unwrap(), b"the attachment");
        assert_eq!(path.file_name().unwrap(), "notes.txt");
        let apps = heads.recv().unwrap();
        assert!(apps.contains(&format!("origin: {origin}")) || apps.contains(&format!("Origin: {origin}")), "{apps}");
        assert!(download_into(&dir, &lodge, "p/mac/m/1/2/other.txt", "other.txt").is_err());
        assert!(download_into(&dir, &lodge, "../etc/passwd", "x").is_err());
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn download_names_stay_put() {
        assert_eq!(safe_name("../../etc/passwd"), "_.._etc_passwd");
        assert_eq!(safe_name(".hidden"), "hidden");
        assert_eq!(safe_name("Résumé (final).pdf"), "Résumé (final).pdf");
        assert_eq!(safe_name("a\u{0}b"), "a_b");
        assert_eq!(safe_name(""), "attachment");
    }
}
