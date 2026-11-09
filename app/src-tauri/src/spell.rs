//! Spelling checked while the user writes (docs/specs/compose-ui.md,
//! Spelling). WebKitGTK checks no spelling until the app turns it on in the
//! web context, with the languages to check; WebView2 on Windows follows the
//! page's `spellcheck` attributes alone, so this module does nothing there.

/// Turn on spell checking in the window's web context, in the system's
/// languages. All of the app's windows share one context; setting it again
/// for each window keeps every window right without depending on that.
#[cfg(target_os = "linux")]
pub(crate) fn enable(window: &tauri::WebviewWindow) {
    let names: Vec<String> = webkit2gtk::glib::language_names().iter().map(|n| n.to_string()).collect();
    let languages = languages(&names);
    let _ = window.with_webview(move |webview| {
        use webkit2gtk::{WebContextExt, WebViewExt};
        if let Some(context) = webview.inner().context() {
            let languages: Vec<&str> = languages.iter().map(String::as_str).collect();
            context.set_spell_checking_languages(&languages);
            context.set_spell_checking_enabled(true);
        }
    });
}

#[cfg(not(target_os = "linux"))]
pub(crate) fn enable(_window: &tauri::WebviewWindow) {}

/// The language the checker uses when the system names none.
#[cfg_attr(not(any(target_os = "linux", test)), allow(dead_code))]
const FALLBACK: &str = "en_US";

/// The spelling languages for the system's locale names, as
/// `g_get_language_names` gives them, most preferred first: each name
/// without its encoding (`.UTF-8`) or modifier (`@euro`), without `C` and
/// `POSIX`, and once only, in the order given; `en_US` when none is left.
#[cfg_attr(not(any(target_os = "linux", test)), allow(dead_code))]
fn languages<S: AsRef<str>>(names: &[S]) -> Vec<String> {
    let mut out: Vec<String> = Vec::new();
    for name in names {
        let name = name.as_ref();
        let name = name.split(['.', '@']).next().unwrap_or_default();
        let valid = !name.is_empty() && name.chars().all(|c| c.is_ascii_alphanumeric() || c == '_' || c == '-');
        if !valid || name == "C" || name == "POSIX" || out.iter().any(|n| n == name) {
            continue;
        }
        out.push(name.to_string());
    }
    if out.is_empty() {
        out.push(FALLBACK.to_string());
    }
    out
}

#[cfg(test)]
mod tests {
    use super::languages;

    #[test]
    fn the_locale_names_without_encoding_or_c() {
        assert_eq!(languages(&["en_US.UTF-8", "en_US", "en.UTF-8", "en", "C"]), ["en_US", "en"]);
        assert_eq!(languages(&["en_US.UTF-8", "en_US", "en", "C"]), ["en_US", "en"]);
    }

    #[test]
    fn en_us_when_the_system_names_no_language() {
        assert_eq!(languages(&["C"]), ["en_US"]);
        assert_eq!(languages(&["C.UTF-8", "C", "POSIX"]), ["en_US"]);
        assert_eq!(languages::<&str>(&[]), ["en_US"]);
    }

    #[test]
    fn a_modifier_is_dropped() {
        assert_eq!(languages(&["de_DE@euro", "de_DE", "de@euro", "de", "C"]), ["de_DE", "de"]);
        assert_eq!(languages(&["de_DE.UTF-8@euro"]), ["de_DE"]);
    }

    #[test]
    fn several_languages_keep_their_order() {
        assert_eq!(
            languages(&["fr_CA.UTF-8", "fr_CA", "fr", "en_GB.UTF-8", "en_GB", "en", "C"]),
            ["fr_CA", "fr", "en_GB", "en"]
        );
    }
}
