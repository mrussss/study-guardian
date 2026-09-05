use std::{
    env,
    fs,
    fs::OpenOptions,
    io::{self, Read, Write},
    net::{SocketAddr, TcpStream, ToSocketAddrs},
    path::{Path, PathBuf},
    time::Duration,
};

use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::sync::{Arc, Mutex};
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    AppHandle, Emitter, Manager, PhysicalPosition, WebviewWindow, WebviewWindowBuilder, Window, WindowEvent,
};

struct ClickThroughState(Arc<Mutex<bool>>);
struct ControlCenterRouteState(Arc<Mutex<String>>);
// Only auxiliary-window workers acquire this mutex; the UI thread never does.
struct AuxiliaryWindowState(Arc<Mutex<()>>);

enum AuxiliaryWindowAction {
    QuickPanel,
    ControlCenter(String),
    HideQuickPanel(&'static str),
    HideControlCenter,
}

#[derive(Serialize)]
struct NativeWindowDiagnostic {
    label: &'static str,
    exists: bool,
    visible: bool,
    focused: bool,
}

#[derive(Serialize)]
struct PetWindowDiagnostics {
    windows: Vec<NativeWindowDiagnostic>,
    control_center_route: String,
}

fn next_click_through(current: bool) -> bool { !current }

const REQUEST_TIMEOUT: Duration = Duration::from_secs(2);
const MAX_RESPONSE_BYTES: usize = 128 * 1024;
const SUPERVISOR_GET_PATHS: &[&str] = &[
    "/v1/activity/current",
    "/v1/status",
    "/v1/task-presets",
    "/v1/settings/reminder",
    "/v1/settings/ai",
    "/v1/motivation/status",
    "/v1/motivation/settings",
    "/v1/motivation/history?days=7",
    "/v1/motivation/achievements",
    "/v1/missions",
    "/v1/rewards",
    "/v1/ai/status",
    "/v1/review/daily",
];
const DEFAULT_SUPERVISOR_HOST: &str = "127.0.0.1";
const DEFAULT_SUPERVISOR_PORT: u16 = 17321;
const CONTROL_CENTER_ROUTE_EVENT: &str = "studyguardian://control-center-route";
const CONTROL_CENTER_ROUTES: &[&str] = &[
    "overview",
    "missions",
    "achievements",
    "rewards",
    "review",
    "history",
    "settings",
    "system",
];

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum NativeErrorKind {
    Timeout,
    Unauthorized,
    Rejected,
    Unavailable,
    InvalidResponse,
}

impl NativeErrorKind {
    fn as_str(self) -> &'static str {
        match self {
            Self::Timeout => "timeout",
            Self::Unauthorized => "unauthorized",
            Self::Rejected => "rejected",
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

#[derive(Serialize)]
struct SupervisorControlResult {
    ok: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    error_kind: Option<&'static str>,
}

#[derive(Serialize)]
struct SupervisorDashboardSnapshot {
    connected: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    status: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    motivation: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    task_presets: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    reminder_settings: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    ai_settings: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    history: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    achievements: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    missions: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    rewards: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    ai: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    review: Option<Value>,
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

fn disconnected_dashboard(kind: NativeErrorKind) -> SupervisorDashboardSnapshot {
    SupervisorDashboardSnapshot {
        connected: false,
        status: None,
        motivation: None,
        task_presets: None,
        reminder_settings: None,
        ai_settings: None,
        history: None,
        achievements: None,
        missions: None,
        rewards: None,
        ai: None,
        review: None,
        last_error_kind: Some(kind.as_str()),
    }
}

fn configured_aux_window(app: &AppHandle, label: &str) -> Result<WebviewWindow, String> {
    let config = app
        .config()
        .app
        .windows
        .iter()
        .find(|window| window.label == label)
        .cloned()
        .ok_or_else(|| format!("window configuration missing: {label}"))?;
    let is_quick_panel = label == "quick-panel";
    let window = WebviewWindowBuilder::from_config(app, &config)
        .map_err(|error| format!("window configuration rejected: {error}"))?
        .build()
        .map_err(|error| format!("window creation failed: {error}"))?;
    if is_quick_panel {
        record_quick_panel_debug_event("quick-panel:created");
    }
    let window_for_events = window.clone();
    window.on_window_event(move |event| match event {
            WindowEvent::CloseRequested { api, .. } => {
                api.prevent_close();
                let action = if is_quick_panel {
                    AuxiliaryWindowAction::HideQuickPanel("quick-panel:hide-reason:close-request")
                } else {
                    AuxiliaryWindowAction::HideControlCenter
                };
                schedule_aux_window(window_for_events.app_handle().clone(), action);
            }
            WindowEvent::Focused(true) if is_quick_panel => {
                record_quick_panel_debug_event("quick-panel:focused-true");
            }
            WindowEvent::Focused(false) if is_quick_panel => {
                record_quick_panel_debug_event("quick-panel:focused-false");
            }
            _ => {}
        });
    Ok(window)
}

fn bounded_panel_position(
    desired_x: i64,
    desired_y: i64,
    work_x: i64,
    work_y: i64,
    work_width: i64,
    work_height: i64,
    panel_width: i64,
    panel_height: i64,
) -> (i32, i32) {
    let max_x = (work_x + work_width - panel_width).max(work_x);
    let max_y = (work_y + work_height - panel_height).max(work_y);
    (desired_x.clamp(work_x, max_x) as i32, desired_y.clamp(work_y, max_y) as i32)
}

fn position_quick_panel(pet: &WebviewWindow, panel: &WebviewWindow) -> Result<(), String> {
    let monitor = pet
        .current_monitor()
        .map_err(|error| format!("monitor lookup failed: {error}"))?
        .ok_or_else(|| "pet monitor unavailable".to_string())?;
    let work_area = monitor.work_area();
    let pet_position = pet.outer_position().map_err(|error| error.to_string())?;
    let pet_size = pet.outer_size().map_err(|error| error.to_string())?;
    let panel_size = panel.outer_size().map_err(|error| error.to_string())?;
    let gap = 12_i64;
    let work_x = i64::from(work_area.position.x);
    let work_y = i64::from(work_area.position.y);
    let work_width = i64::from(work_area.size.width);
    let work_height = i64::from(work_area.size.height);
    let panel_width = i64::from(panel_size.width);
    let panel_height = i64::from(panel_size.height);
    let pet_x = i64::from(pet_position.x);
    let pet_y = i64::from(pet_position.y);
    let pet_width = i64::from(pet_size.width);
    let pet_height = i64::from(pet_size.height);
    let right = (pet_x + pet_width + gap, pet_y);
    let left = (pet_x - panel_width - gap, pet_y);
    let below = (pet_x, pet_y + pet_height + gap);
    let above = (pet_x, pet_y - panel_height - gap);
    let work_right = work_x + work_width;
    let work_bottom = work_y + work_height;
    let fits = |(x, y): (i64, i64)| x >= work_x && y >= work_y && x + panel_width <= work_right && y + panel_height <= work_bottom;
    let (desired_x, desired_y) = [right, left, below, above].into_iter().find(|candidate| fits(*candidate)).unwrap_or(right);
    let (x, y) = bounded_panel_position(desired_x, desired_y, work_x, work_y, work_width, work_height, panel_width, panel_height);
    panel.set_position(PhysicalPosition::new(x, y)).map_err(|error| error.to_string())
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

fn bounded_pet_drag_debug_event(event: &str) -> Option<&str> {
    match event {
        "drag:down" | "drag:move" | "drag:threshold" | "drag:manual-start" | "drag:position-request" | "drag:position-ok"
        | "drag:quick-panel-called" | "drag:quick-panel-ok" | "drag:quick-panel-failed"
        | "drag:start-called" | "drag:start-ok" | "drag:click" | "drag:clear"
        | "drag:position-failed:ipc_rejected"
        | "drag:position-failed:window_unavailable"
        | "drag:position-failed:permission_denied"
        | "drag:position-failed:unknown"
        | "drag:start-failed:ipc_rejected"
        | "drag:start-failed:window_unavailable"
        | "drag:start-failed:permission_denied"
        | "drag:start-failed:unknown" => Some(event),
        _ => None,
    }
}

fn write_pet_drag_debug_event(event: &str) -> Result<(), String> {
    let bounded = bounded_pet_drag_debug_event(event).ok_or_else(|| "invalid_debug_event".to_string())?;
    let path = runtime_root().join("logs").join("pet-v3-drag-debug.log");
    let parent = path.parent().ok_or_else(|| "debug_log_unavailable".to_string())?;
    fs::create_dir_all(parent).map_err(|_| "debug_log_unavailable".to_string())?;
    if fs::metadata(&path).map(|metadata| metadata.len() > 64 * 1024).unwrap_or(false) {
        fs::write(&path, b"").map_err(|_| "debug_log_unavailable".to_string())?;
    }
    let mut file = OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
        .map_err(|_| "debug_log_unavailable".to_string())?;
    writeln!(file, "{bounded}").map_err(|_| "debug_log_unavailable".to_string())
}

fn bounded_quick_panel_debug_event(event: &str) -> Option<&str> {
    match event {
        "quick-panel:created"
        | "quick-panel:open-command"
        | "quick-panel:show-request"
        | "quick-panel:show-ok"
        | "quick-panel:show-failed"
        | "quick-panel:focus-request"
        | "quick-panel:focus-ok"
        | "quick-panel:focus-failed"
        | "quick-panel:position-failed"
        | "quick-panel:focused-true"
        | "quick-panel:focused-false"
        | "quick-panel:hide-reason:explicit"
        | "quick-panel:hide-reason:open-control-center"
        | "quick-panel:hide-reason:close-request" => Some(event),
        _ => None,
    }
}

fn write_quick_panel_debug_event(event: &str) -> Result<(), String> {
    let bounded = bounded_quick_panel_debug_event(event).ok_or_else(|| "invalid_debug_event".to_string())?;
    let path = runtime_root().join("logs").join("pet-v3-quick-panel-debug.log");
    let parent = path.parent().ok_or_else(|| "debug_log_unavailable".to_string())?;
    fs::create_dir_all(parent).map_err(|_| "debug_log_unavailable".to_string())?;
    if fs::metadata(&path).map(|metadata| metadata.len() > 64 * 1024).unwrap_or(false) {
        fs::write(&path, b"").map_err(|_| "debug_log_unavailable".to_string())?;
    }
    let mut file = OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
        .map_err(|_| "debug_log_unavailable".to_string())?;
    writeln!(file, "{bounded}").map_err(|_| "debug_log_unavailable".to_string())
}

fn record_quick_panel_debug_event(event: &str) {
    let _ = write_quick_panel_debug_event(event);
}

fn bounded_control_center_route(route: &str) -> Option<&'static str> {
    CONTROL_CENTER_ROUTES.iter().copied().find(|allowed| *allowed == route)
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

fn classify_control_status(status: u16) -> Result<(), NativeErrorKind> {
    match status {
        200..=299 => Ok(()),
        401 | 403 => Err(NativeErrorKind::Unauthorized),
        400 | 409 => Err(NativeErrorKind::Rejected),
        _ => Err(NativeErrorKind::Unavailable),
    }
}

fn response_parts(response: &[u8]) -> Result<(usize, u16), NativeErrorKind> {
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
    Ok((separator + 4, status))
}

fn parse_http_response(response: &[u8]) -> Result<Value, NativeErrorKind> {
    let (body_start, status) = response_parts(response)?;
    classify_http_status(status)?;
    serde_json::from_slice(&response[body_start..])
        .map_err(|_| NativeErrorKind::InvalidResponse)
}

fn fetch_supervisor_get(host: &str, port: u16, token: &str, path: &str) -> Result<Value, NativeErrorKind> {
    if !SUPERVISOR_GET_PATHS.contains(&path) || host.contains('\r') || host.contains('\n') {
        return Err(NativeErrorKind::Unavailable);
    }
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
        "GET {path} HTTP/1.1\r\nHost: {host}\r\nAuthorization: Bearer {token}\r\nConnection: close\r\n\r\n"
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

fn fetch_current_activity(host: &str, port: u16, token: &str) -> Result<Value, NativeErrorKind> {
    fetch_supervisor_get(host, port, token, "/v1/activity/current")
}

#[derive(Debug, PartialEq, Eq)]
struct ModeRequest {
    path: &'static str,
    body: Vec<u8>,
}

fn build_mode_request(mode: &str, task: Option<&str>) -> Result<ModeRequest, NativeErrorKind> {
    match mode {
        "STUDY" => {
            let task = task.unwrap_or_default();
            if task.chars().count() > 256 {
                return Err(NativeErrorKind::Rejected);
            }
            let body = serde_json::to_vec(&json!({ "task": task }))
                .map_err(|_| NativeErrorKind::InvalidResponse)?;
            Ok(ModeRequest { path: "/v1/mode/study", body })
        }
        "BREAK" if task.is_none() => Ok(ModeRequest { path: "/v1/mode/break", body: Vec::new() }),
        "OFF" if task.is_none() => Ok(ModeRequest { path: "/v1/mode/off", body: Vec::new() }),
        _ => Err(NativeErrorKind::Rejected),
    }
}

fn build_daily_target_body(minutes: i64) -> Result<Vec<u8>, NativeErrorKind> {
    if !(1..=1440).contains(&minutes) {
        return Err(NativeErrorKind::Rejected);
    }
    serde_json::to_vec(&json!({ "daily_target_minutes": minutes }))
        .map_err(|_| NativeErrorKind::InvalidResponse)
}

fn post_supervisor_mode(host: &str, port: u16, token: &str, request: &ModeRequest) -> Result<Value, NativeErrorKind> {
    if host.contains('\r') || host.contains('\n') {
        return Err(NativeErrorKind::Unavailable);
    }
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

    let request_head = format!(
        "POST {} HTTP/1.1\r\nHost: {host}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        request.path,
        request.body.len(),
    );
    stream
        .write_all(request_head.as_bytes())
        .and_then(|_| stream.write_all(&request.body))
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
    let (body_start, status) = response_parts(&response)?;
    classify_control_status(status)?;
    serde_json::from_slice(&response[body_start..])
        .map_err(|_| NativeErrorKind::InvalidResponse)
}

fn task_preset_path_allowed(path: &str) -> bool {
    if path == "/v1/task" || path == "/v1/task-presets" { return true; }
    let Some(rest) = path.strip_prefix("/v1/task-presets/") else { return false; };
    let mut parts = rest.split('/');
    let Some(id) = parts.next() else { return false; };
    if id.is_empty() || id.len() > 128 || !id.chars().all(|ch| ch.is_ascii_alphanumeric() || ch == '-') { return false; }
    match (parts.next(), parts.next()) {
        (None, None) | (Some("select"), None) => true,
        _ => false,
    }
}

fn task_supervisor_request(host: &str, port: u16, token: &str, method: &str, path: &str, body: &[u8]) -> Result<Value, NativeErrorKind> {
    if !["POST", "PUT", "DELETE"].contains(&method) || !task_preset_path_allowed(path) || host.contains(['\r', '\n']) {
        return Err(NativeErrorKind::Rejected);
    }
    if token.is_empty() || token.contains(['\r', '\n']) { return Err(NativeErrorKind::Unauthorized); }
    let address = loopback_address(host, port)?;
    let mut stream = TcpStream::connect_timeout(&address, REQUEST_TIMEOUT).map_err(|error| map_io_error(&error))?;
    stream.set_read_timeout(Some(REQUEST_TIMEOUT)).map_err(|error| map_io_error(&error))?;
    stream.set_write_timeout(Some(REQUEST_TIMEOUT)).map_err(|error| map_io_error(&error))?;
    let head = format!("{method} {path} HTTP/1.1\r\nHost: {host}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", body.len());
    stream.write_all(head.as_bytes()).and_then(|_| stream.write_all(body)).map_err(|error| map_io_error(&error))?;
    stream.flush().map_err(|error| map_io_error(&error))?;
    let mut response = Vec::with_capacity(4096);
    let mut chunk = [0_u8; 4096];
    loop {
        match stream.read(&mut chunk) {
            Ok(0) => break,
            Ok(read) if response.len() + read <= MAX_RESPONSE_BYTES => response.extend_from_slice(&chunk[..read]),
            Ok(_) => return Err(NativeErrorKind::InvalidResponse),
            Err(error) => return Err(map_io_error(&error)),
        }
    }
    let (body_start, status) = response_parts(&response)?;
    classify_control_status(status)?;
    if status == 204 { return Ok(json!({})); }
    serde_json::from_slice(&response[body_start..]).map_err(|_| NativeErrorKind::InvalidResponse)
}

fn put_daily_target(host: &str, port: u16, token: &str, minutes: i64) -> Result<Value, NativeErrorKind> {
    let body = build_daily_target_body(minutes)?;
    if host.contains('\r') || host.contains('\n') {
        return Err(NativeErrorKind::Unavailable);
    }
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
    let request_head = format!(
        "PUT /v1/motivation/settings HTTP/1.1\r\nHost: {host}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        body.len(),
    );
    stream
        .write_all(request_head.as_bytes())
        .and_then(|_| stream.write_all(&body))
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
    let (body_start, status) = response_parts(&response)?;
    classify_control_status(status)?;
    serde_json::from_slice(&response[body_start..]).map_err(|_| NativeErrorKind::InvalidResponse)
}

fn enum_field(object: &serde_json::Map<String, Value>, key: &str, allowed: &[&str]) -> Result<String, NativeErrorKind> {
    let value = object
        .get(key)
        .and_then(Value::as_str)
        .filter(|value| allowed.contains(value))
        .ok_or(NativeErrorKind::InvalidResponse)?;
    Ok(value.to_string())
}

fn text_field(object: &serde_json::Map<String, Value>, key: &str, max_bytes: usize) -> Result<String, NativeErrorKind> {
    object
        .get(key)
        .and_then(Value::as_str)
        .filter(|value| value.len() <= max_bytes)
        .map(str::to_string)
        .ok_or(NativeErrorKind::InvalidResponse)
}

fn optional_text_field(object: &serde_json::Map<String, Value>, key: &str, max_bytes: usize) -> Result<Option<String>, NativeErrorKind> {
    match object.get(key) {
        None | Some(Value::Null) => Ok(None),
        Some(Value::String(value)) if value.len() <= max_bytes => Ok(Some(value.clone())),
        _ => Err(NativeErrorKind::InvalidResponse),
    }
}

fn non_negative_i64_field(object: &serde_json::Map<String, Value>, key: &str) -> Result<i64, NativeErrorKind> {
    object
        .get(key)
        .and_then(Value::as_i64)
        .filter(|value| *value >= 0)
        .ok_or(NativeErrorKind::InvalidResponse)
}

fn bounded_progress_field(object: &serde_json::Map<String, Value>, key: &str) -> Result<f64, NativeErrorKind> {
    object
        .get(key)
        .and_then(Value::as_f64)
        .filter(|value| value.is_finite() && (0.0..=1.0).contains(value))
        .ok_or(NativeErrorKind::InvalidResponse)
}

fn bool_field(object: &serde_json::Map<String, Value>, key: &str) -> Result<bool, NativeErrorKind> {
    object
        .get(key)
        .and_then(Value::as_bool)
        .ok_or(NativeErrorKind::InvalidResponse)
}

fn sanitize_status(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    let user_mode = enum_field(object, "user_mode", &["STANDBY", "STUDY", "BREAK", "OFF"])?;
    let interaction_state = enum_field(object, "interaction_state", &["ACTIVE", "IDLE_STATIC", "IDLE_DYNAMIC", "UNKNOWN"])?;
    let task_relation = enum_field(object, "task_relation", &["FOCUSED", "DISTRACTED", "UNKNOWN"])?;
    let privacy_state = enum_field(object, "privacy_state", &["NORMAL", "SENSITIVE"])?;
    let confidence = bounded_progress_field(object, "confidence")?;
    let task = text_field(object, "task", 4096)?;
    let study_seconds = non_negative_i64_field(object, "study_seconds")?;
    let break_seconds = non_negative_i64_field(object, "break_seconds")?;
    let active_seconds = non_negative_i64_field(object, "active_seconds")?;
    let activitywatch_ok = bool_field(object, "activitywatch_ok")?;
    let screen_sensor_ok = bool_field(object, "screen_sensor_ok")?;
    let mut output = json!({
        "user_mode": user_mode,
        "interaction_state": interaction_state,
        "task_relation": task_relation,
        "privacy_state": privacy_state,
        "confidence": confidence,
        "task": task,
        "study_seconds": study_seconds,
        "break_seconds": break_seconds,
        "active_seconds": active_seconds,
        "activitywatch_ok": activitywatch_ok,
        "screen_sensor_ok": screen_sensor_ok,
    });
    if let Some(last_activity_at) = optional_text_field(object, "last_activity_at", 128)? {
        output["last_activity_at"] = json!(last_activity_at);
    }
    Ok(output)
}

fn sanitize_motivation(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    let today_credited_focus_minutes = non_negative_i64_field(object, "today_credited_focus_minutes")?;
    let total_credited_focus_minutes = non_negative_i64_field(object, "total_credited_focus_minutes")?;
    let today_earned_ap_milli = non_negative_i64_field(object, "today_earned_ap_milli")?;
    let today_spent_ap_milli = non_negative_i64_field(object, "today_spent_ap_milli")?;
    let balance_ap_milli = non_negative_i64_field(object, "balance_ap_milli")?;
    let daily_target_minutes = non_negative_i64_field(object, "daily_target_minutes")?;
    let target_progress = bounded_progress_field(object, "target_progress")?;
    let streak_days = non_negative_i64_field(object, "streak_days")?;
    let mut output = json!({
        "today_credited_focus_minutes": today_credited_focus_minutes,
        "total_credited_focus_minutes": total_credited_focus_minutes,
        "today_earned_ap_milli": today_earned_ap_milli,
        "today_spent_ap_milli": today_spent_ap_milli,
        "balance_ap_milli": balance_ap_milli,
        "checkin_completed": bool_field(object, "checkin_completed")?,
        "daily_target_minutes": daily_target_minutes,
        "target_progress": target_progress,
        "streak_days": streak_days,
    });
    if let Some(event) = object.get("last_event") {
        let event_object = event.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        output["last_event"] = json!({
            "id": non_negative_i64_field(event_object, "id")?,
            "type": text_field(event_object, "type", 64)?,
            "message": text_field(event_object, "message", 512)?,
            "created_at": text_field(event_object, "created_at", 128)?,
        });
    }
    Ok(output)
}

fn sanitize_history(value: &Value) -> Result<Value, NativeErrorKind> {
    let rows = value.as_array().ok_or(NativeErrorKind::InvalidResponse)?;
    if rows.len() > 90 {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let mut output = Vec::with_capacity(rows.len());
    for row in rows {
        let object = row.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        output.push(json!({
            "date": text_field(object, "date", 32)?,
            "focus_minutes": non_negative_i64_field(object, "focus_minutes")?,
            "target_minutes": non_negative_i64_field(object, "target_minutes")?,
            "checkin_completed": bool_field(object, "checkin_completed")?,
            "target_completed": bool_field(object, "target_completed")?,
        }));
    }
    Ok(Value::Array(output))
}

fn sanitize_achievements(value: &Value) -> Result<Value, NativeErrorKind> {
    let rows = value.as_array().ok_or(NativeErrorKind::InvalidResponse)?;
    if rows.len() > 32 {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let mut output = Vec::with_capacity(rows.len());
    for row in rows {
        let object = row.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        let mut item = json!({
            "achievement_id": text_field(object, "achievement_id", 64)?,
            "name": text_field(object, "name", 128)?,
            "description": text_field(object, "description", 512)?,
            "progress": bounded_progress_field(object, "progress")?,
            "unlocked": bool_field(object, "unlocked")?,
        });
        if let Some(unlocked_at) = optional_text_field(object, "unlocked_at", 128)? {
            item["unlocked_at"] = json!(unlocked_at);
        }
        output.push(item);
    }
    Ok(Value::Array(output))
}

fn sanitize_missions(value: &Value) -> Result<Value, NativeErrorKind> {
    let rows = value.as_array().ok_or(NativeErrorKind::InvalidResponse)?;
    if rows.len() > 100 {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let mut output = Vec::with_capacity(rows.len());
    for row in rows {
        let object = row.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        let mut item = json!({
            "id": text_field(object, "id", 128)?,
            "title": text_field(object, "title", 256)?,
            "description": text_field(object, "description", 1024)?,
            "reward_milli_ap": non_negative_i64_field(object, "reward_milli_ap")?,
            "status": enum_field(object, "status", &["OPEN", "COMPLETED", "CANCELLED"] )?,
            "created_at": text_field(object, "created_at", 128)?,
        });
        if let Some(due_date) = optional_text_field(object, "due_date", 32)? {
            item["due_date"] = json!(due_date);
        }
        if let Some(completed_at) = optional_text_field(object, "completed_at", 128)? {
            item["completed_at"] = json!(completed_at);
        }
        output.push(item);
    }
    Ok(Value::Array(output))
}

fn sanitize_rewards(value: &Value) -> Result<Value, NativeErrorKind> {
    let rows = value.as_array().ok_or(NativeErrorKind::InvalidResponse)?;
    if rows.len() > 100 {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let mut output = Vec::with_capacity(rows.len());
    for row in rows {
        let object = row.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        output.push(json!({
            "id": text_field(object, "id", 128)?,
            "name": text_field(object, "name", 256)?,
            "type": text_field(object, "type", 64)?,
            "cost_milli_ap": non_negative_i64_field(object, "cost_milli_ap")?,
            "description": text_field(object, "description", 1024)?,
            "enabled": bool_field(object, "enabled")?,
        }));
    }
    Ok(Value::Array(output))
}

fn sanitize_ai_settings(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    let endpoint = |key: &str| -> Result<Value, NativeErrorKind> {
        let row = object.get(key).and_then(Value::as_object).ok_or(NativeErrorKind::InvalidResponse)?;
        Ok(json!({
            "enabled": bool_field(row, "enabled")?,
            "provider": text_field(row, "provider", 64)?,
            "model": text_field(row, "model", 128)?,
            "base_url": text_field(row, "base_url", 1024)?,
            "api_key_configured": bool_field(row, "api_key_configured")?,
            "timeout_seconds": non_negative_i64_field(row, "timeout_seconds")?,
            "json_mode": text_field(row, "json_mode", 32)?,
        }))
    };
    let min_confidence = object.get("min_confidence").and_then(Value::as_f64).filter(|value| value.is_finite() && (0.0..=1.0).contains(value)).ok_or(NativeErrorKind::InvalidResponse)?;
    Ok(json!({ "enabled": bool_field(object, "enabled")?, "min_confidence": min_confidence, "text": endpoint("text")?, "vision": endpoint("vision")? }))
}

fn sanitize_reminder_settings(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    let cooldown = non_negative_i64_field(object, "cooldown_minutes")?;
    if !(1..=1440).contains(&cooldown) { return Err(NativeErrorKind::InvalidResponse); }
    let periods = object.get("quiet_periods").and_then(Value::as_array).ok_or(NativeErrorKind::InvalidResponse)?;
    if periods.len() > 12 { return Err(NativeErrorKind::InvalidResponse); }
    let mut safe = Vec::with_capacity(periods.len());
    for period in periods {
        let row = period.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        safe.push(json!({ "start": text_field(row, "start", 5)?, "end": text_field(row, "end", 5)? }));
    }
    Ok(json!({ "cooldown_minutes": cooldown, "quiet_periods": safe }))
}

fn sanitize_task_preset_list(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    let sanitize_rows = |key: &str, max: usize| -> Result<Value, NativeErrorKind> {
        let rows = object.get(key).and_then(Value::as_array).ok_or(NativeErrorKind::InvalidResponse)?;
        if rows.len() > max { return Err(NativeErrorKind::InvalidResponse); }
        let mut output = Vec::with_capacity(rows.len());
        for row in rows {
            let item = row.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
            output.push(json!({
                "id": text_field(item, "id", 128)?,
                "name": text_field(item, "name", 256)?,
                "pinned": bool_field(item, "pinned")?,
                "sort_order": item.get("sort_order").and_then(Value::as_i64).ok_or(NativeErrorKind::InvalidResponse)?,
                "use_count": non_negative_i64_field(item, "use_count")?,
                "last_used_at": optional_text_field(item, "last_used_at", 128)?,
            }));
        }
        Ok(Value::Array(output))
    };
    Ok(json!({ "pinned": sanitize_rows("pinned", 8)?, "recent": sanitize_rows("recent", 6)? }))
}

fn sanitize_ai(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    let mut output = json!({
        "enabled": bool_field(object, "enabled")?,
        "text_provider": text_field(object, "text_provider", 64)?,
        "text_configured": bool_field(object, "text_configured")?,
        "vision_enabled": bool_field(object, "vision_enabled")?,
    });
    if let Some(text_model) = optional_text_field(object, "text_model", 128)? {
        output["text_model"] = json!(text_model);
    }
    if let Some(warning) = optional_text_field(object, "warning", 512)? {
        output["warning"] = json!(warning);
    }
    Ok(output)
}

fn bounded_text_list(value: &Value, max_items: usize, max_bytes: usize) -> Result<Value, NativeErrorKind> {
    let values = value.as_array().ok_or(NativeErrorKind::InvalidResponse)?;
    if values.len() > max_items {
        return Err(NativeErrorKind::InvalidResponse);
    }
    values
        .iter()
        .map(|item| item.as_str().filter(|text| text.len() <= max_bytes).map(Value::from).ok_or(NativeErrorKind::InvalidResponse))
        .collect::<Result<Vec<_>, _>>()
        .map(Value::Array)
}

fn sanitize_review(value: &Value) -> Result<Value, NativeErrorKind> {
    let object = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
    if object.get("schema_version").and_then(Value::as_u64) != Some(1) {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let topics = object.get("topics").and_then(Value::as_array).ok_or(NativeErrorKind::InvalidResponse)?;
    if topics.len() > 16 {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let mut safe_topics = Vec::with_capacity(topics.len());
    for topic in topics {
        let item = topic.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        safe_topics.push(json!({
            "name": text_field(item, "name", 128)?,
            "summary": text_field(item, "summary", 512)?,
            "confidence": bounded_progress_field(item, "confidence")?,
        }));
    }
    let accomplishments = object.get("accomplishments").and_then(Value::as_array).ok_or(NativeErrorKind::InvalidResponse)?;
    if accomplishments.len() > 32 {
        return Err(NativeErrorKind::InvalidResponse);
    }
    let mut safe_accomplishments = Vec::with_capacity(accomplishments.len());
    for accomplishment in accomplishments {
        let item = accomplishment.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
        safe_accomplishments.push(json!({
            "text": text_field(item, "text", 512)?,
            "confidence": bounded_progress_field(item, "confidence")?,
        }));
    }
    let behavior = object.get("behavior").and_then(Value::as_object).ok_or(NativeErrorKind::InvalidResponse)?;
    let safe_behavior = json!({
        "distraction_count": non_negative_i64_field(behavior, "distraction_count")?,
        "largest_distraction_seconds": non_negative_i64_field(behavior, "largest_distraction_seconds")?,
        "average_recovery_seconds": non_negative_i64_field(behavior, "average_recovery_seconds")?,
    });
    let warnings = object.get("warnings").map(|value| bounded_text_list(value, 16, 512)).transpose()?.unwrap_or_else(|| Value::Array(Vec::new()));
    Ok(json!({
        "schema_version": 1,
        "date": text_field(object, "date", 32)?,
        "headline": text_field(object, "headline", 512)?,
        "topics": safe_topics,
        "accomplishments": safe_accomplishments,
        "unfinished": bounded_text_list(object.get("unfinished").ok_or(NativeErrorKind::InvalidResponse)?, 32, 512)?,
        "difficulties": bounded_text_list(object.get("difficulties").ok_or(NativeErrorKind::InvalidResponse)?, 32, 512)?,
        "behavior": safe_behavior,
        "tomorrow_priority": text_field(object, "tomorrow_priority", 512)?,
        "warnings": warnings,
        "status": enum_field(object, "status", &["PENDING", "READY", "STALE", "FAILED"] )?,
        "generation_mode": enum_field(object, "generation_mode", &["AI", "FALLBACK", ""] )?,
        "provider": text_field(object, "provider", 64)?,
        "model": text_field(object, "model", 128)?,
        "revision": non_negative_i64_field(object, "revision")?,
        "attempt_count": non_negative_i64_field(object, "attempt_count")?,
        "error_code": optional_text_field(object, "error_code", 64)?,
        "warnings_count": non_negative_i64_field(object, "warnings_count")?,
    }))
}

fn supervisor_credentials() -> Result<(String, u16, String), NativeErrorKind> {
    let root = runtime_root();
    let token = fs::read_to_string(root.join("config").join("auth.token"))
        .map_err(|_| NativeErrorKind::Unavailable)?
        .trim()
        .to_string();
    let (host, port) = runtime_endpoint(&root);
    Ok((host, port, token))
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

// Supervisor requests use blocking sockets with bounded timeouts. Keep them
// off both the native UI thread and async executor workers.
#[tauri::command]
async fn supervisor_snapshot() -> SupervisorSnapshot {
    tauri::async_runtime::spawn_blocking(supervisor_snapshot_blocking)
        .await
        .unwrap_or_else(|_| disconnected(NativeErrorKind::Unavailable))
}

fn supervisor_snapshot_blocking() -> SupervisorSnapshot {
    let (host, port, token) = match supervisor_credentials() {
        Ok(credentials) => credentials,
        Err(kind) => return disconnected(kind),
    };
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
async fn supervisor_dashboard_snapshot() -> SupervisorDashboardSnapshot {
    tauri::async_runtime::spawn_blocking(supervisor_dashboard_snapshot_blocking)
        .await
        .unwrap_or_else(|_| disconnected_dashboard(NativeErrorKind::Unavailable))
}

fn supervisor_dashboard_snapshot_blocking() -> SupervisorDashboardSnapshot {
    let (host, port, token) = match supervisor_credentials() {
        Ok(credentials) => credentials,
        Err(kind) => return disconnected_dashboard(kind),
    };
    let status_raw = match fetch_supervisor_get(&host, port, &token, "/v1/status") {
        Ok(value) => value,
        Err(kind) => return disconnected_dashboard(kind),
    };
    let status = match sanitize_status(&status_raw) {
        Ok(value) => value,
        Err(kind) => return disconnected_dashboard(kind),
    };
    let motivation = fetch_supervisor_get(&host, port, &token, "/v1/motivation/status")
        .ok()
        .and_then(|value| sanitize_motivation(&value).ok());
    let task_presets = fetch_supervisor_get(&host, port, &token, "/v1/task-presets")
        .ok()
        .and_then(|value| sanitize_task_preset_list(&value).ok());
    let reminder_settings = fetch_supervisor_get(&host, port, &token, "/v1/settings/reminder")
        .ok()
        .and_then(|value| sanitize_reminder_settings(&value).ok());
    let ai_settings = fetch_supervisor_get(&host, port, &token, "/v1/settings/ai")
        .ok()
        .and_then(|value| sanitize_ai_settings(&value).ok());
    let history = fetch_supervisor_get(&host, port, &token, "/v1/motivation/history?days=7")
        .ok()
        .and_then(|value| sanitize_history(&value).ok());
    let achievements = fetch_supervisor_get(&host, port, &token, "/v1/motivation/achievements")
        .ok()
        .and_then(|value| sanitize_achievements(&value).ok());
    let missions = fetch_supervisor_get(&host, port, &token, "/v1/missions")
        .ok()
        .and_then(|value| sanitize_missions(&value).ok());
    let rewards = fetch_supervisor_get(&host, port, &token, "/v1/rewards")
        .ok()
        .and_then(|value| sanitize_rewards(&value).ok());
    let ai = fetch_supervisor_get(&host, port, &token, "/v1/ai/status")
        .ok()
        .and_then(|value| sanitize_ai(&value).ok());
    let review = fetch_supervisor_get(&host, port, &token, "/v1/review/daily")
        .ok()
        .and_then(|value| sanitize_review(&value).ok());
    SupervisorDashboardSnapshot {
        connected: true,
        status: Some(status),
        motivation,
        task_presets,
        reminder_settings,
        ai_settings,
        history,
        achievements,
        missions,
        rewards,
        ai,
        review,
        last_error_kind: None,
    }
}

#[tauri::command]
async fn supervisor_set_mode(mode: String, task: Option<String>) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || supervisor_set_mode_blocking(mode, task))
        .await
        .unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

fn supervisor_set_mode_blocking(mode: String, task: Option<String>) -> SupervisorControlResult {
    let request = match build_mode_request(&mode, task.as_deref()) {
        Ok(request) => request,
        Err(kind) => return SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    };
    let (host, port, token) = match supervisor_credentials() {
        Ok(credentials) => credentials,
        Err(kind) => return SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    };
    match post_supervisor_mode(&host, port, &token, &request) {
        Ok(_) => SupervisorControlResult { ok: true, error_kind: None },
        Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    }
}

fn task_control_result(method: &str, path: String, body: Value) -> SupervisorControlResult {
    let body = match serde_json::to_vec(&body) {
        Ok(value) => value,
        Err(_) => return SupervisorControlResult { ok: false, error_kind: Some("invalid_response") },
    };
    let (host, port, token) = match supervisor_credentials() {
        Ok(credentials) => credentials,
        Err(kind) => return SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    };
    match task_supervisor_request(&host, port, &token, method, &path, &body) {
        Ok(_) => SupervisorControlResult { ok: true, error_kind: None },
        Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    }
}

fn bounded_task_name(name: &str) -> bool {
    let count = name.trim().chars().count();
    (1..=64).contains(&count)
}

#[tauri::command]
async fn supervisor_set_task(task: String) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        if !bounded_task_name(&task) { return SupervisorControlResult { ok: false, error_kind: Some("rejected") }; }
        task_control_result("POST", "/v1/task".to_string(), json!({ "task": task }))
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_create_task_preset(name: String, pinned: bool) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        if !bounded_task_name(&name) { return SupervisorControlResult { ok: false, error_kind: Some("rejected") }; }
        task_control_result("POST", "/v1/task-presets".to_string(), json!({ "name": name, "pinned": pinned, "sort_order": 0 }))
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_select_task_preset(id: String) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || task_control_result("POST", format!("/v1/task-presets/{id}/select"), json!({})))
        .await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_update_task_preset(id: String, name: String, pinned: bool, sort_order: i64) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        if !bounded_task_name(&name) { return SupervisorControlResult { ok: false, error_kind: Some("rejected") }; }
        task_control_result("PUT", format!("/v1/task-presets/{id}"), json!({ "name": name, "pinned": pinned, "sort_order": sort_order }))
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_delete_task_preset(id: String) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || task_control_result("DELETE", format!("/v1/task-presets/{id}"), json!({})))
        .await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[derive(Deserialize, Serialize)]
struct AIEndpointInput {
    enabled: bool, provider: String, model: String, base_url: String,
    api_key_configured: bool, timeout_seconds: i64, json_mode: String,
}

#[derive(Deserialize, Serialize)]
struct AISettingsInput { enabled: bool, min_confidence: f64, text: AIEndpointInput, vision: AIEndpointInput }

fn ai_supervisor_request(method: &str, path: &str, body: &[u8]) -> Result<Value, NativeErrorKind> {
    let allowed = matches!((method, path), ("PUT", "/v1/settings/ai") | ("PUT", "/v1/settings/ai/secret") | ("DELETE", "/v1/settings/ai/secret") | ("POST", "/v1/settings/ai/test") | ("POST", "/v1/review/generate"));
    if !allowed { return Err(NativeErrorKind::Rejected); }
    let (host, port, token) = supervisor_credentials()?;
    if token.is_empty() || token.contains(['\r', '\n']) { return Err(NativeErrorKind::Unauthorized); }
    let address = loopback_address(&host, port)?;
    let mut stream = TcpStream::connect_timeout(&address, REQUEST_TIMEOUT).map_err(|error| map_io_error(&error))?;
    stream.set_read_timeout(Some(Duration::from_secs(125))).map_err(|error| map_io_error(&error))?;
    stream.set_write_timeout(Some(REQUEST_TIMEOUT)).map_err(|error| map_io_error(&error))?;
    let head = format!("{method} {path} HTTP/1.1\r\nHost: {host}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", body.len());
    stream.write_all(head.as_bytes()).and_then(|_| stream.write_all(body)).map_err(|error| map_io_error(&error))?;
    stream.flush().map_err(|error| map_io_error(&error))?;
    let mut response = Vec::new(); stream.take(MAX_RESPONSE_BYTES as u64 + 1).read_to_end(&mut response).map_err(|error| map_io_error(&error))?;
    if response.len() > MAX_RESPONSE_BYTES { return Err(NativeErrorKind::InvalidResponse); }
    let (body_start, status) = response_parts(&response)?;
    if path.ends_with("/test") {
        if status != 200 && status != 502 { classify_control_status(status)?; }
    } else { classify_control_status(status)?; }
    serde_json::from_slice(&response[body_start..]).map_err(|_| NativeErrorKind::InvalidResponse)
}

#[tauri::command]
async fn supervisor_save_ai_settings(settings: AISettingsInput) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        let body = match serde_json::to_vec(&settings) { Ok(value) => value, Err(_) => return SupervisorControlResult { ok: false, error_kind: Some("invalid_response") } };
        match ai_supervisor_request("PUT", "/v1/settings/ai", &body) { Ok(_) => SupervisorControlResult { ok: true, error_kind: None }, Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) } }
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_put_ai_secret(target: String, api_key: String) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        if !["text", "vision"].contains(&target.as_str()) || api_key.trim().is_empty() || api_key.len() > 8192 { return SupervisorControlResult { ok: false, error_kind: Some("rejected") }; }
        let body = serde_json::to_vec(&json!({ "target": target, "api_key": api_key })).unwrap_or_default();
        match ai_supervisor_request("PUT", "/v1/settings/ai/secret", &body) { Ok(_) => SupervisorControlResult { ok: true, error_kind: None }, Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) } }
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_delete_ai_secret(target: String) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        let body = serde_json::to_vec(&json!({ "target": target })).unwrap_or_default();
        match ai_supervisor_request("DELETE", "/v1/settings/ai/secret", &body) { Ok(_) => SupervisorControlResult { ok: true, error_kind: None }, Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) } }
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_test_ai_connection(target: String) -> Value {
    tauri::async_runtime::spawn_blocking(move || {
        let body = serde_json::to_vec(&json!({ "target": target })).unwrap_or_default();
        match ai_supervisor_request("POST", "/v1/settings/ai/test", &body).and_then(|value| {
            let row = value.as_object().ok_or(NativeErrorKind::InvalidResponse)?;
            Ok(json!({ "ok": bool_field(row, "ok")?, "provider": text_field(row, "provider", 64)?, "model": text_field(row, "model", 128)?, "latency_ms": non_negative_i64_field(row, "latency_ms")?, "error_kind": optional_text_field(row, "error_kind", 64)? }))
        }) { Ok(value) => value, Err(kind) => json!({ "ok": false, "provider": "", "model": "", "latency_ms": 0, "error_kind": kind.as_str() }) }
    }).await.unwrap_or_else(|_| json!({ "ok": false, "provider": "", "model": "", "latency_ms": 0, "error_kind": "unavailable" }))
}

#[tauri::command]
async fn supervisor_generate_review() -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        match ai_supervisor_request("POST", "/v1/review/generate", b"{}") {
            Ok(_) => SupervisorControlResult { ok: true, error_kind: None },
            Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
        }
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[derive(Deserialize, Serialize)]
struct QuietPeriodInput { start: String, end: String }

#[tauri::command]
async fn supervisor_set_reminder_settings(cooldown_minutes: i64, quiet_periods: Vec<QuietPeriodInput>) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || {
        if !(1..=1440).contains(&cooldown_minutes) || quiet_periods.len() > 12 {
            return SupervisorControlResult { ok: false, error_kind: Some("rejected") };
        }
        let body = match serde_json::to_vec(&json!({ "cooldown_minutes": cooldown_minutes, "quiet_periods": quiet_periods })) {
            Ok(value) => value,
            Err(_) => return SupervisorControlResult { ok: false, error_kind: Some("invalid_response") },
        };
        let (host, port, token) = match supervisor_credentials() {
            Ok(credentials) => credentials,
            Err(kind) => return SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
        };
        let address = match loopback_address(&host, port) { Ok(value) => value, Err(kind) => return SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) } };
        let result = (|| -> Result<(), NativeErrorKind> {
            if token.is_empty() || token.contains(['\r', '\n']) { return Err(NativeErrorKind::Unauthorized); }
            let mut stream = TcpStream::connect_timeout(&address, REQUEST_TIMEOUT).map_err(|error| map_io_error(&error))?;
            stream.set_read_timeout(Some(REQUEST_TIMEOUT)).map_err(|error| map_io_error(&error))?;
            stream.set_write_timeout(Some(REQUEST_TIMEOUT)).map_err(|error| map_io_error(&error))?;
            let head = format!("PUT /v1/settings/reminder HTTP/1.1\r\nHost: {host}\r\nAuthorization: Bearer {token}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", body.len());
            stream.write_all(head.as_bytes()).and_then(|_| stream.write_all(&body)).map_err(|error| map_io_error(&error))?;
            stream.flush().map_err(|error| map_io_error(&error))?;
            let mut response = Vec::new(); stream.read_to_end(&mut response).map_err(|error| map_io_error(&error))?;
            let (_, status) = response_parts(&response)?; classify_control_status(status)
        })();
        match result { Ok(()) => SupervisorControlResult { ok: true, error_kind: None }, Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) } }
    }).await.unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

