use std::{
    env,
    fs,
    io::{self, Read, Write},
    net::{SocketAddr, TcpStream, ToSocketAddrs},
    path::{Path, PathBuf},
    time::Duration,
};

use serde::Serialize;
use serde_json::{json, Value};
use std::sync::{Arc, Mutex};
use tauri::{menu::{Menu, MenuItem}, tray::TrayIconBuilder, Manager, Window};

struct ClickThroughState(Arc<Mutex<bool>>);

fn next_click_through(current: bool) -> bool { !current }

const REQUEST_TIMEOUT: Duration = Duration::from_secs(2);
const MAX_RESPONSE_BYTES: usize = 128 * 1024;
const DEFAULT_SUPERVISOR_HOST: &str = "127.0.0.1";
const DEFAULT_SUPERVISOR_PORT: u16 = 17321;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum NativeErrorKind {
    Timeout,
    Unauthorized,
    Unavailable,
    InvalidResponse,
}

impl NativeErrorKind {
    fn as_str(self) -> &'static str {
        match self {
            Self::Timeout => "timeout",
            Self::Unauthorized => "unauthorized",
            Self::Unavailable => "unavailable",
            Self::InvalidResponse => "invalid_response",
        }
    }
}

#[derive(Serialize)]
struct SupervisorSnapshot {
    connected: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    semantic: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    last_success_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    last_error_kind: Option<&'static str>,
}

fn disconnected(kind: NativeErrorKind) -> SupervisorSnapshot {
    SupervisorSnapshot {
        connected: false,
        semantic: None,
        last_success_at: None,
        last_error_kind: Some(kind.as_str()),
    }
}

fn runtime_root() -> PathBuf {
    for variable in ["STUDYGUARDIAN_RUNTIME_DIR", "STUDYGUARDIAN_ROOT"] {
        if let Some(value) = env::var_os(variable) {
            let path = PathBuf::from(value);
            if !path.as_os_str().is_empty() {
                return path;
            }
        }
    }
    PathBuf::from(r"D:\StudyGuardianDev")
}

fn config_scalar(config: &str, key: &str) -> Option<String> {
    config.lines().find_map(|line| {
        let value = line.trim().strip_prefix(key)?.strip_prefix(':')?.trim();
        let value = value.split('#').next()?.trim();
        if value.is_empty() {
            return None;
        }
        Some(value.trim_matches(|character| character == '"' || character == '\'').to_string())
    })
}

fn runtime_endpoint(root: &Path) -> (String, u16) {
    let config_path = root.join("config").join("config.yaml");
    let Ok(config) = fs::read_to_string(config_path) else {
        return (DEFAULT_SUPERVISOR_HOST.to_string(), DEFAULT_SUPERVISOR_PORT);
    };
    let host = config_scalar(&config, "supervisor_host")
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| DEFAULT_SUPERVISOR_HOST.to_string());
    let port = config_scalar(&config, "supervisor_port")
        .and_then(|value| value.parse::<u16>().ok())
        .filter(|value| *value != 0)
        .unwrap_or(DEFAULT_SUPERVISOR_PORT);
    (host, port)
}

fn map_io_error(error: &io::Error) -> NativeErrorKind {
    match error.kind() {
        io::ErrorKind::TimedOut | io::ErrorKind::WouldBlock => NativeErrorKind::Timeout,
        _ => NativeErrorKind::Unavailable,
    }
}

fn loopback_address(host: &str, port: u16) -> Result<SocketAddr, NativeErrorKind> {
    let mut addresses = (host.trim(), port)
        .to_socket_addrs()
        .map_err(|_| NativeErrorKind::Unavailable)?;
    addresses
        .find(|address| address.ip().is_loopback())
        .ok_or(NativeErrorKind::Unavailable)
}

fn find_bytes(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack.windows(needle.len()).position(|window| window == needle)
}

fn classify_http_status(status: u16) -> Result<(), NativeErrorKind> {
    match status {
        200..=299 => Ok(()),
        401 | 403 => Err(NativeErrorKind::Unauthorized),
        _ => Err(NativeErrorKind::Unavailable),
    }
}

fn parse_http_response(response: &[u8]) -> Result<Value, NativeErrorKind> {
    let separator = find_bytes(response, b"\r\n\r\n").ok_or(NativeErrorKind::InvalidResponse)?;
    let header = &response[..separator];
    let status_line_end = find_bytes(header, b"\r\n").unwrap_or(header.len());
    let status_line = std::str::from_utf8(&header[..status_line_end])
        .map_err(|_| NativeErrorKind::InvalidResponse)?;
    let status = status_line
        .split_whitespace()
        .nth(1)
        .and_then(|value| value.parse::<u16>().ok())
        .ok_or(NativeErrorKind::InvalidResponse)?;
    classify_http_status(status)?;
    serde_json::from_slice(&response[separator + 4..])
        .map_err(|_| NativeErrorKind::InvalidResponse)
}

