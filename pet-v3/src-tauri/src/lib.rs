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
use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    AppHandle, Manager, PhysicalPosition, WebviewWindow, WebviewWindowBuilder, Window, WindowEvent,
};

struct ClickThroughState(Arc<Mutex<bool>>);

fn next_click_through(current: bool) -> bool { !current }

const REQUEST_TIMEOUT: Duration = Duration::from_secs(2);
const MAX_RESPONSE_BYTES: usize = 128 * 1024;
const SUPERVISOR_GET_PATHS: &[&str] = &[
    "/v1/activity/current",
    "/v1/status",
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
    let hide_on_focus_loss = label == "quick-panel";
    let window = WebviewWindowBuilder::from_config(app, &config)
        .map_err(|error| format!("window configuration rejected: {error}"))?
        .build()
        .map_err(|error| format!("window creation failed: {error}"))?;
    let window_for_events = window.clone();
    window.on_window_event(move |event| match event {
            WindowEvent::CloseRequested { api, .. } => {
                api.prevent_close();
                let _ = window_for_events.hide();
            }
            WindowEvent::Focused(false) if hide_on_focus_loss => {
                let _ = window_for_events.hide();
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

#[tauri::command]
fn supervisor_snapshot() -> SupervisorSnapshot {
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
fn supervisor_dashboard_snapshot() -> SupervisorDashboardSnapshot {
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
fn supervisor_set_mode(mode: String, task: Option<String>) -> SupervisorControlResult {
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

#[tauri::command]
fn supervisor_set_daily_target(minutes: i64) -> SupervisorControlResult {
    let (host, port, token) = match supervisor_credentials() {
        Ok(credentials) => credentials,
        Err(kind) => return SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    };
    match put_daily_target(&host, port, &token, minutes) {
        Ok(_) => SupervisorControlResult { ok: true, error_kind: None },
        Err(kind) => SupervisorControlResult { ok: false, error_kind: Some(kind.as_str()) },
    }
}

#[tauri::command]
fn open_quick_panel(app: AppHandle) -> Result<(), String> {
    let panel = app
        .get_webview_window("quick-panel")
        .map(Ok)
        .unwrap_or_else(|| configured_aux_window(&app, "quick-panel"))?;
    if let Some(pet) = app.get_webview_window("main") {
        position_quick_panel(&pet, &panel)?;
    }
    panel.show().map_err(|error| error.to_string())?;
    panel.set_focus().map_err(|error| error.to_string())
}

#[tauri::command]
fn hide_quick_panel(app: AppHandle) -> Result<(), String> {
    if let Some(panel) = app.get_webview_window("quick-panel") {
        panel.hide().map_err(|error| error.to_string())?;
    }
    Ok(())
}

#[tauri::command]
fn open_control_center(app: AppHandle) -> Result<(), String> {
    let center = app
        .get_webview_window("control-center")
        .map(Ok)
        .unwrap_or_else(|| configured_aux_window(&app, "control-center"))?;
    center.show().map_err(|error| error.to_string())?;
    center.set_focus().map_err(|error| error.to_string())
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
        .invoke_handler(tauri::generate_handler![
            set_click_through,
            supervisor_snapshot,
            supervisor_dashboard_snapshot,
            supervisor_set_mode,
            supervisor_set_daily_target,
            open_quick_panel,
            hide_quick_panel,
            open_control_center,
        ])
        .setup(|app| {
            let window = app.get_webview_window("main").ok_or("main window missing")?;
            window.set_always_on_top(true)?;
            window.set_shadow(false)?;
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

    use serde_json::{json, Value};

    use super::{
        bounded_panel_position, build_daily_target_body, build_mode_request, classify_control_status, classify_http_status,
        disconnected, fetch_supervisor_get, map_io_error, next_click_through, parse_http_response,
        sanitize_missions, sanitize_motivation, sanitize_review, sanitize_semantic, sanitize_status,
        NativeErrorKind,
        SupervisorSnapshot,
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
        assert_eq!(classify_control_status(400), Err(NativeErrorKind::Rejected));
        assert_eq!(classify_control_status(409), Err(NativeErrorKind::Rejected));
        assert_eq!(classify_control_status(401), Err(NativeErrorKind::Unauthorized));
        assert_eq!(classify_control_status(403), Err(NativeErrorKind::Unauthorized));
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