#[tauri::command]
async fn supervisor_set_daily_target(minutes: i64) -> SupervisorControlResult {
    tauri::async_runtime::spawn_blocking(move || supervisor_set_daily_target_blocking(minutes))
        .await
        .unwrap_or(SupervisorControlResult { ok: false, error_kind: Some("unavailable") })
}

fn supervisor_set_daily_target_blocking(minutes: i64) -> SupervisorControlResult {
    let (host, port, token) = match supervisor_credentials() {
        Ok(credentials) => credentials,
        Err(kind) => return SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    };
    match put_daily_target(&host, port, &token, minutes) {
        Ok(_) => SupervisorControlResult { ok: true, error_kind: None },
        Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    }
}

// WebView2 creation must not run in a synchronous IPC callback on Windows.
// Serialize complete operations on blocking workers, including route updates,
// so concurrent opens cannot create duplicate labels or reorder route events.
async fn run_aux_window_operation<T, F>(gate: Arc<Mutex<()>>, operation: F) -> Result<T, String>
where
    T: Send + 'static,
    F: FnOnce() -> Result<T, String> + Send + 'static,
{
    tauri::async_runtime::spawn_blocking(move || {
        let _guard = gate.lock().map_err(|_| "auxiliary_window_state_poisoned".to_string())?;
        operation()
    })
    .await
    .map_err(|_| "auxiliary_window_worker_failed".to_string())?
}

