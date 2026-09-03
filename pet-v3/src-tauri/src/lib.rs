use std::sync::{Arc, Mutex};
use tauri::{menu::{Menu, MenuItem}, tray::TrayIconBuilder, Manager, Window};

struct ClickThroughState(Arc<Mutex<bool>>);

fn next_click_through(current: bool) -> bool { !current }

#[tauri::command]
fn set_click_through(window: Window, state: tauri::State<'_, ClickThroughState>, enabled: bool) -> Result<bool, String> {
    window.set_ignore_cursor_events(enabled).map_err(|err| err.to_string())?;
    *state.0.lock().map_err(|_| "click-through state poisoned".to_string())? = enabled;
    Ok(enabled)
}

pub fn run() {
    tauri::Builder::default()
        .manage(ClickThroughState(Arc::new(Mutex::new(false))))
        .invoke_handler(tauri::generate_handler![set_click_through])
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
    use super::next_click_through;

    #[test]
    fn tray_toggle_is_reversible_for_recovery() {
        assert!(next_click_through(false));
        assert!(!next_click_through(next_click_through(false)));
    }
}
