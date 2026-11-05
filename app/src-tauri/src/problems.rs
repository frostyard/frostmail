//! Problem records of the shell (docs/adr/0049-record-problems-in-the-local-journal.md):
//! its panics and a few coded errors go to the local systemd journal through
//! frostyard/std's crate `frostyard-problems`, which sends nothing over a
//! network. main installs the panic hook; bridge.rs records
//! [`SOCKET_LOST`].

use frostyard_problems::{Component, Recorder};

/// The app ID of every Frostmail record.
pub const APP_ID: &str = "org.frostyard.Frostmail";

/// The version of a build that no release stamped: tauri.conf.json keeps it
/// in git, and packaging/flatpak/stamp-version.sh replaces it on a v* tag.
const UNSTAMPED: &str = "0.0.0";

/// maild's local socket ended under the bridge: maild closed it, or a read
/// from it failed.
pub const SOCKET_LOST: &str = "bridge.socket_lost";

/// The shell's recorder for the app's version, as Tauri's package_info
/// gives it from tauri.conf.json. An unstamped build (0.0.0) gets an inert
/// recorder: it never claims a version that no release has. Any other
/// version that is not a release semver is inert too (the crate's rule).
pub fn recorder(version: &str) -> Recorder {
    if version == UNSTAMPED {
        // Not a semver, so the crate makes the recorder inert, and its err()
        // says why.
        return Recorder::new(APP_ID, "unstamped 0.0.0", Component::Shell);
    }
    Recorder::new(APP_ID, version, Component::Shell)
}

/// A journal socket for tests: a datagram socket in a folder of its own,
/// which drop removes.
#[cfg(all(test, target_os = "linux"))]
pub(crate) mod testing {
    use std::collections::HashMap;
    use std::io::ErrorKind;
    use std::os::unix::net::UnixDatagram;
    use std::path::PathBuf;
    use std::sync::atomic::{AtomicUsize, Ordering};

    use frostyard_problems::{Detail, decode_entry, validate};

    pub(crate) struct Journal {
        dir: PathBuf,
        pub(crate) path: PathBuf,
        sock: UnixDatagram,
    }

    impl Journal {
        pub(crate) fn new() -> Journal {
            static COUNT: AtomicUsize = AtomicUsize::new(0);
            let dir = std::env::temp_dir().join(format!(
                "frostmail-journal-{}-{}",
                std::process::id(),
                COUNT.fetch_add(1, Ordering::SeqCst)
            ));
            let _ = std::fs::remove_dir_all(&dir);
            std::fs::create_dir(&dir).unwrap();
            let path = dir.join("socket");
            let sock = UnixDatagram::bind(&path).unwrap();
            sock.set_nonblocking(true).unwrap();
            Journal { dir, path, sock }
        }

        /// Every record waiting in the socket, decoded and validated. A
        /// recorder sends before its call returns, so a read right after
        /// the code under test sees all it wrote.
        pub(crate) fn records(&self) -> Vec<(HashMap<String, String>, Detail)> {
            let mut out = Vec::new();
            let mut buf = vec![0u8; 1 << 16];
            loop {
                match self.sock.recv(&mut buf) {
                    Ok(n) => {
                        let fields = decode_entry(&buf[..n]).expect("decode the record");
                        let detail = validate(&fields).expect("a valid record");
                        out.push((fields, detail));
                    }
                    Err(e) if e.kind() == ErrorKind::WouldBlock => return out,
                    Err(e) => panic!("read the test journal: {e}"),
                }
            }
        }
    }

    impl Drop for Journal {
        fn drop(&mut self) {
            let _ = std::fs::remove_dir_all(&self.dir);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{APP_ID, recorder};

    #[test]
    fn an_unstamped_build_records_nothing() {
        let rec = recorder("0.0.0");
        assert!(rec.err().is_some_and(|why| why.contains("not a release semver")), "{:?}", rec.err());
        for version in ["0.4.0-dev", "v0.4.0", ""] {
            assert!(recorder(version).err().is_some(), "{version} must give an inert recorder");
        }
        for version in ["0.4.0", "0.4.1-rc.2", "1.0.0"] {
            assert_eq!(recorder(version).err(), None, "{version} must record");
        }
        assert_eq!(APP_ID, "org.frostyard.Frostmail");
    }

    #[cfg(target_os = "linux")]
    mod panics {
        use std::env;
        use std::process::Command;

        use super::super::recorder;
        use crate::problems::testing::Journal;

        /// Makes a run of the test binary the helper that panics, writing
        /// to this socket.
        const SOCKET_ENV: &str = "FROSTMAIL_PROBLEMS_TEST_SOCKET";

        /// The line of the panic in `boom`.
        const BOOM_LINE: u32 = line!() + 3;
        #[inline(never)]
        fn boom() {
            panic!("frostmail test: a panic in the shell");
        }

        /// The hook that main installs records a panic of the shell with
        /// its message and its location in the shell's crate, and the
        /// process still dies of it.
        #[test]
        fn a_panic_records_its_location() {
            if let Some(socket) = env::var_os(SOCKET_ENV) {
                let rec = recorder("1.2.3").with_socket(socket);
                rec.install_panic_hook(env!("CARGO_MANIFEST_DIR"));
                boom();
                return;
            }
            let journal = Journal::new();
            let out = Command::new(env::current_exe().unwrap())
                .args(["--exact", "problems::tests::panics::a_panic_records_its_location", "--nocapture", "--test-threads=1"])
                .env(SOCKET_ENV, &journal.path)
                .output()
                .unwrap();
            let stdout = String::from_utf8_lossy(&out.stdout);
            let stderr = String::from_utf8_lossy(&out.stderr);
            assert!(
                !out.status.success() && stdout.contains("running 1 test") && stderr.contains("frostmail test: a panic in the shell"),
                "the helper must run 1 test and panic: {stdout}\n{stderr}"
            );
            let records = journal.records();
            assert_eq!(records.len(), 1, "records of the helper");
            let (fields, detail) = &records[0];
            let location = format!("{}:{BOOM_LINE}", file!());
            assert_eq!(location, format!("src/problems.rs:{BOOM_LINE}"));
            assert_eq!(fields["MESSAGE"], "frostmail test: a panic in the shell");
            assert_eq!(
                (detail.app_id.as_str(), detail.app_version.as_str(), detail.runtime.as_str()),
                ("org.frostyard.Frostmail", "1.2.3", "rust")
            );
            assert_eq!((detail.component.as_str(), detail.class.as_str()), ("shell", "panic"));
            assert_eq!(detail.code, None);
            assert_eq!(detail.location.as_deref(), Some(location.as_str()));
        }
    }
}