// Tray handlers can dispatch this same operation through async_runtime::spawn.
async fn dispatch_aux_window(app: AppHandle, action: AuxiliaryWindowAction) -> Result<(), String> {
    let gate = app.state::<AuxiliaryWindowState>().0.clone();
    run_aux_window_operation(gate, move || match action {
        AuxiliaryWindowAction::QuickPanel => show_quick_panel(&app),
        AuxiliaryWindowAction::ControlCenter(route) => show_control_center(&app, &route),
        AuxiliaryWindowAction::HideQuickPanel(reason) => hide_quick_panel_with_reason(&app, reason),
        AuxiliaryWindowAction::HideControlCenter => {
            if let Some(center) = app.get_webview_window("control-center") {
                center.hide().map_err(|_| "control_center_hide_failed".to_string())?;
            }
            Ok(())
        }
    }).await
}

fn schedule_aux_window(app: AppHandle, action: AuxiliaryWindowAction) {
    tauri::async_runtime::spawn(async move {
        let _ = dispatch_aux_window(app, action).await;
    });
}

#[tauri::command]
async fn open_quick_panel(app: AppHandle) -> Result<(), String> {
    record_quick_panel_debug_event("quick-panel:open-command");
    dispatch_aux_window(app, AuxiliaryWindowAction::QuickPanel).await
}

