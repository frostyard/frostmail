//! M0 spike support (docs/plans/0001-m0-foundations.md): the spike page
//! reports its checks here so a run can be judged from stdout. Removed with
//! the spike page in M2.

use tauri::AppHandle;

/// Print the report as one line; exit when FROSTMAIL_SPIKE_EXIT=1.
#[tauri::command]
pub fn spike_report(app: AppHandle, report: serde_json::Value) {
    println!("SPIKE-REPORT {report}");
    if std::env::var_os("FROSTMAIL_SPIKE_EXIT").is_some_and(|v| v == "1") {
        app.exit(0);
    }
}
