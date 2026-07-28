pub mod gcode;
pub mod grbl;
pub mod machine;
#[cfg(feature = "serial")]
pub mod serial_link;
pub mod settings;

#[cfg(feature = "desktop")]
use machine::{AppSettings, Axis, Machine, Snapshot, Vec4};
#[cfg(feature = "desktop")]
use serde::{Deserialize, Serialize};
#[cfg(feature = "desktop")]
use std::{
    io::{Read, Write},
    path::{Path, PathBuf},
    sync::{Arc, Mutex},
    time::{Duration, Instant},
};
#[cfg(feature = "desktop")]
use tauri::{Emitter, Manager, Runtime};

#[cfg(feature = "desktop")]
struct AppState {
    machine: Arc<Mutex<Machine>>,
    serial: Mutex<Option<serial_link::SerialLink>>,
    settings_path: std::path::PathBuf,
}

#[cfg(feature = "desktop")]
#[derive(Clone, Debug, Deserialize, Serialize)]
struct FileCandidate {
    path: String,
    label: String,
    is_dir: bool,
}

#[cfg(feature = "desktop")]
#[derive(Clone, Debug, Deserialize, Serialize)]
struct GcodeFilePreview {
    path: String,
    filename: String,
    size_bytes: u64,
    line_count: usize,
    preview_lines: Vec<String>,
}

#[cfg(feature = "desktop")]
#[derive(Clone, Debug, Deserialize, Serialize)]
struct ProgramProgress {
    next_line: usize,
    current_line: Option<usize>,
    queued_lines: Vec<usize>,
    running: bool,
    optional_stop: bool,
    pause_after_line: Option<usize>,
    tool_change_line: Option<usize>,
    run_started_line: usize,
    total_bytes_in_flight: usize,
    running_elapsed_seconds: f64,
    estimated_seconds: f64,
    remaining_seconds: f64,
}