fn show_quick_panel(app: &AppHandle) -> Result<(), String> {
    let panel = app
        .get_webview_window("quick-panel")
        .map(Ok)
        .unwrap_or_else(|| configured_aux_window(app, "quick-panel"))?;
    if let Some(pet) = app.get_webview_window("main") {
        if let Err(error) = position_quick_panel(&pet, &panel) {
            record_quick_panel_debug_event("quick-panel:position-failed");
            return Err(error);
        }
    }
    record_quick_panel_debug_event("quick-panel:show-request");
    if let Err(error) = panel.show() {
        record_quick_panel_debug_event("quick-panel:show-failed");
        return Err(error.to_string());
    }
    record_quick_panel_debug_event("quick-panel:show-ok");
    record_quick_panel_debug_event("quick-panel:focus-request");
    if let Err(error) = panel.set_focus() {
        record_quick_panel_debug_event("quick-panel:focus-failed");
        return Err(error.to_string());
    }
    record_quick_panel_debug_event("quick-panel:focus-ok");
    Ok(())
}

fn hide_quick_panel_with_reason(app: &AppHandle, reason: &'static str) -> Result<(), String> {
    if let Some(panel) = app.get_webview_window("quick-panel") {
        record_quick_panel_debug_event(reason);
        panel.hide().map_err(|error| error.to_string())?;
    }
    Ok(())
}