fn fetch_current_activity(host: &str, port: u16, token: &str) -> Result<Value, NativeErrorKind> {
    if token.is_empty() || token.contains('\r') || token.contains('\n') {
        return Err(NativeErrorKind::Unauthorized);
    }
    let address = loopback_address(host, port)?;
    let mut stream = TcpStream::connect_timeout(&address, REQUEST_TIMEOUT)
        .map_err(|error| map_io_error(&error))?;
    stream
        .set_read_timeout(Some(REQUEST_TIMEOUT))
        .map_err(|error| map_io_error(&error))?;
    stream
        .set_write_timeout(Some(REQUEST_TIMEOUT))
        .map_err(|error| map_io_error(&error))?;

    let request = format!(
        "GET /v1/activity/current HTTP/1.1\r\nHost: {host}\r\nAuthorization: Bearer {token}\r\nConnection: close\r\n\r\n"
    );
    stream
        .write_all(request.as_bytes())
        .map_err(|error| map_io_error(&error))?;
    stream.flush().map_err(|error| map_io_error(&error))?;

    let mut response = Vec::with_capacity(16 * 1024);
    let mut chunk = [0_u8; 8192];
    loop {
        match stream.read(&mut chunk) {
            Ok(0) => break,
            Ok(read) => {
                if response.len() + read > MAX_RESPONSE_BYTES {
                    return Err(NativeErrorKind::InvalidResponse);
                }
                response.extend_from_slice(&chunk[..read]);
            }
            Err(error) => return Err(map_io_error(&error)),
        }
    }
    parse_http_response(&response)
}

fn enum_field(object: &serde_json::Map<String, Value>, key: &str, allowed: &[&str]) -> Result<String, NativeErrorKind> {
    let value = object
        .get(key)
        .and_then(Value::as_str)
        .filter(|value| allowed.contains(value))
        .ok_or(NativeErrorKind::InvalidResponse)?;
    Ok(value.to_string())
}

fn sanitize_semantic(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    if object.get("schema_version").and_then(Value::as_u64) != Some(1) {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let observed_at = object
        .get("observed_at")
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty() && value.len() <= 128)
        .ok_or(NativeErrorKind::InvalidResponse)?
        .to_string();
    let fresh = object
        .get("fresh")
        .and_then(Value::as_bool)
        .ok_or(NativeErrorKind::InvalidResponse)?;
    let user_mode = enum_field(object, "user_mode", &["STANDBY", "STUDY", "BREAK", "OFF"])?;
    let task = object
        .get("task")
        .and_then(Value::as_str)
        .filter(|value| value.len() <= 4096)
        .ok_or(NativeErrorKind::InvalidResponse)?
        .to_string();
    let interaction = enum_field(object, "interaction", &["ACTIVE", "IDLE_STATIC", "IDLE_DYNAMIC", "UNKNOWN"])?;
    let relation = enum_field(object, "relation", &["FOCUSED", "DISTRACTED", "UNKNOWN"])?;
    let privacy = enum_field(object, "privacy", &["NORMAL", "SENSITIVE"])?;
    let activity = enum_field(
        object,
        "activity",
        &[
            "CODING", "ALGORITHM", "READING", "WRITING", "WATCHING", "AI_ASSISTED", "BROWSING",
            "GENERAL_STUDY", "UNKNOWN",
        ],
    )?;
    let confidence = object
        .get("confidence")
        .and_then(Value::as_f64)
        .filter(|value| value.is_finite() && (0.0..=1.0).contains(value))
        .ok_or(NativeErrorKind::InvalidResponse)?;

    Ok(json!({
        "schema_version": 1,
        "observed_at": observed_at,
        "fresh": fresh,
        "user_mode": user_mode,
        "task": task,
        "interaction": interaction,
        "relation": relation,
        "privacy": privacy,
        "activity": activity,
        "confidence": confidence,
    }))
}

#[tauri::command]
fn supervisor_snapshot() -> SupervisorSnapshot {
    let root = runtime_root();
    let token_path = root.join("config").join("auth.token");
    let token = match fs::read_to_string(token_path) {
        Ok(token) => token.trim().to_string(),
        Err(_) => return disconnected(NativeErrorKind::Unavailable),
    };
    let (host, port) = runtime_endpoint(&root);
    let raw = match fetch_current_activity(&host, port, &token) {
        Ok(value) => value,
        Err(kind) => return disconnected(kind),
    };
    let semantic = match sanitize_semantic(&raw) {
        Ok(value) => value,
        Err(kind) => return disconnected(kind),
    };
    let last_success_at = semantic
        .get("observed_at")
        .and_then(Value::as_str)
        .map(str::to_string);
    SupervisorSnapshot {
        connected: true,
        semantic: Some(semantic),
        last_success_at,
        last_error_kind: None,
    }
}