#[cfg(feature = "desktop")]
#[derive(Clone, Debug, Deserialize, Serialize)]
struct LiveSnapshot {
    status: machine::ControllerStatus,
    program: ProgramProgress,
    messages: Vec<machine::UiMessage>,
    console: Vec<machine::ConsoleEntry>,
    settings: AppSettings,
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn get_snapshot(state: tauri::State<'_, AppState>) -> Snapshot {
    state
        .machine
        .lock()
        .expect("machine mutex poisoned")
        .snapshot()
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn save_settings(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    settings: AppSettings,
) -> Result<(), String> {
    with_machine_persist(&app, &state, |machine| {
        machine.update_settings(settings);
        Ok(())
    })
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn connect_sim(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    close_serial(&state);
    {
        let mut machine = state.machine.lock().expect("machine mutex poisoned");
        machine.connect_sim();
    }
    emit_live_snapshot(&app, &state.machine);
    Ok(())
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn connect_port(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    port_name: String,
) -> Result<(), String> {
    connect_port_inner(&app, &state, port_name)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn auto_connect(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    close_serial(&state);
    let preferred = state
        .machine
        .lock()
        .expect("machine mutex poisoned")
        .settings()
        .last_port
        .clone();
    let candidates = auto_connect_candidates(preferred);
    if candidates.is_empty() {
        let error = "no serial ports found".to_string();
        {
            let mut machine = state.machine.lock().expect("machine mutex poisoned");
            machine.connection_failed("<auto>".to_string(), error.clone());
        }
        emit_live_snapshot(&app, &state.machine);
        return Err(error);
    }

    let mut tried = Vec::new();
    for port_name in candidates {
        match probe_grbl_port(&port_name) {
            Ok(true) => return connect_port_inner(&app, &state, port_name),
            Ok(false) => tried.push(format!("{port_name}: no Grbl response")),
            Err(error) => tried.push(format!("{port_name}: {error}")),
        }
    }

    let error = format!("no Grbl controller found ({})", tried.join("; "));
    {
        let mut machine = state.machine.lock().expect("machine mutex poisoned");
        machine.connection_failed("<auto>".to_string(), error.clone());
    }
    emit_live_snapshot(&app, &state.machine);
    Err(error)
}

#[cfg(feature = "desktop")]
fn connect_port_inner(
    app: &tauri::AppHandle<impl Runtime>,
    state: &tauri::State<'_, AppState>,
    port_name: String,
) -> Result<(), String> {
    close_serial(state);

    let machine = Arc::clone(&state.machine);
    let serial_app = app.clone();
    let mut last_live_emit = Instant::now() - Duration::from_millis(250);

    let settings = {
        let mut machine = state.machine.lock().expect("machine mutex poisoned");
        machine.remember_port(port_name.clone());
        machine.connect_port(port_name.clone());
        machine.settings().clone()
    };
    persist_settings_async(state.settings_path.clone(), settings);
    emit_live_snapshot(app, &state.machine);

    let link = match serial_link::SerialLink::open(&port_name, 115_200, move |line| {
        let outbox = {
            let mut machine = machine.lock().expect("machine mutex poisoned");
            machine.receive_controller_line(&line);
            machine.take_outbox()
        };
        let now = Instant::now();
        if line != "ok" || now.duration_since(last_live_emit) >= Duration::from_millis(250) {
            emit_live_snapshot(&serial_app, &machine);
            last_live_emit = now;
        }
        if let Ok(session) = serial_app.state::<AppState>().serial.lock() {
            if let Some(serial) = session.as_ref() {
                let _ = send_serial_outbox(serial, outbox);
            }
        }
    }) {
        Ok(link) => link,
        Err(error) => {
            {
                let mut machine = state.machine.lock().expect("machine mutex poisoned");
                machine.connection_failed(port_name, error.clone());
            }
            emit_live_snapshot(app, &state.machine);
            return Err(error);
        }
    };
    link.poll_every("?".to_string(), std::time::Duration::from_millis(200));
    {
        let outbox = {
            let mut machine = state.machine.lock().expect("machine mutex poisoned");
            machine.queue_startup_probe();
            machine.take_outbox()
        };
        if let Err(error) = send_serial_outbox(&link, outbox) {
            {
                let mut machine = state.machine.lock().expect("machine mutex poisoned");
                machine.connection_failed(port_name, error.clone());
            }
            emit_live_snapshot(app, &state.machine);
            return Err(error);
        }
    }

    state
        .serial
        .lock()
        .expect("serial mutex poisoned")
        .replace(link);
    Ok(())
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn disconnect(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    close_serial(&state);
    {
        let mut machine = state.machine.lock().expect("machine mutex poisoned");
        machine.disconnect();
    }
    emit_live_snapshot(&app, &state.machine);
    Ok(())
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn load_gcode_text(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    filename: String,
    contents: String,
) -> Result<(), String> {
    with_machine_snapshot(&app, &state, |machine| {
        machine.load_gcode_text(filename, contents)
    })
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn load_gcode_path(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    path: String,
) -> Result<(), String> {
    let path = PathBuf::from(path);
    let contents = std::fs::read_to_string(&path)
        .map_err(|err| format!("failed to read {}: {err}", path.display()))?;
    let filename = file_display_name(&path);
    with_machine_snapshot_persist(&app, &state, |machine| {
        machine.load_gcode_text(filename, contents)?;
        machine.remember_gcode_path(path.to_string_lossy().into_owned());
        Ok(())
    })
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn complete_gcode_path(partial: String) -> Result<Vec<FileCandidate>, String> {
    Ok(complete_path(&partial, 80))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn preview_gcode_path(path: String) -> Result<GcodeFilePreview, String> {
    preview_path(&path, 80)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn run_program(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::run_program)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn run_or_resume_program(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::run_or_resume_program)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn run_from_line(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    line: usize,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| machine.run_from_line(line))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn toggle_optional_stop(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| {
        machine.toggle_optional_stop();
        Ok(())
    })
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn set_pause_after_line(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    line: Option<usize>,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| machine.set_pause_after_line(line))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn pause_program(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::pause_program)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn resume_program(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::resume_program)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn stop_program(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::stop_program)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn drain_program(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::drain_program)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn single_step(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::single_step)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn send_mdi(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    line: String,
) -> Result<(), String> {
    with_machine_snapshot_persist(&app, &state, |machine| machine.send_mdi(line))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn jog_step(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    axis: Axis,
    delta: f64,
    feed: f64,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| machine.jog_step(axis, delta, feed))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn start_jog(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    axis: Axis,
    direction: i32,
    step: f64,
    feed: f64,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| {
        machine.start_jog(axis, direction, step, feed)
    })
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn extend_jog(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    axis: Axis,
    direction: i32,
    period_ms: u32,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| {
        machine.extend_jog(axis, direction, period_ms)
    })
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn stop_jog(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::stop_jog)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn jog_to_xy(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    x: f64,
    y: f64,
    feed: f64,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| machine.jog_to_xy(x, y, feed))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn set_work_position(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    values: Vec4,
) -> Result<(), String> {
    with_machine_persist(&app, &state, |machine| machine.set_work_position(values))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn unlock_alarm(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
) -> Result<(), String> {
    with_machine(&app, &state, Machine::unlock_alarm)
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn adjust_feed_override(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    delta: i32,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| machine.adjust_feed_override(delta))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn set_rapid_override(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    value: i32,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| machine.set_rapid_override(value))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn adjust_spindle_override(
    app: tauri::AppHandle<impl Runtime>,
    state: tauri::State<'_, AppState>,
    delta: i32,
) -> Result<(), String> {
    with_machine(&app, &state, |machine| machine.adjust_spindle_override(delta))
}

#[cfg(feature = "desktop")]
#[tauri::command]
fn list_ports() -> Vec<String> {
    serialport::available_ports()
        .map(|ports| ports.into_iter().map(|p| p.port_name).collect())
        .unwrap_or_default()
}

#[cfg(feature = "desktop")]
fn auto_connect_candidates(preferred: Option<String>) -> Vec<String> {
    let mut ports = serialport::available_ports().unwrap_or_default();
    ports.sort_by(|a, b| a.port_name.cmp(&b.port_name));

    let mut candidates = Vec::new();
    if let Some(preferred) = preferred {
        if ports.iter().any(|port| port.port_name == preferred) {
            candidates.push(preferred);
        }
    }

    for prefer_usb in [true, false] {
        for port in &ports {
            let is_usb = matches!(port.port_type, serialport::SerialPortType::UsbPort(_));
            if is_usb == prefer_usb && !candidates.iter().any(|item| item == &port.port_name) {
                candidates.push(port.port_name.clone());
            }
        }
    }
    candidates
}

#[cfg(feature = "desktop")]
fn probe_grbl_port(port_name: &str) -> Result<bool, String> {
    let mut port = serialport::new(port_name, 115_200)
        .timeout(Duration::from_millis(100))
        .open()
        .map_err(|err| format!("open failed: {err}"))?;

    let _ = port.write_data_terminal_ready(false);
    std::thread::sleep(Duration::from_millis(80));
    let _ = port.write_data_terminal_ready(true);
    std::thread::sleep(Duration::from_millis(1200));
    let _ = port.clear(serialport::ClearBuffer::Input);
    port.write_all(b"\n$I\n?\n")
        .map_err(|err| format!("probe write failed: {err}"))?;
    port.flush()
        .map_err(|err| format!("probe flush failed: {err}"))?;

    let started = Instant::now();
    let mut buf = [0_u8; 256];
    let mut transcript = String::new();
    while started.elapsed() < Duration::from_secs(2) {
        match port.read(&mut buf) {
            Ok(0) => {}
            Ok(read) => {
                transcript.push_str(&String::from_utf8_lossy(&buf[..read]));
                if transcript_contains_grbl(&transcript) {
                    return Ok(true);
                }
            }
            Err(err) if err.kind() == std::io::ErrorKind::TimedOut => {}
            Err(err) => return Err(format!("probe read failed: {err}")),
        }
    }
    Ok(false)
}

#[cfg(feature = "desktop")]
fn transcript_contains_grbl(transcript: &str) -> bool {
    let lower = transcript.to_ascii_lowercase();
    lower.contains("grbl")
        || lower.contains("[ver:")
        || lower.contains("[firmware:grblhal")
        || lower.contains("<idle|")
}

#[cfg(feature = "desktop")]
pub fn run() {
    let settings_path = settings::settings_path();
    let settings = settings::load(&settings_path);

    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .manage(AppState {
            machine: Arc::new(Mutex::new(Machine::new_with_settings(settings))),
            serial: Mutex::new(None),
            settings_path,
        })
        .invoke_handler(tauri::generate_handler![
            get_snapshot,
            save_settings,
            connect_sim,
            connect_port,
            auto_connect,
            disconnect,
            load_gcode_text,
            load_gcode_path,
            complete_gcode_path,
            preview_gcode_path,
            run_program,
            run_or_resume_program,
            run_from_line,
            toggle_optional_stop,
            set_pause_after_line,
            pause_program,
            resume_program,
            stop_program,
            drain_program,
            single_step,
            send_mdi,
            jog_step,
            start_jog,
            extend_jog,
            stop_jog,
            jog_to_xy,
            set_work_position,
            unlock_alarm,
            adjust_feed_override,
            set_rapid_override,
            adjust_spindle_override,
            list_ports
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

#[cfg(feature = "desktop")]
fn with_machine<R: Runtime, T>(
    app: &tauri::AppHandle<R>,
    state: &tauri::State<'_, AppState>,
    f: impl FnOnce(&mut Machine) -> Result<T, String>,
) -> Result<T, String> {
    with_machine_emit(app, state, f, emit_live_snapshot, false)
}

#[cfg(feature = "desktop")]
fn with_machine_persist<R: Runtime, T>(
    app: &tauri::AppHandle<R>,
    state: &tauri::State<'_, AppState>,
    f: impl FnOnce(&mut Machine) -> Result<T, String>,
) -> Result<T, String> {
    with_machine_emit(app, state, f, emit_live_snapshot, true)
}

#[cfg(feature = "desktop")]
fn with_machine_snapshot<R: Runtime, T>(
    app: &tauri::AppHandle<R>,
    state: &tauri::State<'_, AppState>,
    f: impl FnOnce(&mut Machine) -> Result<T, String>,
) -> Result<T, String> {
    with_machine_emit(app, state, f, emit_snapshot, false)
}

#[cfg(feature = "desktop")]
fn with_machine_snapshot_persist<R: Runtime, T>(
    app: &tauri::AppHandle<R>,
    state: &tauri::State<'_, AppState>,
    f: impl FnOnce(&mut Machine) -> Result<T, String>,
) -> Result<T, String> {
    with_machine_emit(app, state, f, emit_snapshot, true)
}

#[cfg(feature = "desktop")]
fn with_machine_emit<R: Runtime, T>(
    app: &tauri::AppHandle<R>,
    state: &tauri::State<'_, AppState>,
    f: impl FnOnce(&mut Machine) -> Result<T, String>,
    emit: impl Fn(&tauri::AppHandle<R>, &Arc<Mutex<Machine>>),
    persist_settings: bool,
) -> Result<T, String> {
    let (result, outbox, settings) = {
        let mut machine = state.machine.lock().expect("machine mutex poisoned");
        let result = f(&mut machine);
        let outbox = machine.take_outbox();
        let settings = (persist_settings && result.is_ok()).then(|| machine.settings().clone());
        (result, outbox, settings)
    };
    if !outbox.is_empty() {
        if let Some(serial) = state.serial.lock().expect("serial mutex poisoned").as_ref() {
            send_serial_outbox(serial, outbox)?;
        }
    }
    if let Some(settings) = settings {
        persist_settings_async(state.settings_path.clone(), settings);
    }
    emit(app, &state.machine);
    result
}

#[cfg(feature = "desktop")]
fn persist_settings_async(path: PathBuf, settings: AppSettings) {
    std::thread::spawn(move || {
        if let Err(err) = settings::save(&path, &settings) {
            eprintln!("save settings: {err}");
        }
    });
}

#[cfg(feature = "desktop")]
fn emit_snapshot(app: &tauri::AppHandle<impl Runtime>, machine: &Arc<Mutex<Machine>>) {
    let snapshot = machine.lock().expect("machine mutex poisoned").snapshot();
    if let Err(err) = app.emit("machine:snapshot", snapshot) {
        eprintln!("emit machine:snapshot: {err}");
    }
}

#[cfg(feature = "desktop")]
fn emit_live_snapshot(app: &tauri::AppHandle<impl Runtime>, machine: &Arc<Mutex<Machine>>) {
    let snapshot = machine.lock().expect("machine mutex poisoned").snapshot();
    let remaining_seconds = program_remaining_seconds(&snapshot);
    let live = LiveSnapshot {
        status: snapshot.status,
        program: ProgramProgress {
            next_line: snapshot.program.next_line,
            current_line: snapshot.program.current_line,
            queued_lines: snapshot.program.queued_lines,
            running: snapshot.program.running,
            optional_stop: snapshot.program.optional_stop,
            pause_after_line: snapshot.program.pause_after_line,
            tool_change_line: snapshot.program.tool_change_line,
            run_started_line: snapshot.program.run_started_line,
            total_bytes_in_flight: snapshot.program.total_bytes_in_flight,
            running_elapsed_seconds: snapshot.program.running_elapsed_seconds,
            estimated_seconds: snapshot.program.toolpath.estimated_seconds,
            remaining_seconds,
        },
        messages: snapshot.messages,
        console: snapshot.console,
        settings: snapshot.settings,
    };
    if let Err(err) = app.emit("machine:live", live) {
        eprintln!("emit machine:live: {err}");
    }
}

#[cfg(feature = "desktop")]
fn program_remaining_seconds(snapshot: &machine::Snapshot) -> f64 {
    let program = &snapshot.program;
    let remaining = &program.toolpath.remaining_seconds_by_line;
    if remaining.is_empty() {
        return 0.0;
    }
    let line = program.current_line.unwrap_or(program.next_line);
    let ack_remaining = remaining[line.min(remaining.len() - 1)];
    let cycle_active = matches!(
        snapshot.status.mode,
        machine::MachineMode::Running | machine::MachineMode::Hold | machine::MachineMode::Draining
    ) || program.running
        || program.total_bytes_in_flight > 0;
    if !cycle_active || program.running_elapsed_seconds <= 0.0 {
        return ack_remaining;
    }
    let start_line = program.run_started_line.min(remaining.len() - 1);
    let elapsed_remaining = (remaining[start_line] - program.running_elapsed_seconds).max(0.0);
    ack_remaining.max(elapsed_remaining)
}

#[cfg(feature = "desktop")]
fn close_serial(state: &tauri::State<'_, AppState>) {
    state.serial.lock().expect("serial mutex poisoned").take();
}

#[cfg(feature = "desktop")]
fn send_serial_outbox(serial: &serial_link::SerialLink, outbox: Vec<String>) -> Result<(), String> {
    if outbox.iter().any(|command| command.contains('\u{18}')) {
        serial.clear_pending();
    }
    serial.send_all(outbox)
}

#[cfg(feature = "desktop")]
fn file_display_name(path: &Path) -> String {
    path.file_name()
        .and_then(|name| name.to_str())
        .map(str::to_string)
        .unwrap_or_else(|| path.display().to_string())
}

#[cfg(feature = "desktop")]
fn complete_path(partial: &str, limit: usize) -> Vec<FileCandidate> {
    let expanded = expand_home(partial.trim());
    let (directory, prefix) = completion_base(&expanded);
    let Ok(entries) = std::fs::read_dir(&directory) else {
        return Vec::new();
    };

    let mut candidates = Vec::new();
    for entry in entries.flatten() {
        if candidates.len() >= limit {
            break;
        }
        let file_name = entry.file_name().to_string_lossy().into_owned();
        if file_name.starts_with('.') && !prefix.starts_with('.') {
            continue;
        }
        if !file_name.to_lowercase().starts_with(&prefix.to_lowercase()) {
            continue;
        }
        let path = entry.path();
        let is_dir = path.is_dir();
        if !is_dir && !is_gcode_file(&path) {
            continue;
        }
        let label = if is_dir {
            format!("{file_name}/")
        } else {
            file_name
        };
        let mut display_path = path.to_string_lossy().into_owned();
        if is_dir {
            display_path.push(std::path::MAIN_SEPARATOR);
        }
        candidates.push(FileCandidate {
            path: display_path,
            label,
            is_dir,
        });
    }
    candidates.sort_by(|a, b| b.is_dir.cmp(&a.is_dir).then_with(|| a.label.cmp(&b.label)));
    candidates
}

#[cfg(feature = "desktop")]
fn completion_base(path: &Path) -> (PathBuf, String) {
    if path.is_dir() {
        return (path.to_path_buf(), String::new());
    }
    let directory = path
        .parent()
        .unwrap_or_else(|| Path::new("."))
        .to_path_buf();
    let prefix = path
        .file_name()
        .map(|name| name.to_string_lossy().into_owned())
        .unwrap_or_default();
    (directory, prefix)
}

#[cfg(feature = "desktop")]
fn expand_home(path: &str) -> PathBuf {
    if path == "~" {
        return std::env::var_os("HOME")
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from(path));
    }
    if let Some(rest) = path.strip_prefix("~/") {
        if let Some(home) = std::env::var_os("HOME") {
            return PathBuf::from(home).join(rest);
        }
    }
    PathBuf::from(path)
}

#[cfg(feature = "desktop")]
fn is_gcode_file(path: &Path) -> bool {
    const EXTENSIONS: &[&str] = &["nc", "gcode", "tap", "cnc", "ngc", "txt"];
    path.extension()
        .and_then(|extension| extension.to_str())
        .map(|extension| {
            EXTENSIONS
                .iter()
                .any(|candidate| extension.eq_ignore_ascii_case(candidate))
        })
        .unwrap_or(false)
}

#[cfg(feature = "desktop")]
fn preview_path(path: &str, max_lines: usize) -> Result<GcodeFilePreview, String> {
    let path = expand_home(path.trim());
    if path.is_dir() {
        return Err(format!("{} is a directory", path.display()));
    }
    if !is_gcode_file(&path) {
        return Err(format!("{} is not a supported G-code file", path.display()));
    }
    let metadata = std::fs::metadata(&path)
        .map_err(|err| format!("failed to stat {}: {err}", path.display()))?;
    let contents = std::fs::read_to_string(&path)
        .map_err(|err| format!("failed to preview {}: {err}", path.display()))?;
    let mut line_count = 0;
    let mut preview_lines = Vec::new();
    for line in contents.lines() {
        line_count += 1;
        if preview_lines.len() < max_lines {
            preview_lines.push(line.to_string());
        }
    }
    Ok(GcodeFilePreview {
        path: path.to_string_lossy().into_owned(),
        filename: file_display_name(&path),
        size_bytes: metadata.len(),
        line_count,
        preview_lines,
    })
}

#[cfg(not(feature = "desktop"))]
pub fn run() {}

#[cfg(all(test, feature = "desktop-test"))]
mod desktop_command_tests {
    use super::*;
    use crate::machine::MachineMode;
    use tauri::test::{mock_builder, mock_context, noop_assets, MockRuntime, INVOKE_KEY};

    fn test_app() -> tauri::App<MockRuntime> {
        let settings_path = std::env::temp_dir().join(format!(
            "pugsender-test-settings-{}.json",
            std::process::id()
        ));
        let _ = std::fs::remove_file(&settings_path);

        mock_builder()
            .manage(AppState {
                machine: Arc::new(Mutex::new(Machine::new())),
                serial: Mutex::new(None),
                settings_path,
            })
            .invoke_handler(tauri::generate_handler![
                get_snapshot,
                save_settings,
                connect_sim,
                connect_port,
                auto_connect,
                disconnect,
                load_gcode_text,
                load_gcode_path,
                complete_gcode_path,
                preview_gcode_path,
                run_program,
                run_or_resume_program,
                run_from_line,
                toggle_optional_stop,
                set_pause_after_line,
                pause_program,
                resume_program,
                stop_program,
                drain_program,
                single_step,
                send_mdi,
                jog_step,
                start_jog,
                extend_jog,
                stop_jog,
                jog_to_xy,
                set_work_position,
                unlock_alarm,
                adjust_feed_override,
                set_rapid_override,
                adjust_spindle_override,
                list_ports
            ])
            .build(mock_context(noop_assets()))
            .expect("failed to build test app")
    }

    #[test]
    fn remaining_time_survives_drained_sender_queue_while_controller_runs() {
        let mut machine = Machine::new();
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G1 F60 X10\nG1 X20\n".to_string())
            .unwrap();
        machine.run_program().unwrap();
        while machine.snapshot().program.total_bytes_in_flight > 0 {
            machine.receive_controller_line("ok");
        }
        machine.receive_controller_line("<Run|MPos:0.000,0.000,0.000|Bf:15,128>");
        std::thread::sleep(std::time::Duration::from_millis(10));

        let snapshot = machine.snapshot();
        assert_eq!(snapshot.status.mode, MachineMode::Running);
        assert_eq!(snapshot.program.next_line, snapshot.program.lines.len());
        assert_eq!(snapshot.program.total_bytes_in_flight, 0);
        assert!(program_remaining_seconds(&snapshot) > 0.0);
    }

    fn request(cmd: &str, body: serde_json::Value) -> tauri::webview::InvokeRequest {
        tauri::webview::InvokeRequest {
            cmd: cmd.into(),
            callback: tauri::ipc::CallbackFn(0),
            error: tauri::ipc::CallbackFn(1),
            url: "tauri://localhost".parse().unwrap(),
            body: tauri::ipc::InvokeBody::Json(body),
            headers: Default::default(),
            invoke_key: INVOKE_KEY.to_string(),
        }
    }

    #[test]
    fn transcript_detection_accepts_grbl_and_grblhal_responses() {
        assert!(transcript_contains_grbl("Grbl 1.1h ['$' for help]\r\n"));
        assert!(transcript_contains_grbl(
            "[FIRMWARE:grblHAL]\n[VER:1.1f.20240506:]\n"
        ));
        assert!(transcript_contains_grbl("<Idle|MPos:0.000,0.000,0.000>"));
        assert!(!transcript_contains_grbl("ok\nerror:2\n"));
    }

    #[test]
    #[ignore = "requires a physical Grbl-compatible controller; set PUGSENDER_SERIAL_PORT"]
    fn auto_connect_finds_physical_grbl_controller() {
        let expected_port = std::env::var("PUGSENDER_SERIAL_PORT")
            .expect("set PUGSENDER_SERIAL_PORT to a Grbl serial device");
        let app = test_app();
        let webview = tauri::WebviewWindowBuilder::new(&app, "main", Default::default())
            .build()
            .unwrap();

        tauri::test::get_ipc_response(&webview, request("auto_connect", serde_json::json!({})))
            .expect("auto_connect command failed");

        let snapshot = wait_for_snapshot(&webview, |snapshot| snapshot.status.ready);
        assert!(snapshot.status.ready);
        assert_eq!(snapshot.status.port_name, expected_port);
        assert_eq!(
            snapshot.status.capabilities.version.as_deref(),
            Some("1.1h")
        );

        tauri::test::get_ipc_response(&webview, request("disconnect", serde_json::json!({})))
            .expect("disconnect command failed");
    }

    fn wait_for_snapshot(
        webview: &tauri::WebviewWindow<MockRuntime>,
        condition: impl Fn(&Snapshot) -> bool,
    ) -> Snapshot {
        let started = std::time::Instant::now();
        loop {
            let body = tauri::test::get_ipc_response(
                webview,
                request("get_snapshot", serde_json::json!({})),
            )
            .expect("get_snapshot command failed");
            let snapshot = body.deserialize::<Snapshot>().unwrap();
            if condition(&snapshot) || started.elapsed() > std::time::Duration::from_secs(5) {
                return snapshot;
            }
            std::thread::sleep(std::time::Duration::from_millis(50));
        }
    }

    #[test]
    fn ipc_commands_drive_machine_state() {
        let app = test_app();
        let webview = tauri::WebviewWindowBuilder::new(&app, "main", Default::default())
            .build()
            .unwrap();

        tauri::test::get_ipc_response(&webview, request("connect_sim", serde_json::json!({})))
            .expect("connect_sim command failed");
        let gcode_path =
            std::env::temp_dir().join(format!("pugsender-command-test-{}.nc", std::process::id()));
        std::fs::write(&gcode_path, "G0 X1\nG0 X2\n").expect("write temp gcode");

        tauri::test::get_ipc_response(
            &webview,
            request(
                "load_gcode_path",
                serde_json::json!({
                    "path": gcode_path.to_string_lossy()
                }),
            ),
        )
        .expect("load_gcode_path command failed");
        tauri::test::get_ipc_response(&webview, request("run_program", serde_json::json!({})))
            .expect("run_program command failed");

        let body =
            tauri::test::get_ipc_response(&webview, request("get_snapshot", serde_json::json!({})))
                .expect("get_snapshot command failed");
        let snapshot = body.deserialize::<Snapshot>().unwrap();
        assert_eq!(snapshot.status.mode, MachineMode::Idle);
        assert_eq!(snapshot.status.wpos.x, 2.0);
        assert_eq!(
            snapshot.status.capabilities.firmware.as_deref(),
            Some("grblHAL simulator")
        );
        assert_eq!(
            snapshot.settings.last_gcode_path.as_deref(),
            gcode_path.to_str()
        );
        let _ = std::fs::remove_file(gcode_path);
    }

    #[test]
    fn complete_path_lists_gcode_files_and_directories() {
        let root =
            std::env::temp_dir().join(format!("pugsender-complete-test-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&root);
        std::fs::create_dir_all(root.join("jobs")).expect("create jobs dir");
        std::fs::write(root.join("alpha.nc"), "G0 X1\n").expect("write nc");
        std::fs::write(root.join("alpha.md"), "# nope\n").expect("write md");
        std::fs::write(root.join("beta.tap"), "G0 X2\n").expect("write tap");

        let candidates = complete_path(&root.join("a").to_string_lossy(), 20);
        assert_eq!(candidates.len(), 1);
        assert_eq!(candidates[0].label, "alpha.nc");
        assert!(!candidates[0].is_dir);

        let candidates = complete_path(&root.join("").to_string_lossy(), 20);
        assert!(candidates
            .iter()
            .any(|candidate| candidate.label == "jobs/" && candidate.is_dir));
        assert!(candidates
            .iter()
            .any(|candidate| candidate.label == "beta.tap" && !candidate.is_dir));
        assert!(!candidates
            .iter()
            .any(|candidate| candidate.label == "alpha.md"));

        let _ = std::fs::remove_dir_all(root);
    }

    #[test]
    fn preview_path_reads_bounded_gcode_metadata() {
        let path =
            std::env::temp_dir().join(format!("pugsender-preview-test-{}.nc", std::process::id()));
        std::fs::write(&path, "G0 X1\nG0 X2\nG0 X3\n").expect("write preview file");

        let preview = preview_path(&path.to_string_lossy(), 2).expect("preview path");
        assert_eq!(
            preview.filename,
            path.file_name().unwrap().to_string_lossy()
        );
        assert_eq!(preview.line_count, 3);
        assert_eq!(preview.preview_lines, vec!["G0 X1", "G0 X2"]);
        assert!(preview.size_bytes > 0);

        let _ = std::fs::remove_file(path);
    }
}