#[tauri::command]
async fn hide_quick_panel(app: AppHandle) -> Result<(), String> {
    dispatch_aux_window(app, AuxiliaryWindowAction::HideQuickPanel("quick-panel:hide-reason:explicit")).await
}

#[tauri::command]
async fn open_control_center(app: AppHandle, route: Option<String>) -> Result<(), String> {
    let requested_route = route.as_deref().unwrap_or("overview");
    let route = bounded_control_center_route(requested_route)
        .ok_or_else(|| "invalid_control_center_route".to_string())?;
    dispatch_aux_window(app, AuxiliaryWindowAction::ControlCenter(route.to_string())).await
}

fn show_control_center(app: &AppHandle, route: &str) -> Result<(), String> {
    // Drop the route lock before any window call: getters marshal to the UI
    // thread, which also handles the control_center_route IPC command.
    *app.state::<ControlCenterRouteState>().0.lock()
        .map_err(|_| "control center route state poisoned".to_string())? = route.to_string();
    hide_quick_panel_with_reason(app, "quick-panel:hide-reason:open-control-center")?;
    let center = app
        .get_webview_window("control-center")
        .map(Ok)
        .unwrap_or_else(|| configured_aux_window(app, "control-center"))?;
    center.show().map_err(|error| error.to_string())?;
    center.set_focus().map_err(|error| error.to_string())?;
    let _ = app.emit_to("control-center", CONTROL_CENTER_ROUTE_EVENT, route);
    Ok(())
}

