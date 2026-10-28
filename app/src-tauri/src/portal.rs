//! Save As through the file chooser portal
//! (docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md, Files in
//! the sandbox): the Flatpak's window cannot write outside the sandbox, so
//! the Rust layer asks org.freedesktop.portal.FileChooser.SaveFile over the
//! session bus where the file goes. The path it answers is under the
//! document portal, which every instance of the app can write. Outside the
//! Flatpak, and on Windows, the app keeps Tauri's dialog instead
//! (app/src/app/ReaderContainer.tsx).

#[cfg(not(target_os = "linux"))]
use std::path::PathBuf;

/// save_file asks the file chooser portal where to save a file: the path
/// it answers, or None when the user cancels. The portal opens a window and
/// the wait ends when it answers or leaves the session bus, so the command
/// awaits it (commands::portal_save_file).
#[cfg(target_os = "linux")]
pub use linux::save_file;

/// Windows, and any other system, has no Flatpak and no such portal: the
/// command refuses Save As there before it gets here (paths::in_flatpak).
#[cfg(not(target_os = "linux"))]
pub async fn save_file(
    _title: &str,
    _current_name: &str,
    _filter_name: &str,
    _extension: &str,
) -> Result<Option<PathBuf>, String> {
    Err("Save As through the file chooser portal is only in the Flatpak".to_string())
}

/// The portal's bus code: zbus comes with the app on Linux only
/// (Cargo.toml). Its helpers are separate functions so that the tests reach
/// them without a bus.
#[cfg(target_os = "linux")]
mod linux {
    use std::collections::HashMap;
    use std::ffi::OsString;
    use std::os::unix::ffi::OsStringExt;
    use std::path::PathBuf;
    use std::sync::atomic::{AtomicU64, Ordering};

    use futures_util::StreamExt;
    use zbus::message::Type;
    use zbus::zvariant::{OwnedObjectPath, OwnedValue, Value};
    use zbus::{Connection, MatchRule, MessageStream};

    /// The portal: its name, its object path, and the interfaces of the
    /// request, of its answer, and of the bus's own names.
    const PORTAL_NAME: &str = "org.freedesktop.portal.Desktop";
    const PORTAL_PATH: &str = "/org/freedesktop/portal/desktop";
    const FILE_CHOOSER: &str = "org.freedesktop.portal.FileChooser";
    const REQUEST: &str = "org.freedesktop.portal.Request";
    const BUS: &str = "org.freedesktop.DBus";
    const BUS_PATH: &str = "/org/freedesktop/DBus";

    /// TOKENS numbers the requests, so that two Save As dialogs at one time
    /// get two request paths.
    static TOKENS: AtomicU64 = AtomicU64::new(0);

    /// save_file opens the portal's save dialog on the session bus.
    pub async fn save_file(
        title: &str,
        current_name: &str,
        filter_name: &str,
        extension: &str,
    ) -> Result<Option<PathBuf>, String> {
        let conn = Connection::session().await.map_err(|e| format!("the session bus: {e}"))?;
        save_file_on(&conn, PORTAL_NAME, title, current_name, filter_name, extension).await
    }

