//! One call to maild over an open connection (docs/specs/rpc-protocol.md):
//! rpc.hello pipelined with the call, then the responses, read until the
//! call's.

use std::io::{BufRead, Write};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use serde_json::{Value, json};

/// The protocol major version (api.Protocol).
const PROTOCOL: i64 = 1;

/// Call method with params and wait for its result.
pub fn call(reader: impl BufRead, mut writer: impl Write, method: &str, params: Value) -> Result<Value, String> {
    let client = concat!("frostmail-notify/", env!("CARGO_PKG_VERSION"));
    let mut out = Vec::new();
    for (id, method, params) in [(1, "rpc.hello", json!({"protocol": PROTOCOL, "client": client})), (2, method, params)] {
        serde_json::to_writer(&mut out, &json!({"jsonrpc": "2.0", "id": id, "method": method, "params": params}))
            .map_err(|e| e.to_string())?;
        out.push(b'\n');
    }
    writer.write_all(&out).and_then(|()| writer.flush()).map_err(|e| format!("write to maild: {e}"))?;
    for line in reader.lines() {
        let line = line.map_err(|e| format!("read from maild: {e}"))?;
        let msg: Value = serde_json::from_str(&line).map_err(|e| format!("maild's answer: {e}"))?;
        let (id, what) = match msg.get("id").and_then(Value::as_i64) {
            Some(1) => (1, "rpc.hello"),
            Some(2) => (2, method),
            _ => continue, // an event or another answer
        };
        if let Some(err) = msg.get("error") {
            let text = err.get("message").and_then(Value::as_str).unwrap_or("an error");
            return Err(format!("{what}: {text}"));
        }
        if id == 2 {
            return Ok(msg.get("result").cloned().unwrap_or(Value::Null));
        }
    }
    Err(format!("{method}: maild closed the connection"))
}

/// calendar.snooze's params: ten minutes from now, at the minute, as
/// maild's own Snooze does.
pub fn snooze(id: &str, now: SystemTime) -> Value {
    let secs = (now + Duration::from_secs(10 * 60)).duration_since(UNIX_EPOCH).map_or(0, |d| d.as_secs());
    json!({"ids": [id], "until": rfc3339(secs - secs % 60)})
}

/// calendar.dismiss's params.
pub fn dismiss(id: &str) -> Value {
    json!({"ids": [id]})
}

/// Seconds since the epoch as an RFC 3339 time in UTC.
fn rfc3339(secs: u64) -> String {
    let days = (secs / 86400) as i64;
    let rem = secs % 86400;
    // Howard Hinnant's civil_from_days.
    let z = days + 719_468;
    let era = z.div_euclid(146_097);
    let doe = z.rem_euclid(146_097);
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146_096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let day = doy - (153 * mp + 2) / 5 + 1;
    let month = if mp < 10 { mp + 3 } else { mp - 9 };
    let year = yoe + era * 400 + i64::from(month <= 2);
    format!("{year:04}-{month:02}-{day:02}T{:02}:{:02}:{:02}Z", rem / 3600, rem / 60 % 60, rem % 60)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn run(answers: &str, method: &str) -> (Result<Value, String>, Vec<Value>) {
        let mut sent = Vec::new();
        let result = call(answers.as_bytes(), &mut sent, method, json!({"ids": ["r1"]}));
        let sent = sent.split(|&b| b == b'\n').filter(|l| !l.is_empty()).map(|l| serde_json::from_slice(l).unwrap()).collect();
        (result, sent)
    }

    #[test]
    fn hello_then_the_call() {
        let answers = concat!(
            r#"{"jsonrpc":"2.0","id":1,"result":{"protocol":1}}"#, "\n",
            r#"{"jsonrpc":"2.0","method":"event","params":{"event":"calendar.changed","data":{}}}"#, "\n",
            r#"{"jsonrpc":"2.0","id":2,"result":null}"#, "\n",
        );
        let (result, sent) = run(answers, "calendar.dismiss");
        assert_eq!(result, Ok(Value::Null));
        assert_eq!(sent.len(), 2);
        assert_eq!(sent[0]["method"], "rpc.hello");
        assert_eq!(sent[0]["params"]["protocol"], 1);
        assert_eq!(sent[1], json!({"jsonrpc": "2.0", "id": 2, "method": "calendar.dismiss", "params": {"ids": ["r1"]}}));
    }

    #[test]
    fn answers_may_come_in_any_order() {
        let answers = concat!(
            r#"{"jsonrpc":"2.0","id":2,"result":{"ok":true}}"#, "\n",
            r#"{"jsonrpc":"2.0","id":1,"result":{"protocol":1}}"#, "\n",
        );
        assert_eq!(run(answers, "calendar.snooze").0, Ok(json!({"ok": true})));
    }

    #[test]
    fn errors() {
        let hello = r#"{"jsonrpc":"2.0","id":1,"error":{"code":1003,"message":"protocol mismatch"}}"#;
        assert_eq!(run(hello, "calendar.dismiss").0, Err("rpc.hello: protocol mismatch".into()));
        let call = concat!(
            r#"{"jsonrpc":"2.0","id":1,"result":{}}"#, "\n",
            r#"{"jsonrpc":"2.0","id":2,"error":{"code":1001,"message":"no such reminder"}}"#, "\n",
        );
        assert_eq!(run(call, "calendar.dismiss").0, Err("calendar.dismiss: no such reminder".into()));
        assert!(run(r#"{"jsonrpc":"2.0","id":1,"result":{}}"#, "calendar.dismiss").0.unwrap_err().contains("closed"));
        assert!(run("not json\n", "calendar.dismiss").0.is_err());
    }

    #[test]
    fn snooze_is_ten_minutes_at_the_minute() {
        let now = UNIX_EPOCH + Duration::from_secs(1_792_142_059); // 2026-10-16T09:14:19Z
        assert_eq!(snooze("r1", now), json!({"ids": ["r1"], "until": "2026-10-16T09:24:00Z"}));
    }

    #[test]
    fn times() {
        assert_eq!(rfc3339(0), "1970-01-01T00:00:00Z");
        assert_eq!(rfc3339(951_782_400), "2000-02-29T00:00:00Z");
        assert_eq!(rfc3339(1_798_761_599), "2026-12-31T23:59:59Z");
        assert_eq!(rfc3339(4_107_542_400), "2100-03-01T00:00:00Z");
    }
}