// Fixed, non-business diagnostics for the native input/lifecycle harness.
// Release builds reject this command before reading any window state.
#[tauri::command]
async fn pet_window_diagnostics(app: AppHandle) -> Result<PetWindowDiagnostics, String> {
    if !cfg!(debug_assertions) {
        return Err("diagnostic_unavailable".to_string());
    }
    let gate = app.state::<AuxiliaryWindowState>().0.clone();
    run_aux_window_operation(gate, move || {
        let mut windows = Vec::with_capacity(3);
        for label in ["main", "quick-panel", "control-center"] {
            let status = if let Some(window) = app.get_webview_window(label) {
                NativeWindowDiagnostic {
                    label,
                    exists: true,
                    visible: window.is_visible().map_err(|_| "window_state_unavailable".to_string())?,
                    focused: window.is_focused().map_err(|_| "window_state_unavailable".to_string())?,
                }
            } else {
                NativeWindowDiagnostic { label, exists: false, visible: false, focused: false }
            };
            windows.push(status);
        }
        let control_center_route = app.state::<ControlCenterRouteState>().0.lock()
            .map_err(|_| "control_center_route_unavailable".to_string())?.clone();
        Ok(PetWindowDiagnostics { windows, control_center_route })
    }).await
}

#[tauri::command]
fn control_center_route(state: tauri::State<'_, ControlCenterRouteState>) -> Result<String, String> {
    state
        .0
        .lock()
        .map(|route| route.clone())
        .map_err(|_| "control center route state poisoned".to_string())
}