    /// save_file_on asks the portal named destination, so that the tests
    /// serve a portal of their own instead of the desktop's.
    async fn save_file_on(
        conn: &Connection,
        destination: &str,
        title: &str,
        current_name: &str,
        filter_name: &str,
        extension: &str,
    ) -> Result<Option<PathBuf>, String> {
        let sender = conn.unique_name().ok_or("the session bus gave this connection no name")?.to_string();
        let token = format!("frostmail{}", TOKENS.fetch_add(1, Ordering::Relaxed));
        let expected = request_path(&sender, &token);
        // Both subscriptions come first: a portal may answer, and leave the
        // bus, before its own reply arrives. The answer's rule takes every
        // Response this connection receives (a portal answers the caller
        // alone), because an old portal answers on a path it makes itself
        // rather than from the token; the loop then takes ours by its path
        // and by its sender.
        let answer_rule = MatchRule::builder()
            .msg_type(Type::Signal)
            .interface(REQUEST)
            .map_err(|e| e.to_string())?
            .member("Response")
            .map_err(|e| e.to_string())?
            .build();
        let mut answers = MessageStream::for_match_rule(answer_rule, conn, None)
            .await
            .map_err(|e| format!("the portal's answer: {e}"))?;
        // A portal that leaves the bus never answers, so its own name's
        // OwnerChanged ends the wait instead of it lasting forever.
        let owner_rule = MatchRule::builder()
            .msg_type(Type::Signal)
            .sender(BUS)
            .map_err(|e| e.to_string())?
            .interface(BUS)
            .map_err(|e| e.to_string())?
            .member("NameOwnerChanged")
            .map_err(|e| e.to_string())?
            .path(BUS_PATH)
            .map_err(|e| e.to_string())?
            .arg(0, destination)
            .map_err(|e| e.to_string())?
            .build();
        let mut owners =
            MessageStream::for_match_rule(owner_rule, conn, None).await.map_err(|e| format!("the bus's names: {e}"))?;
        let filters: Vec<(String, Vec<(u32, String)>)> =
            vec![(filter_name.to_string(), vec![(0, format!("*.{extension}"))])];
        let options: HashMap<&str, Value<'static>> = HashMap::from([
            ("handle_token", Value::from(token.clone())),
            ("modal", Value::from(true)),
            ("current_name", Value::from(current_name.to_string())),
            ("filters", Value::from(filters.clone())),
            ("current_filter", Value::from(filters[0].clone())),
        ]);
        let reply = conn
            .call_method(Some(destination), PORTAL_PATH, Some(FILE_CHOOSER), "SaveFile", &("", title, options))
            .await
            .map_err(|e| format!("the file chooser portal: {e}"))?;
        let handle: OwnedObjectPath =
            reply.body().deserialize().map_err(|e| format!("the file chooser portal's answer: {e}"))?;
        // Only the portal's own Response counts: any peer of the session bus
        // may send a signal on the request path, and an answer without a
        // sender is no portal's.
        let portal = reply.header().sender().map(|s| s.to_string());
        loop {
            // Answers first: the connection's reader fills both queues in the
            // order the bus delivers, so a Response the portal sent before it
            // left is in the answers' queue by the time its leave is in the
            // owners'. Biased, the loop drains every queued answer before it
            // looks at a leave, and a valid answer is never lost to one.
            tokio::select! {
                biased;
                answer = answers.next() => {
                    let Some(answer) = answer else {
                        return Err("the file chooser portal sent no Response".to_string());
                    };
                    let answer = answer.map_err(|e| format!("the file chooser portal's answer: {e}"))?;
                    let from = answer.header().sender().map(|s| s.to_string());
                    let at = answer.header().path().map(|p| p.to_string()).unwrap_or_default();
                    if !ours(from.as_deref(), portal.as_deref(), &at, &expected, handle.as_str()) {
                        continue;
                    }
                    let (code, hints): (u32, HashMap<String, OwnedValue>) =
                        answer.body().deserialize().map_err(|e| format!("the file chooser portal's Response: {e}"))?;
                    return chosen(code, &hints);
                }
                owner = owners.next() => {
                    let Some(owner) = owner else {
                        return Err("the session bus ended the wait for the file chooser portal".to_string());
                    };
                    let owner = owner.map_err(|e| format!("the bus's names: {e}"))?;
                    let (_, old, _): (String, String, String) = owner
                        .body()
                        .deserialize()
                        .map_err(|e| format!("the bus's NameOwnerChanged: {e}"))?;
                    if Some(old.as_str()) == portal.as_deref() {
                        return Err("the file chooser portal left the session bus".to_string());
                    }
                }
            }
        }
    }

    /// ours reports whether a Response answers this request: it comes from
    /// the unique name that sent the SaveFile reply (any peer of the session
    /// bus may signal on the request path, and an answer without a sender is
    /// no portal's), on the token's path or the handle the reply named.
    fn ours(from: Option<&str>, portal: Option<&str>, at: &str, expected: &str, handle: &str) -> bool {
        from.is_some() && from == portal && (at == expected || at == handle)
    }

    /// request_path is the object path the portal answers a request on:
    /// /org/freedesktop/portal/desktop/request/<sender>/<token>, with the
    /// caller's unique name as a path part (no leading colon, dots
    /// replaced by underscores).
    fn request_path(sender: &str, token: &str) -> String {
        format!("{PORTAL_PATH}/request/{}/{token}", sender.trim_start_matches(':').replace('.', "_"))
    }

    /// chosen maps a Response's code and hints: 0 with the first of uris is
    /// the file the user chose, 1 is a cancel, and any other code is a
    /// failure (the FileChooser portal's Response).
    fn chosen(code: u32, hints: &HashMap<String, OwnedValue>) -> Result<Option<PathBuf>, String> {
        match code {
            0 => {
                let uris = hints.get("uris").ok_or("the file chooser portal answered no uris")?;
                let uris: Vec<String> = Value::from(uris.clone())
                    .try_into()
                    .map_err(|e| format!("the file chooser portal's uris: {e}"))?;
                let first = uris.first().ok_or("the file chooser portal chose no file")?;
                Ok(Some(uri_to_path(first)?))
            }
            1 => Ok(None),
            code => Err(format!("the file chooser portal failed Save As: response {code}")),
        }
    }

    /// uri_to_path is the local path a file:// URI names: the portal answers
    /// URIs, whose bytes show escaped (a space is %20). Another host, and a
    /// relative path, are refused: the app writes only a file on this
    /// machine.
    fn uri_to_path(uri: &str) -> Result<PathBuf, String> {
        let rest = uri.strip_prefix("file://").ok_or_else(|| format!("{uri} is not a file:// URI"))?;
        let (host, path) = match rest.find('/') {
            Some(at) => rest.split_at(at),
            None => (rest, ""),
        };
        if !host.is_empty() && host != "localhost" {
            return Err(format!("{uri} is not on this machine"));
        }
        if !path.starts_with('/') {
            return Err(format!("{uri} names no absolute path"));
        }
        Ok(PathBuf::from(OsString::from_vec(percent_decoded(path)?)))
    }

    /// percent_decoded turns the escaped bytes of a URI's path into the
    /// bytes they name, so that a name that is not UTF-8 survives too.
    fn percent_decoded(path: &str) -> Result<Vec<u8>, String> {
        let mut out = Vec::with_capacity(path.len());
        let mut bytes = path.bytes();
        while let Some(byte) = bytes.next() {
            if byte != b'%' {
                out.push(byte);
                continue;
            }
            let digits: Vec<u8> = bytes.by_ref().take(2).collect();
            let hex = std::str::from_utf8(&digits)
                .ok()
                .filter(|h| h.len() == 2 && h.bytes().all(|b| b.is_ascii_hexdigit()))
                .ok_or_else(|| format!("{path}: a % escape that is not 2 hexadecimal digits"))?;
            out.push(u8::from_str_radix(hex, 16).map_err(|e| format!("{path}: %{hex}: {e}"))?);
        }
        Ok(out)
    }

    #[cfg(test)]
    mod tests {
        use std::collections::{HashMap, VecDeque};
        use std::path::PathBuf;
        use std::sync::mpsc::{self, Receiver};
        use std::sync::{Arc, Mutex};
        use std::time::Duration;

        use zbus::zvariant::{OwnedObjectPath, OwnedValue, Value};

        use super::{PORTAL_PATH, REQUEST, chosen, ours, request_path, save_file_on, uri_to_path};

        #[test]
        fn the_request_path_is_the_token_under_the_callers_name() {
            assert_eq!(request_path(":1.42", "frostmail7"), "/org/freedesktop/portal/desktop/request/1_42/frostmail7");
            assert_eq!(request_path("1.42", "t"), "/org/freedesktop/portal/desktop/request/1_42/t");
        }

        #[test]
        fn a_uri_becomes_the_path_it_names() {
            assert_eq!(uri_to_path("file:///home/u/My%20Mail.eml"), Ok(PathBuf::from("/home/u/My Mail.eml")));
            assert_eq!(
                uri_to_path("file://localhost/home/u/R%C3%A9sum%C3%A9.eml"),
                Ok(PathBuf::from("/home/u/Résumé.eml"))
            );
            // Another scheme, another host, a relative path and a broken
            // escape are refused.
            for refused in [
                "https://example.test/Out.eml",
                "sftp://example.test/Out.eml",
                "file://otherhost/home/u/Out.eml",
                "file://home/u/Out.eml",
                "file:///home/u/%2.eml",
                "file:///home/u/%zz.eml",
            ] {
                assert!(uri_to_path(refused).is_err(), "{refused}");
            }
        }

        #[test]
        fn a_response_maps_its_code() {
            let picked = uris("file:///home/u/Out.eml");
            assert_eq!(chosen(0, &picked), Ok(Some(PathBuf::from("/home/u/Out.eml"))));
            assert_eq!(chosen(1, &picked), Ok(None), "a cancel writes nothing");
            assert!(chosen(2, &picked).is_err(), "a failure is an error");
            assert!(chosen(0, &HashMap::new()).is_err(), "no uris is an error");
            assert!(chosen(0, &uris("https://example.test/Out.eml")).is_err());
        }

        /// A Response's hints for a file: its uris.
        fn uris(uri: &str) -> HashMap<String, OwnedValue> {
            HashMap::from([("uris".to_string(), Value::from(vec![uri.to_string()]).try_to_owned().unwrap())])
        }

        /// The test portals' own names: they serve the FileChooser interface
        /// as the desktop's portal does, so that a test never reaches one.
        const TEST_PORTAL: &str = "org.frostyard.Frostmail.TestPortal";
        const LEAVING_PORTAL: &str = "org.frostyard.Frostmail.TestPortalLeaves";
        const ANSWERING_PORTAL: &str = "org.frostyard.Frostmail.TestPortalAnswersThenLeaves";

        /// WAIT is how long a test waits for a call before it fails it, so
        /// that a wait that never ends fails the test instead of hanging it.
        const WAIT: Duration = Duration::from_secs(10);

        /// Saving is one Save As's answer.
        type Saving = Result<Option<PathBuf>, String>;

        /// What one SaveFile was asked.
        #[derive(Debug, PartialEq)]
        struct Asked {
            title: String,
            current_name: String,
            filters: Vec<(String, Vec<(u32, String)>)>,
        }

        /// A file chooser portal that records each SaveFile and answers what
        /// the test configured, in order: the Response's code and its hints.
        /// A second connection forges a Response first, so that the app must
        /// take the portal's own and no other peer's.
        struct TestPortal {
            answers: Arc<Mutex<VecDeque<(u32, HashMap<String, OwnedValue>)>>>,
            asked: Arc<Mutex<Vec<Asked>>>,
            forger: zbus::Connection,
        }

        #[zbus::interface(name = "org.freedesktop.portal.FileChooser")]
        impl TestPortal {
            async fn save_file(
                &self,
                _parent_window: &str,
                title: &str,
                options: HashMap<String, OwnedValue>,
                #[zbus(header)] header: zbus::message::Header<'_>,
                #[zbus(connection)] connection: &zbus::Connection,
            ) -> OwnedObjectPath {
                let value = |key: &str| options.get(key).map(|v| Value::from(v.clone()));
                let text = |key: &str| value(key).and_then(|v| String::try_from(v).ok()).unwrap_or_default();
                let token = text("handle_token");
                let filters: Vec<(String, Vec<(u32, String)>)> =
                    value("filters").and_then(|v| v.try_into().ok()).unwrap_or_default();
                self.asked
                    .lock()
                    .unwrap()
                    .push(Asked { title: title.to_string(), current_name: text("current_name"), filters });
                let (code, hints) = self.answers.lock().unwrap().pop_front().expect("a configured answer");
                let sender = header.sender().map(|s| s.as_str().to_string()).unwrap_or_default();
                let path = OwnedObjectPath::try_from(request_path(&sender, &token)).expect("a valid request path");
                let destination = (!sender.is_empty()).then_some(sender.as_str());
                // A peer that is not the portal answers first, with another
                // file: the app must skip it and take the portal's.
                self.forger
                    .emit_signal(
                        destination,
                        path.as_str(),
                        REQUEST,
                        "Response",
                        &(0u32, uris("file:///home/u/Forged.eml")),
                    )
                    .await
                    .expect("the forged Response");
                connection
                    .emit_signal(destination, path.as_str(), REQUEST, "Response", &(code, hints))
                    .await
                    .expect("the Response signal");
                path
            }
        }

        /// A portal that leaves the session bus as it answers. With an answer,
        /// it sends that Response first, then leaves, then replies: the app
        /// must keep the answer. Without one, it sends no Response: the app
        /// must end its wait.
        struct LeavingPortal {
            name: &'static str,
            answer: Mutex<Option<(u32, HashMap<String, OwnedValue>)>>,
            notes: Mutex<Option<mpsc::Sender<()>>>,
        }

        #[zbus::interface(name = "org.freedesktop.portal.FileChooser")]
        impl LeavingPortal {
            async fn save_file(
                &self,
                _parent_window: &str,
                _title: &str,
                options: HashMap<String, OwnedValue>,
                #[zbus(header)] header: zbus::message::Header<'_>,
                #[zbus(connection)] connection: &zbus::Connection,
            ) -> OwnedObjectPath {
                let token = options
                    .get("handle_token")
                    .and_then(|v| String::try_from(Value::from(v.clone())).ok())
                    .unwrap_or_default();
                let sender = header.sender().map(|s| s.as_str().to_string()).unwrap_or_default();
                let path = OwnedObjectPath::try_from(request_path(&sender, &token)).expect("a valid request path");
                let answer = self.answer.lock().unwrap().take();
                if let Some(answer) = answer {
                    let destination = (!sender.is_empty()).then_some(sender.as_str());
                    connection
                        .emit_signal(destination, path.as_str(), REQUEST, "Response", &answer)
                        .await
                        .expect("the Response signal");
                }
                // It leaves before it replies, so the app has the leave
                // queued before the reply (the object stays served, so the
                // reply still goes out).
                connection.release_name(self.name).await.expect("the name is released");
                if let Some(notes) = self.notes.lock().unwrap().take() {
                    notes.send(()).expect("the test hears the call");
                }
                path
            }
        }

        /// leave serves a LeavingPortal under name, asks it for a file on a
        /// thread of its own, and gives the result, within WAIT.
        fn leave(name: &'static str, answer: Option<(u32, HashMap<String, OwnedValue>)>) -> Saving {
            let (notes, heard) = mpsc::channel();
            let portal = LeavingPortal { name, answer: Mutex::new(answer), notes: Mutex::new(Some(notes)) };
            let server = zbus::blocking::connection::Builder::session()
                .unwrap()
                .name(name)
                .unwrap()
                .serve_at(PORTAL_PATH, portal)
                .unwrap()
                .build()
                .unwrap();
            let (results, answered) = mpsc::channel();
            std::thread::spawn(move || {
                let run = tauri::async_runtime::block_on(async {
                    let conn = zbus::Connection::session().await.map_err(|e| format!("the session bus: {e}"))?;
                    save_file_on(&conn, name, "Save Message", "Out.eml", "Email Message", "eml").await
                });
                let _ = results.send(run);
            });
            heard.recv_timeout(WAIT).expect("the portal was asked");
            let result = answered.recv_timeout(WAIT).expect("the wait ended");
            drop(server);
            result
        }

        /// save_all asks the portal named destination the 3 answers on a
        /// thread of its own, so that a wait that never ends fails the test
        /// instead of hanging it.
        fn save_all(destination: &'static str) -> Receiver<Result<Vec<Saving>, String>> {
            let (results, answers) = mpsc::channel::<Result<Vec<Saving>, String>>();
            std::thread::spawn(move || {
                let run: Result<Vec<Saving>, String> = tauri::async_runtime::block_on(async {
                    let conn = zbus::Connection::session().await.map_err(|e| format!("the session bus: {e}"))?;
                    let mut asked = Vec::new();
                    for name in ["My Mail.eml", "My Mail.eml", "My Mail.eml"] {
                        let saving = save_file_on(&conn, destination, "Save Message", name, "Email Message", "eml");
                        asked.push(saving.await);
                    }
                    Ok(asked)
                });
                let _ = results.send(run);
            });
            answers
        }

        /// Run with a session bus, as `dbus-run-session -- cargo test` does;
        /// it skips without one.
        #[test]
        fn save_file_answers_through_a_portal_of_its_own() {
            if std::env::var_os("DBUS_SESSION_BUS_ADDRESS").is_none() {
                eprintln!("no session bus: skipped");
                return;
            }
            let answers = Arc::new(Mutex::new(VecDeque::from([
                (0, uris("file:///home/u/My%20Mail.eml")),
                (1, HashMap::new()),
                (2, HashMap::new()),
            ])));
            let asked: Arc<Mutex<Vec<Asked>>> = Arc::default();
            let forger = tauri::async_runtime::block_on(zbus::Connection::session()).expect("the session bus");
            let portal = TestPortal { answers: Arc::clone(&answers), asked: Arc::clone(&asked), forger };
            let _server = zbus::blocking::connection::Builder::session()
                .unwrap()
                .name(TEST_PORTAL)
                .unwrap()
                .serve_at(PORTAL_PATH, portal)
                .unwrap()
                .build()
                .unwrap();
            let results = save_all(TEST_PORTAL).recv_timeout(WAIT).expect("the portal answers each SaveFile");

            // A chosen file, then a cancel, then a failure, each from the
            // portal and not from the peer that forges one first.
            let results = results.expect("the 3 calls run");
            assert_eq!(results[0], Ok(Some(PathBuf::from("/home/u/My Mail.eml"))));
            assert_eq!(results[1], Ok(None), "a cancel writes nothing");
            assert!(results[2].is_err(), "a failure is an error");

            let asked = asked.lock().unwrap();
            assert_eq!(asked.len(), 3);
            assert_eq!(
                asked[0],
                Asked {
                    title: "Save Message".into(),
                    current_name: "My Mail.eml".into(),
                    filters: vec![("Email Message".into(), vec![(0, "*.eml".into())])],
                }
            );
        }

        /// The portal leaves the bus without an answer: the wait ends with an
        /// error and does not last.
        #[test]
        fn save_file_ends_when_the_portal_leaves_the_bus() {
            if std::env::var_os("DBUS_SESSION_BUS_ADDRESS").is_none() {
                eprintln!("no session bus: skipped");
                return;
            }
            assert_eq!(leave(LEAVING_PORTAL, None), Err("the file chooser portal left the session bus".to_string()));
        }

        /// The portal answers, then leaves, then replies: both are queued
        /// when the wait begins, and the answer wins.
        #[test]
        fn an_answer_sent_before_the_portal_leaves_is_kept() {
            if std::env::var_os("DBUS_SESSION_BUS_ADDRESS").is_none() {
                eprintln!("no session bus: skipped");
                return;
            }
            let answer = (0, uris("file:///home/u/Out.eml"));
            assert_eq!(leave(ANSWERING_PORTAL, Some(answer)), Ok(Some(PathBuf::from("/home/u/Out.eml"))));
        }

        #[test]
        fn only_the_portals_answer_on_the_request_is_ours() {
            let (path, handle) = ("/r/1_42/frostmail0", "/r/1_42/old");
            assert!(ours(Some(":1.7"), Some(":1.7"), path, path, handle));
            assert!(ours(Some(":1.7"), Some(":1.7"), handle, path, handle), "an old portal's own path");
            assert!(!ours(Some(":1.9"), Some(":1.7"), path, path, handle), "another peer forges it");
            assert!(!ours(None, Some(":1.7"), path, path, handle), "no sender");
            assert!(!ours(None, None, path, path, handle), "no sender on either side");
            assert!(!ours(Some(":1.7"), Some(":1.7"), "/r/1_42/other", path, handle), "another request");
        }
    }
}