#[tauri::command]
fn set_click_through(window: Window, state: tauri::State<'_, ClickThroughState>, enabled: bool) -> Result<bool, String> {
    window.set_ignore_cursor_events(enabled).map_err(|err| err.to_string())?;
    *state.0.lock().map_err(|_| "click-through state poisoned".to_string())? = enabled;
    Ok(enabled)
}

pub fn run() {
    tauri::Builder::default()
        .manage(ClickThroughState(Arc::new(Mutex::new(false))))
        .invoke_handler(tauri::generate_handler![set_click_through, supervisor_snapshot])
        .setup(|app| {
            let window = app.get_webview_window("main").ok_or("main window missing")?;
            window.set_always_on_top(true)?;
            let toggle = MenuItem::with_id(app, "toggle-click-through", "切换鼠标穿透", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "退出 Pet", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&toggle, &quit])?;
            let tray_state = app.state::<ClickThroughState>().inner().0.clone();
            let mut tray_builder = TrayIconBuilder::new().menu(&menu);
            if let Some(icon) = app.default_window_icon() {
                tray_builder = tray_builder.icon(icon.clone());
            }
            let _tray = tray_builder
                .on_menu_event(move |app, event| {
                    match event.id.as_ref() {
                        "toggle-click-through" => {
                            if let Some(window) = app.get_webview_window("main") {
                                if let Ok(mut current) = tray_state.lock() {
                                    let next = next_click_through(*current);
                                    if window.set_ignore_cursor_events(next).is_ok() { *current = next; }
                                }
                            }
                        }
                        "quit" => app.exit(0),
                        _ => {}
                    }
                })
                .build(app)?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running StudyGuardian Pet v3");
}

#[cfg(test)]
mod tests {
    use std::io;

    use serde_json::json;

    use super::{
        classify_http_status, disconnected, map_io_error, parse_http_response, sanitize_semantic,
        next_click_through, NativeErrorKind, SupervisorSnapshot,
    };

    #[test]
    fn tray_toggle_is_reversible_for_recovery() {
        assert!(next_click_through(false));
        assert!(!next_click_through(next_click_through(false)));
    }

    #[test]
    fn native_snapshot_serialization_never_contains_token_or_extra_fields() {
        let semantic = sanitize_semantic(&json!({
            "schema_version": 1,
            "observed_at": "2026-09-04T00:00:00Z",
            "fresh": true,
            "user_mode": "STUDY",
            "task": "unit test",
            "interaction": "ACTIVE",
            "relation": "FOCUSED",
            "privacy": "NORMAL",
            "activity": "CODING",
            "confidence": 0.9,
            "token": "must-never-cross-boundary",
        }))
        .expect("valid semantic contract");
        let snapshot = SupervisorSnapshot {
            connected: true,
            semantic: Some(semantic),
            last_success_at: Some("2026-09-04T00:00:00Z".to_string()),
            last_error_kind: None,
        };
        let encoded = serde_json::to_string(&snapshot).expect("snapshot serializes");
        assert!(!encoded.contains("must-never-cross-boundary"));
        assert!(!encoded.contains("auth.token"));
        assert!(!encoded.contains("last_error_kind"));
    }

    #[test]
    fn native_http_error_mapping_is_bounded() {
        assert_eq!(classify_http_status(401), Err(NativeErrorKind::Unauthorized));
        assert_eq!(classify_http_status(504), Err(NativeErrorKind::Unavailable));
        assert_eq!(map_io_error(&io::Error::new(io::ErrorKind::TimedOut, "hidden detail")), NativeErrorKind::Timeout);
        assert_eq!(map_io_error(&io::Error::new(io::ErrorKind::ConnectionRefused, "hidden detail")), NativeErrorKind::Unavailable);
    }

    #[test]
    fn malformed_http_json_is_invalid_response() {
        let response = b"HTTP/1.1 200 OK\r\nContent-Length: 8\r\n\r\n{broken}";
        assert_eq!(parse_http_response(response), Err(NativeErrorKind::InvalidResponse));
    }

    #[test]
    fn semantic_sanitizer_keeps_only_frontend_contract() {
        let raw = json!({
            "schema_version": 1,
            "observed_at": "2026-09-04T00:00:00Z",
            "fresh": true,
            "user_mode": "STUDY",
            "task": "unit test",
            "interaction": "ACTIVE",
            "relation": "FOCUSED",
            "privacy": "NORMAL",
            "activity": "CODING",
            "confidence": 0.9,
            "secret": "must be dropped",
        });
        let sanitized = sanitize_semantic(&raw).expect("valid semantic contract");
        assert!(sanitized.get("secret").is_none());
        assert_eq!(sanitized.as_object().expect("object").len(), 10);
        assert_eq!(disconnected(NativeErrorKind::Timeout).last_error_kind, Some("timeout"));
    }
}