#[tauri::command]
fn set_click_through(window: Window, state: tauri::State<'_, ClickThroughState>, enabled: bool) -> Result<bool, String> {
    window.set_ignore_cursor_events(enabled).map_err(|err| err.to_string())?;
    *state.0.lock().map_err(|_| "click-through state poisoned".to_string())? = enabled;
    Ok(enabled)
}

#[tauri::command]
fn record_pet_drag_debug(event: String) -> Result<(), String> {
    write_pet_drag_debug_event(&event)
}

pub fn run() {
    tauri::Builder::default()
        .manage(ClickThroughState(Arc::new(Mutex::new(false))))
        .manage(ControlCenterRouteState(Arc::new(Mutex::new("overview".to_string()))))
        .manage(AuxiliaryWindowState(Arc::new(Mutex::new(()))))
        .invoke_handler(tauri::generate_handler![
            set_click_through,
            record_pet_drag_debug,
            supervisor_snapshot,
            supervisor_dashboard_snapshot,
            supervisor_set_mode,
            supervisor_set_task,
            supervisor_create_task_preset,
            supervisor_select_task_preset,
            supervisor_update_task_preset,
            supervisor_delete_task_preset,
            supervisor_set_reminder_settings,
            supervisor_save_ai_settings,
            supervisor_put_ai_secret,
            supervisor_delete_ai_secret,
            supervisor_test_ai_connection,
            supervisor_generate_review,
            supervisor_set_daily_target,
            open_quick_panel,
            hide_quick_panel,
            open_control_center,
            control_center_route,
            pet_window_diagnostics,
        ])
        .setup(|app| {
            let window = app.get_webview_window("main").ok_or("main window missing")?;
            // A previous dev-panel toggle can leave the native window click-through
            // until the process exits; always start the Pet in an interactive state.
            window.set_ignore_cursor_events(false)?;
            window.set_always_on_top(true)?;
            window.set_shadow(false)?;
            let toggle = MenuItem::with_id(app, "toggle-click-through", "切换鼠标穿透", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "退出 Pet", true, None::<&str>)?;
            let quick_panel = MenuItem::with_id(app, "open-quick-panel", "打开快捷面板", true, None::<&str>)?;
            let control_center = MenuItem::with_id(app, "open-control-center", "打开控制中心", true, None::<&str>)?;
            let settings = MenuItem::with_id(app, "open-settings", "打开设置", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&quick_panel, &control_center, &settings, &toggle, &quit])?;
            let tray_state = app.state::<ClickThroughState>().inner().0.clone();
            let mut tray_builder = TrayIconBuilder::new().menu(&menu);
            if let Some(icon) = app.default_window_icon() {
                tray_builder = tray_builder.icon(icon.clone());
            }
            let _tray = tray_builder
                .on_menu_event(move |app, event| {
                    match event.id.as_ref() {
                        "open-quick-panel" => schedule_aux_window(app.clone(), AuxiliaryWindowAction::QuickPanel),
                        "open-control-center" => schedule_aux_window(app.clone(), AuxiliaryWindowAction::ControlCenter("overview".to_string())),
                        "open-settings" => schedule_aux_window(app.clone(), AuxiliaryWindowAction::ControlCenter("settings".to_string())),
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

    use serde_json::{json, Value};

    use super::{
        bounded_control_center_route, bounded_panel_position, bounded_quick_panel_debug_event, build_daily_target_body, build_mode_request, classify_control_status, classify_http_status,
        disconnected, fetch_supervisor_get, map_io_error, next_click_through, parse_http_response,
        sanitize_missions, sanitize_motivation, sanitize_review, sanitize_semantic, sanitize_status, bounded_pet_drag_debug_event,
        NativeErrorKind,
        SupervisorSnapshot,
    };

    #[test]
    fn auxiliary_window_operations_run_off_caller_and_do_not_overlap() {
        use std::sync::{atomic::{AtomicUsize, Ordering}, Arc, Mutex};

        let gate = Arc::new(Mutex::new(()));
        let active = Arc::new(AtomicUsize::new(0));
        let completed = Arc::new(AtomicUsize::new(0));
        let caller = std::thread::current().id();
        let operations: Vec<_> = (0..12).map(|index| {
            let gate = gate.clone();
            let active = active.clone();
            let completed = completed.clone();
            tauri::async_runtime::spawn(super::run_aux_window_operation(gate, move || {
                assert_ne!(std::thread::current().id(), caller);
                assert_eq!(active.fetch_add(1, Ordering::SeqCst), 0, "window operations overlapped");
                // Give another queued operation an opportunity to race the
                // simulated create/show sequence if serialization is removed.
                std::thread::sleep(std::time::Duration::from_millis(2));
                assert_eq!(active.fetch_sub(1, Ordering::SeqCst), 1);
                completed.fetch_add(1, Ordering::SeqCst);
                Ok(index)
            }))
        }).collect();
        for (index, operation) in operations.into_iter().enumerate() {
            assert_eq!(tauri::async_runtime::block_on(operation).unwrap().unwrap(), index);
        }
        assert_eq!(completed.load(Ordering::SeqCst), 12);
    }

    #[test]
    fn auxiliary_window_failure_does_not_block_next_open() {
        let gate = std::sync::Arc::new(std::sync::Mutex::new(()));
        let failure = tauri::async_runtime::block_on(super::run_aux_window_operation(gate.clone(), || {
            Err::<(), _>("window_creation_failed".to_string())
        }));
        assert_eq!(failure, Err("window_creation_failed".to_string()));
        let retry = tauri::async_runtime::block_on(super::run_aux_window_operation(gate, || Ok("shown")));
        assert_eq!(retry, Ok("shown"));
    }

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
        assert_eq!(classify_control_status(400), Err(NativeErrorKind::Rejected));
        assert_eq!(classify_control_status(409), Err(NativeErrorKind::Rejected));
        assert_eq!(classify_control_status(401), Err(NativeErrorKind::Unauthorized));
        assert_eq!(classify_control_status(403), Err(NativeErrorKind::Unauthorized));
        assert_eq!(map_io_error(&io::Error::new(io::ErrorKind::TimedOut, "hidden detail")), NativeErrorKind::Timeout);
        assert_eq!(map_io_error(&io::Error::new(io::ErrorKind::ConnectionRefused, "hidden detail")), NativeErrorKind::Unavailable);
    }

    #[test]
    fn pet_drag_debug_events_are_bounded() {
        assert_eq!(bounded_pet_drag_debug_event("drag:down"), Some("drag:down"));
        assert_eq!(bounded_pet_drag_debug_event("drag:manual-start"), Some("drag:manual-start"));
        assert_eq!(bounded_pet_drag_debug_event("drag:position-failed:unknown"), Some("drag:position-failed:unknown"));
        assert_eq!(bounded_pet_drag_debug_event("drag:quick-panel-failed"), Some("drag:quick-panel-failed"));
        assert_eq!(bounded_pet_drag_debug_event("drag:start-failed:permission_denied"), Some("drag:start-failed:permission_denied"));
        assert_eq!(bounded_pet_drag_debug_event("drag:start-failed:raw-secret"), None);
    }

    #[test]
    fn quick_panel_lifecycle_events_are_bounded() {
        assert_eq!(bounded_quick_panel_debug_event("quick-panel:created"), Some("quick-panel:created"));
        assert_eq!(bounded_quick_panel_debug_event("quick-panel:open-command"), Some("quick-panel:open-command"));
        assert_eq!(bounded_quick_panel_debug_event("quick-panel:hide-reason:explicit"), Some("quick-panel:hide-reason:explicit"));
        assert_eq!(bounded_quick_panel_debug_event("quick-panel:focus-failed:raw-secret"), None);
    }

    #[test]
    fn control_center_routes_are_bounded() {
        assert_eq!(bounded_control_center_route("overview"), Some("overview"));
        assert_eq!(bounded_control_center_route("settings"), Some("settings"));
        assert_eq!(bounded_control_center_route("review"), Some("review"));
        assert_eq!(bounded_control_center_route("javascript:alert(1)"), None);
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

    #[test]
    fn mode_requests_use_fixed_paths_and_json_encoding() {
        let study = build_mode_request("STUDY", Some("quote \" and newline\n"))
            .expect("study request");
        assert_eq!(study.path, "/v1/mode/study");
        let body: Value = serde_json::from_slice(&study.body).expect("study JSON");
        assert_eq!(body["task"], "quote \" and newline\n");

        assert_eq!(build_mode_request("BREAK", None).expect("break request").path, "/v1/mode/break");
        assert_eq!(build_mode_request("OFF", None).expect("off request").path, "/v1/mode/off");
        assert_eq!(build_mode_request("NOPE", None), Err(NativeErrorKind::Rejected));
        assert_eq!(build_mode_request("BREAK", Some("unexpected")), Err(NativeErrorKind::Rejected));
        assert_eq!(build_mode_request("STUDY", Some(&"x".repeat(257))), Err(NativeErrorKind::Rejected));
        let daily_target = build_daily_target_body(120).expect("daily target JSON");
        let target: Value = serde_json::from_slice(&daily_target).expect("daily target body");
        assert_eq!(target["daily_target_minutes"], 120);
        assert_eq!(build_daily_target_body(0), Err(NativeErrorKind::Rejected));
        assert_eq!(build_daily_target_body(1441), Err(NativeErrorKind::Rejected));
    }

    #[test]
    fn dashboard_sanitizers_keep_only_bounded_canonical_fields() {
        let status = sanitize_status(&json!({
            "user_mode": "STUDY",
            "interaction_state": "ACTIVE",
            "task_relation": "FOCUSED",
            "privacy_state": "NORMAL",
            "confidence": 0.9,
            "task": "Go context",
            "study_seconds": 2520,
            "break_seconds": 0,
            "active_seconds": 2520,
            "activitywatch_ok": true,
            "screen_sensor_ok": true,
            "token": "must never cross the native boundary",
        }))
        .expect("valid status");
        assert_eq!(status.as_object().expect("status object").len(), 11);
        assert!(status.get("token").is_none());

        let motivation = sanitize_motivation(&json!({
            "today_credited_focus_minutes": 42,
            "total_credited_focus_minutes": 420,
            "today_earned_ap_milli": 700,
            "today_spent_ap_milli": 0,
            "balance_ap_milli": 7000,
            "checkin_completed": true,
            "daily_target_minutes": 120,
            "target_progress": 0.35,
            "streak_days": 5,
            "secret": "ignored",
        }))
        .expect("valid motivation");
        assert!(motivation.get("secret").is_none());
        assert!(sanitize_missions(&json!([{
            "id": "m-1",
            "title": "Read",
            "description": "",
            "reward_milli_ap": 100,
            "status": "INVALID",
            "created_at": "2026-09-04T00:00:00Z",
        }]))
        .is_err());
        assert_eq!(fetch_supervisor_get("127.0.0.1", 17321, "", "/v1/private"), Err(NativeErrorKind::Unavailable));
        assert_eq!(fetch_supervisor_get("127.0.0.1", 17321, "", "/v1/status"), Err(NativeErrorKind::Unauthorized));

        let review = sanitize_review(&json!({
            "schema_version": 1,
            "date": "2026-09-04",
            "headline": "完成了一小步",
            "topics": [{"name": "Go", "summary": "复习 context", "confidence": 0.8, "evidence_refs": ["private"]}],
            "accomplishments": [{"text": "完成练习", "confidence": 0.9, "evidence_refs": ["private"]}],
            "unfinished": ["整理笔记"],
            "difficulties": [],
            "behavior": {"distraction_count": 1, "largest_distraction_seconds": 30, "average_recovery_seconds": 20},
            "tomorrow_priority": "继续练习",
            "warnings": ["仅供复盘"],
            "status": "READY",
            "generation_mode": "FALLBACK",
            "provider": "",
            "model": "",
            "revision": 1,
            "attempt_count": 1,
            "error_code": "provider_not_configured",
            "warnings_count": 1,
            "raw_chat": "must never cross the native boundary",
        }))
        .expect("valid review");
        assert!(review.get("raw_chat").is_none());
        assert!(review["topics"][0].get("evidence_refs").is_none());
        assert!(review["accomplishments"][0].get("evidence_refs").is_none());
    }

    #[test]
    fn quick_panel_position_stays_inside_negative_or_positive_work_areas() {
        assert_eq!(bounded_panel_position(1900, 900, 0, 0, 1920, 1080, 380, 430), (1540, 650));
        assert_eq!(bounded_panel_position(-600, -400, -1280, 0, 1280, 1024, 380, 430), (-600, 0));
        assert_eq!(bounded_panel_position(-2000, 1200, -1280, 0, 1280, 1024, 380, 430), (-1280, 594));
    }
}
