use crate::gcode::{build_toolpath, Toolpath};
use crate::grbl::{parse_line, GrblLine, ParsedStatus};
use serde::{Deserialize, Serialize};
use std::collections::VecDeque;
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Clone, Copy, Debug, Default, Deserialize, Serialize, PartialEq)]
pub struct Vec4 {
    pub x: f64,
    pub y: f64,
    pub z: f64,
    pub a: f64,
}

#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub enum Axis {
    X,
    Y,
    Z,
    A,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub enum MachineMode {
    Disconnected,
    Connecting,
    Idle,
    Jogging,
    Running,
    Hold,
    Draining,
    Alarm,
    Error,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct ControllerStatus {
    pub port_name: String,
    pub ready: bool,
    pub mode: MachineMode,
    pub axis_count: u8,
    pub capabilities: ControllerCapabilities,
    pub wpos: Vec4,
    pub mpos: Vec4,
    pub wco: Vec4,
    pub feed_rate: f64,
    pub spindle_speed: f64,
    pub feed_override: f64,
    pub rapid_override: f64,
    pub spindle_override: f64,
    pub planner_free: usize,
    pub serial_free: usize,
    pub serial_size: usize,
    pub probe: bool,
    pub pins: String,
    pub active_codes: String,
    pub last_error: Option<String>,
    pub last_alarm: Option<String>,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize, PartialEq, Eq)]
pub struct ControllerCapabilities {
    pub firmware: Option<String>,
    pub version: Option<String>,
    pub build: Option<String>,
    pub opt_flags: Vec<String>,
    pub newopt_flags: Vec<String>,
    pub axis_letters: String,
    pub driver: Option<String>,
    pub driver_version: Option<String>,
    pub board: Option<String>,
    pub signals: Option<String>,
    pub nvs_storage: Option<String>,
    pub aux_io: Option<String>,
    pub plugins: Vec<String>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct ProgramState {
    pub filename: Option<String>,
    pub lines: Vec<String>,
    pub next_line: usize,
    pub current_line: Option<usize>,
    pub queued_lines: Vec<usize>,
    pub running: bool,
    pub optional_stop: bool,
    pub pause_after_line: Option<usize>,
    pub tool_change_line: Option<usize>,
    pub run_started_line: usize,
    pub total_bytes_in_flight: usize,
    pub running_elapsed_seconds: f64,
    pub toolpath: Toolpath,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum ConsoleDirection {
    Tx,
    Rx,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct ConsoleEntry {
    pub direction: ConsoleDirection,
    pub line: String,
    pub at_ms: u128,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(default)]
pub struct AppSettings {
    pub jog_increment: f64,
    pub jog_feed: f64,
    pub rapid_jog: bool,
    pub toolpath_zoom: f64,
    pub left_panel_width: f64,
    pub sidebar_width: f64,
    pub show_console: bool,
    pub hide_state_updates: bool,
    pub keyboard_shortcuts_enabled: bool,
    pub keyboard_jog_requires_shift: bool,
    pub mdi_history: Vec<String>,
    pub last_port: Option<String>,
    pub last_gcode_path: Option<String>,
    pub saved_wpos: Vec4,
}

impl Default for AppSettings {
    fn default() -> Self {
        Self {
            jog_increment: 1.0,
            jog_feed: 300.0,
            rapid_jog: false,
            toolpath_zoom: 1.0,
            left_panel_width: 260.0,
            sidebar_width: 260.0,
            show_console: false,
            hide_state_updates: true,
            keyboard_shortcuts_enabled: true,
            keyboard_jog_requires_shift: false,
            mdi_history: Vec::new(),
            last_port: None,
            last_gcode_path: None,
            saved_wpos: Vec4::default(),
        }
    }
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct UiMessage {
    pub level: MessageLevel,
    pub text: String,
    pub at_ms: u128,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum MessageLevel {
    Info,
    Warn,
    Error,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct Snapshot {
    pub status: ControllerStatus,
    pub program: ProgramState,
    pub messages: Vec<UiMessage>,
    pub console: Vec<ConsoleEntry>,
    pub settings: AppSettings,
}

#[derive(Clone, Debug)]
struct InFlightLine {
    line: String,
    source_line: Option<usize>,
    bytes: usize,
    blocks_stream: bool,
}

#[derive(Clone, Debug)]
struct ActiveJog {
    axis: Axis,
    direction: i32,
    step: f64,
    feed: f64,
    target: f64,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum PendingRealtime {
    Hold,
    Resume,
}

pub struct Machine {
    status: ControllerStatus,
    program: ProgramState,
    outbox: Vec<String>,
    in_flight: VecDeque<InFlightLine>,
    startup_probe_responses_pending: usize,
    messages: Vec<UiMessage>,
    console: Vec<ConsoleEntry>,
    active_jog: Option<ActiveJog>,
    pending_realtime: Option<PendingRealtime>,
    cycle_active: bool,
    settings: AppSettings,
    run_started_at_ms: Option<u128>,
    accumulated_run_ms: u128,
}

impl Machine {
    pub fn new() -> Self {
        Self::new_with_settings(AppSettings::default())
    }

    pub fn new_with_settings(settings: AppSettings) -> Self {
        let saved_wpos = settings.saved_wpos;
        Self {
            status: ControllerStatus {
                port_name: "/dev/null".to_string(),
                ready: false,
                mode: MachineMode::Disconnected,
                axis_count: 3,
                capabilities: ControllerCapabilities::default(),
                wpos: saved_wpos,
                mpos: saved_wpos,
                wco: Vec4::default(),
                feed_rate: 0.0,
                spindle_speed: 0.0,
                feed_override: 100.0,
                rapid_override: 100.0,
                spindle_override: 100.0,
                planner_free: 15,
                serial_free: 128,
                serial_size: 128,
                probe: false,
                pins: String::new(),
                active_codes: "G0 G54 G17 G21 G90 G94 M5 M9 T0 F0 S0".to_string(),
                last_error: None,
                last_alarm: None,
            },
            program: ProgramState {
                filename: None,
                lines: Vec::new(),
                next_line: 0,
                current_line: None,
                queued_lines: Vec::new(),
                running: false,
                optional_stop: true,
                pause_after_line: None,
                tool_change_line: None,
                run_started_line: 0,
                total_bytes_in_flight: 0,
                running_elapsed_seconds: 0.0,
                toolpath: Toolpath::default(),
            },
            outbox: Vec::new(),
            in_flight: VecDeque::new(),
            startup_probe_responses_pending: 0,
            messages: Vec::new(),
            console: Vec::new(),
            active_jog: None,
            pending_realtime: None,
            cycle_active: false,
            settings,
            run_started_at_ms: None,
            accumulated_run_ms: 0,
        }
    }

    pub fn snapshot(&self) -> Snapshot {
        let mut program = self.program.clone();
        program.running_elapsed_seconds = self.running_elapsed_seconds();
        Snapshot {
            status: self.status.clone(),
            program,
            messages: self.messages.iter().rev().take(80).cloned().collect(),
            console: self
                .console
                .iter()
                .rev()
                .filter(|entry| {
                    !self.settings.hide_state_updates || !is_state_update_console_entry(entry)
                })
                .take(250)
                .cloned()
                .collect(),
            settings: self.settings.clone(),
        }
    }

    pub fn settings(&self) -> &AppSettings {
        &self.settings
    }

    fn start_run_timer(&mut self) {
        if self.run_started_at_ms.is_none() {
            self.run_started_at_ms = Some(now_ms());
        }
    }

    fn pause_run_timer(&mut self) {
        if let Some(started_at_ms) = self.run_started_at_ms.take() {
            self.accumulated_run_ms += now_ms().saturating_sub(started_at_ms);
        }
    }

    fn reset_run_timer(&mut self) {
        self.run_started_at_ms = None;
        self.accumulated_run_ms = 0;
        self.program.running_elapsed_seconds = 0.0;
    }

    fn running_elapsed_seconds(&self) -> f64 {
        let active_ms = self
            .run_started_at_ms
            .map(|started_at_ms| now_ms().saturating_sub(started_at_ms))
            .unwrap_or(0);
        (self.accumulated_run_ms + active_ms) as f64 / 1000.0
    }

    pub fn update_settings(&mut self, settings: AppSettings) {
        self.settings = AppSettings {
            jog_increment: settings.jog_increment.max(0.001),
            jog_feed: settings.jog_feed.max(1.0),
            toolpath_zoom: settings.toolpath_zoom.clamp(0.25, 8.0),
            left_panel_width: settings.left_panel_width.clamp(220.0, 700.0),
            sidebar_width: settings.sidebar_width.clamp(260.0, 2000.0),
            ..settings
        };
    }

    pub fn remember_port(&mut self, port_name: String) {
        self.settings.last_port = Some(port_name);
    }

    pub fn remember_gcode_path(&mut self, path: String) {
        self.settings.last_gcode_path = Some(path);
    }

    pub fn connect_sim(&mut self) {
        self.pending_realtime = None;
        self.cycle_active = false;
        self.status.port_name = "<sim>".to_string();
        self.status.ready = true;
        self.status.mode = MachineMode::Idle;
        self.status.axis_count = 4;
        self.status.capabilities = ControllerCapabilities {
            firmware: Some("grblHAL simulator".to_string()),
            version: Some("1.1f".to_string()),
            build: Some("sim".to_string()),
            opt_flags: split_csv("VNMSL"),
            newopt_flags: split_csv("ENUMS,RT+,HOME,SED"),
            axis_letters: "XYZA".to_string(),
            driver: Some("pugsender-sim".to_string()),
            ..ControllerCapabilities::default()
        };
        self.status.serial_free = self.status.serial_size;
        self.status.last_error = None;
        self.status.last_alarm = None;
        self.info("connected to simulator");
    }

    pub fn connect_port(&mut self, port_name: String) {
        self.pending_realtime = None;
        self.cycle_active = false;
        self.status.port_name = port_name.clone();
        self.status.ready = false;
        self.status.mode = MachineMode::Connecting;
        self.status.last_error = None;
        self.status.last_alarm = None;
        self.startup_probe_responses_pending = 0;
        self.info(format!("connecting to {port_name}"));
    }

    pub fn connection_failed(&mut self, port_name: String, error: String) {
        self.pending_realtime = None;
        self.cycle_active = false;
        self.status.port_name = port_name;
        self.status.ready = false;
        self.status.mode = MachineMode::Disconnected;
        self.status.last_error = Some(error.clone());
        self.startup_probe_responses_pending = 0;
        self.outbox.clear();
        self.error(error);
    }

    pub fn queue_startup_probe(&mut self) {
        self.startup_probe_responses_pending = 2;
        self.record_console(ConsoleDirection::Tx, "$I+");
        self.record_console(ConsoleDirection::Tx, "$I");
        self.outbox.push("$I+\n".to_string());
        self.outbox.push("$I\n".to_string());
    }

    pub fn disconnect(&mut self) {
        self.pending_realtime = None;
        self.cycle_active = false;
        self.reset_run_timer();
        self.status.ready = false;
        self.status.mode = MachineMode::Disconnected;
        self.status.port_name = "/dev/null".to_string();
        self.status.capabilities = ControllerCapabilities::default();
        self.program.running = false;
        self.program.tool_change_line = None;
        self.in_flight.clear();
        self.sync_in_flight_lines();
        self.program.total_bytes_in_flight = 0;
        self.status.last_error = None;
        self.status.last_alarm = None;
        self.info("disconnected");
    }

    pub fn take_outbox(&mut self) -> Vec<String> {
        std::mem::take(&mut self.outbox)
    }

    pub fn load_gcode_text(&mut self, filename: String, contents: String) -> Result<(), String> {
        if self.program_is_active() {
            return Err("cannot load G-code while a program is active".to_string());
        }
        self.program.filename = Some(filename);
        self.program.lines = contents.lines().map(|line| line.to_string()).collect();
        self.program.toolpath = build_toolpath(&self.program.lines);
        self.program.next_line = 0;
        self.program.running = false;
        self.program.pause_after_line = None;
        self.program.tool_change_line = None;
        self.program.run_started_line = 0;
        self.in_flight.clear();
        self.sync_in_flight_lines();
        self.program.total_bytes_in_flight = 0;
        self.reset_run_timer();
        self.info("loaded G-code");
        Ok(())
    }

    pub fn run_program(&mut self) -> Result<(), String> {
        self.require_ready()?;
        if self.status.mode != MachineMode::Idle {
            return Err("program can only be started while idle".to_string());
        }
        if self.program.lines.is_empty() {
            return Err("no G-code loaded".to_string());
        }
        if self.program.next_line >= self.program.lines.len() {
            self.program.next_line = 0;
            self.sync_in_flight_lines();
        }
        self.reset_run_timer();
        self.program.run_started_line = self.program.next_line;
        self.start_run_timer();
        self.program.running = true;
        self.cycle_active = true;
        self.program.tool_change_line = None;
        self.status.mode = MachineMode::Running;
        self.realtime("~");
        self.pump_stream();
        Ok(())
    }

    pub fn run_or_resume_program(&mut self) -> Result<(), String> {
        if matches!(self.status.mode, MachineMode::Hold | MachineMode::Draining)
            || self.program.tool_change_line.is_some()
        {
            self.resume_program()
        } else {
            self.run_program()
        }
    }

    pub fn run_from_line(&mut self, line: usize) -> Result<(), String> {
        self.require_ready()?;
        if self.program.lines.is_empty() {
            return Err("no G-code loaded".to_string());
        }
        if line >= self.program.lines.len() {
            return Err(format!("line {} is outside the loaded program", line + 1));
        }
        if self.program_is_active() {
            return Err("cannot change start line while running".to_string());
        }
        self.program.next_line = line;
        self.program.run_started_line = line;
        self.sync_in_flight_lines();
        self.reset_run_timer();
        self.info(format!("next run starts at line {}", line + 1));
        Ok(())
    }

    pub fn toggle_optional_stop(&mut self) {
        self.program.optional_stop = !self.program.optional_stop;
        if self.program.optional_stop {
            self.info("optional stop enabled");
        } else {
            self.info("optional stop disabled");
        }
    }

    pub fn set_pause_after_line(&mut self, line: Option<usize>) -> Result<(), String> {
        if self.program.lines.is_empty() {
            return Err("no G-code loaded".to_string());
        }
        if let Some(line) = line {
            if line >= self.program.lines.len() {
                return Err(format!("line {} is outside the loaded program", line + 1));
            }
            self.program.pause_after_line = Some(line);
            self.info(format!("pause after line {} armed", line + 1));
        } else {
            self.program.pause_after_line = None;
            self.info("pause-after-line cleared");
        }
        Ok(())
    }

    pub fn pause_program(&mut self) -> Result<(), String> {
        self.require_ready()?;
        if !self.program.running
            && !self.cycle_active
            && self.status.mode != MachineMode::Running
            && self.in_flight.is_empty()
        {
            return Err("no running program to hold".to_string());
        }
        self.program.running = false;
        self.pause_run_timer();
        self.status.mode = MachineMode::Hold;
        self.pending_realtime = Some(PendingRealtime::Hold);
        self.realtime("!");
        self.warn("feed hold requested");
        Ok(())
    }

    pub fn resume_program(&mut self) -> Result<(), String> {
        self.require_ready()?;
        if !matches!(self.status.mode, MachineMode::Hold | MachineMode::Draining)
            && self.program.tool_change_line.is_none()
        {
            return Err("program can only be resumed from hold, drain, or tool change".to_string());
        }
        if self.program.lines.is_empty() || self.program.next_line >= self.program.lines.len() {
            return Err("no remaining G-code to resume".to_string());
        }
        self.program.running = true;
        self.cycle_active = true;
        self.start_run_timer();
        self.program.tool_change_line = None;
        self.status.mode = MachineMode::Running;
        self.pending_realtime = Some(PendingRealtime::Resume);
        self.realtime("~");
        self.pump_stream();
        Ok(())
    }

    pub fn stop_program(&mut self) -> Result<(), String> {
        self.require_ready()?;
        if !matches!(
            self.status.mode,
            MachineMode::Running
                | MachineMode::Jogging
                | MachineMode::Hold
                | MachineMode::Draining
                | MachineMode::Alarm
                | MachineMode::Error
        ) && !self.program.running
        {
            return Err("no active program or alarm to stop".to_string());
        }
        self.program.running = false;
        self.cycle_active = false;
        self.reset_run_timer();
        self.program.next_line = 0;
        self.program.run_started_line = 0;
        self.program.pause_after_line = None;
        self.program.tool_change_line = None;
        self.in_flight.clear();
        self.sync_in_flight_lines();
        self.program.total_bytes_in_flight = 0;
        self.status.serial_free = self.status.serial_size;
        self.status.mode = MachineMode::Idle;
        self.pending_realtime = None;
        self.status.last_error = None;
        self.status.last_alarm = None;
        self.realtime("!");
        self.realtime("\u{18}");
        self.warn("program stopped and soft reset requested");
        Ok(())
    }

    pub fn drain_program(&mut self) -> Result<(), String> {
        self.require_ready()?;
        if !self.program.running
            && !matches!(self.status.mode, MachineMode::Running | MachineMode::Hold)
        {
            return Err("no running program to drain".to_string());
        }
        self.program.running = false;
        self.realtime("~");
        self.info("draining planner buffer");
        Ok(())
    }

    pub fn single_step(&mut self) -> Result<(), String> {
        self.require_ready()?;
        if self.status.mode != MachineMode::Idle {
            return Err("single step is only allowed while idle".to_string());
        }
        if !self.in_flight.is_empty() {
            return Err("single step is waiting for controller acknowledgement".to_string());
        }
        while self.program.next_line < self.program.lines.len() {
            if self.send_program_line() {
                if self.is_sim() {
                    self.sim_ack_all();
                }
                break;
            }
        }
        Ok(())
    }

    pub fn send_mdi(&mut self, line: String) -> Result<(), String> {
        self.require_ready()?;
        let line = normalize_line(&line);
        if line.is_empty() {
            return Ok(());
        }
        self.settings.mdi_history.push(line.clone());
        if self.settings.mdi_history.len() > 200 {
            let excess = self.settings.mdi_history.len() - 200;
            self.settings.mdi_history.drain(0..excess);
        }
        self.enqueue_priority(line.clone());
        if self.is_sim() {
            self.apply_simulated_line(&line);
        }
        self.info(format!("MDI injected: {line}"));
        Ok(())
    }

    pub fn jog_step(&mut self, axis: Axis, delta: f64, feed: f64) -> Result<(), String> {
        self.require_ready()?;
        if !matches!(self.status.mode, MachineMode::Idle | MachineMode::Jogging) {
            return Err("jogging is only allowed while idle or already jogging".to_string());
        }
        let mut target = self.status.wpos;
        match axis {
            Axis::X => target.x += delta,
            Axis::Y => target.y += delta,
            Axis::Z => target.z += delta,
            Axis::A => target.a += delta,
        }
        let line = format!(
            "$J=G91 {}{:.3} F{:.3}",
            axis_letter(axis),
            delta,
            feed.max(1.0)
        );
        self.enqueue_priority(line);
        if self.is_sim() {
            self.status.wpos = target;
            self.status.mpos = add(target, self.status.wco);
            self.status.mode = MachineMode::Jogging;
        }
        self.info("jog command sent");
        if self.is_sim() {
            self.status.mode = MachineMode::Idle;
        }
        Ok(())
    }

    pub fn start_jog(
        &mut self,
        axis: Axis,
        direction: i32,
        step: f64,
        feed: f64,
    ) -> Result<(), String> {
        self.require_ready()?;
        if !matches!(self.status.mode, MachineMode::Idle | MachineMode::Jogging) {
            return Err("jogging is only allowed while idle or already jogging".to_string());
        }
        if direction == 0 {
            return Ok(());
        }
        let step = step.abs().max(0.001);
        let feed = feed.max(1.0);
        let current = axis_value(self.status.wpos, axis);
        let distance = step * f64::from(direction.signum());
        let target = current + distance;
        let line = format!("$J=G91{}{:.3}F{:.3}", axis_letter(axis), distance, feed);
        self.enqueue_priority(line);
        self.active_jog = Some(ActiveJog {
            axis,
            direction: direction.signum(),
            step,
            feed,
            target,
        });
        if self.is_sim() {
            self.status.mode = MachineMode::Jogging;
        }
        self.info("incremental jog started");
        Ok(())
    }

    pub fn extend_jog(&mut self, axis: Axis, direction: i32, period_ms: u32) -> Result<(), String> {
        self.require_ready()?;
        let Some(mut jog) = self.active_jog.clone() else {
            return Ok(());
        };
        if jog.axis != axis || jog.direction != direction.signum() || direction == 0 {
            return Ok(());
        }
        let dist = jog.feed * (f64::from(period_ms) / 60_000.0);
        let increments = (dist / jog.step).ceil().max(1.0);
        jog.target += increments * jog.step * f64::from(jog.direction);
        let line = format!(
            "$J=G90{}{:.3}F{:.3}",
            axis_letter(jog.axis),
            jog.target,
            jog.feed
        );
        self.enqueue_priority(line);
        self.active_jog = Some(jog);
        if self.is_sim() {
            self.status.mode = MachineMode::Jogging;
        }
        Ok(())
    }

    pub fn stop_jog(&mut self) -> Result<(), String> {
        self.require_ready()?;
        if matches!(self.status.mode, MachineMode::Jogging | MachineMode::Idle) {
            self.realtime("\u{85}");
            if let Some(jog) = self.active_jog.take() {
                let pos = axis_value(self.status.wpos, jog.axis);
                if (pos - jog.target).abs() > 0.0005 {
                    let line = format!(
                        "$J=G90{}{:.3}F{:.3}",
                        axis_letter(jog.axis),
                        jog.target,
                        jog.feed
                    );
                    self.enqueue_priority(line);
                }
            }
            if self.is_sim() {
                self.status.mode = MachineMode::Idle;
            }
            self.info("jog cancel sent");
        }
        Ok(())
    }

    pub fn jog_to_xy(&mut self, x: f64, y: f64, feed: f64) -> Result<(), String> {
        self.require_ready()?;
        if !matches!(self.status.mode, MachineMode::Idle | MachineMode::Jogging) {
            return Err("jogging is only allowed while idle or already jogging".to_string());
        }
        let mut target = self.status.wpos;
        target.x = x;
        target.y = y;
        let line = format!("$J=G90X{:.3}Y{:.3}F{:.3}", x, y, feed.max(1.0));
        self.enqueue_priority(line);
        if self.is_sim() {
            self.status.wpos = target;
            self.status.mpos = add(target, self.status.wco);
            self.status.mode = MachineMode::Jogging;
        }
        self.info("XY jog command sent");
        if self.is_sim() {
            self.status.mode = MachineMode::Idle;
        }
        Ok(())
    }

    pub fn set_work_position(&mut self, values: Vec4) -> Result<(), String> {
        self.require_ready()?;
        if !matches!(self.status.mode, MachineMode::Idle | MachineMode::Hold) {
            return Err("work position can only be set while idle or in feed hold".to_string());
        }
        self.settings.saved_wpos = values;
        if self.is_sim() {
            self.status.wpos = values;
            self.status.wco = sub(self.status.mpos, values);
        }
        let mut line = format!("G10L20P1X{:.3}Y{:.3}Z{:.3}", values.x, values.y, values.z);
        if self.supports_axis('A') {
            line.push_str(&format!("A{:.3}", values.a));
        }
        self.enqueue_ordered(line, None);
        self.info("work position updated");
        Ok(())
    }

    pub fn unlock_alarm(&mut self) -> Result<(), String> {
        self.require_ready()?;
        self.enqueue_priority("$X".to_string());
        self.status.mode = MachineMode::Idle;
        self.status.last_alarm = None;
        self.warn("alarm unlock sent");
        Ok(())
    }

    pub fn adjust_feed_override(&mut self, delta: i32) -> Result<(), String> {
        self.require_ready()?;
        let command = match delta {
            0 => "\u{90}",
            d if d >= 10 => "\u{91}",
            d if d > 0 => "\u{93}",
            d if d <= -10 => "\u{92}",
            _ => "\u{94}",
        };
        self.realtime(command);
        if self.is_sim() {
            self.status.feed_override = if delta == 0 {
                100.0
            } else {
                self.status.feed_override + f64::from(delta)
            };
        }
        Ok(())
    }

    pub fn set_rapid_override(&mut self, value: i32) -> Result<(), String> {
        self.require_ready()?;
        let requested = if value < 38 {
            self.realtime("\u{97}");
            25.0
        } else if value < 68 {
            self.realtime("\u{96}");
            50.0
        } else {
            self.realtime("\u{95}");
            100.0
        };
        if self.is_sim() {
            self.status.rapid_override = requested;
        }
        Ok(())
    }

    pub fn adjust_spindle_override(&mut self, delta: i32) -> Result<(), String> {
        self.require_ready()?;
        let command = match delta {
            0 => "\u{99}",
            d if d >= 10 => "\u{9a}",
            d if d > 0 => "\u{9c}",
            d if d <= -10 => "\u{9b}",
            _ => "\u{9d}",
        };
        self.realtime(command);
        if self.is_sim() {
            self.status.spindle_override = if delta == 0 {
                100.0
            } else {
                self.status.spindle_override + f64::from(delta)
            };
        }
        Ok(())
    }

    pub fn receive_controller_line(&mut self, line: &str) {
        self.record_console(ConsoleDirection::Rx, line.trim_end_matches(['\r', '\n']));
        match parse_line(line) {
            GrblLine::Status(status) => self.apply_status(*status),
            GrblLine::ActiveCodes(codes) => self.status.active_codes = codes,
            GrblLine::Message(message) => self.info(format!("controller: {message}")),
            GrblLine::Alarm(alarm) => {
                self.program.running = false;
                self.cycle_active = false;
                self.pause_run_timer();
                self.status.mode = MachineMode::Alarm;
                self.status.last_alarm = Some(alarm.clone());
                self.error(alarm);
            }
            GrblLine::Error(error) => {
                if self.in_flight.is_empty() && self.consume_startup_probe_response() {
                    self.warn(format!("startup probe response: {error}"));
                    return;
                }
                let failed_line = self.ack_one();
                let error = describe_controller_error(&error, failed_line.as_ref());
                self.program.running = false;
                self.cycle_active = false;
                self.pause_run_timer();
                self.status.mode = MachineMode::Error;
                self.status.last_error = Some(error.clone());
                self.error(error);
            }
            GrblLine::Ok => {
                if self.in_flight.is_empty() && self.consume_startup_probe_response() {
                    return;
                }
                self.ack_one();
                self.pump_stream();
            }
            GrblLine::Config { .. } => {}
            GrblLine::Info { key, value } => self.apply_info_record(&key, &value),
            GrblLine::Other(line) => {
                if !line.is_empty() {
                    self.info(format!("controller: {line}"));
                }
            }
        }
    }

    fn apply_info_record(&mut self, key: &str, value: &str) {
        match key {
            "VER" => {
                let version = value.split(':').next().unwrap_or(value);
                let parts: Vec<&str> = version.split('.').collect();
                if parts.len() >= 3 {
                    self.status.capabilities.version = Some(parts[..2].join("."));
                    self.status.capabilities.build = Some(parts[2..].join("."));
                } else {
                    self.status.capabilities.version = Some(version.to_string());
                    self.status.capabilities.build = None;
                }
            }
            "OPT" => {
                let mut parts = value.split(',');
                self.status.capabilities.opt_flags =
                    parts.next().map(split_csv).unwrap_or_default();
                if let Some(axes) = parts.nth(2).and_then(|part| part.parse::<u8>().ok()) {
                    self.status.axis_count = axes.max(3);
                }
            }
            "AXS" => {
                let mut parts = value.split(':');
                if let Some(axes) = parts.next().and_then(|part| part.parse::<u8>().ok()) {
                    self.status.axis_count = axes.max(3);
                }
                if let Some(letters) = parts.next() {
                    self.status.capabilities.axis_letters = letters.to_string();
                }
            }
            "NEWOPT" => self.status.capabilities.newopt_flags = split_csv(value),
            "FIRMWARE" => self.status.capabilities.firmware = Some(value.to_string()),
            "DRIVER" => self.status.capabilities.driver = Some(value.to_string()),
            "DRIVER VERSION" => self.status.capabilities.driver_version = Some(value.to_string()),
            "BOARD" => self.status.capabilities.board = Some(value.to_string()),
            "SIGNALS" => self.status.capabilities.signals = Some(value.to_string()),
            "NVS STORAGE" => self.status.capabilities.nvs_storage = Some(value.to_string()),
            "AUX IO" => self.status.capabilities.aux_io = Some(value.to_string()),
            "PLUGIN"
                if !self
                    .status
                    .capabilities
                    .plugins
                    .iter()
                    .any(|item| item == value) =>
            {
                self.status.capabilities.plugins.push(value.to_string());
            }
            _ => {}
        }
    }

    fn require_ready(&self) -> Result<(), String> {
        if self.status.ready {
            Ok(())
        } else {
            Err("not connected".to_string())
        }
    }

    fn program_is_active(&self) -> bool {
        self.program.running
            || self.cycle_active
            || !self.in_flight.is_empty()
            || self.program.tool_change_line.is_some()
            || matches!(
                self.status.mode,
                MachineMode::Running
                    | MachineMode::Jogging
                    | MachineMode::Hold
                    | MachineMode::Draining
            )
    }

    fn pump_stream(&mut self) {
        while self.program.running
            && self.program.next_line < self.program.lines.len()
            && !self.stream_blocked_by_barrier()
        {
            if self.next_program_line_is_tool_change() {
                if !self.in_flight.is_empty() {
                    break;
                }
                self.start_tool_change_drain();
                break;
            }
            if self.next_program_line_needs_barrier() && !self.in_flight.is_empty() {
                break;
            }
            let next_bytes = self.next_program_line_serial_bytes();
            if next_bytes > self.status.serial_size {
                self.fail_stream(format!(
                    "line {} is too long for the controller serial buffer",
                    self.program.next_line + 1
                ));
                break;
            }
            if next_bytes > self.status.serial_free {
                break;
            }
            self.send_program_line();
            if self.is_sim() {
                self.sim_ack_all();
            }
        }

        if self.program.running
            && self.program.next_line >= self.program.lines.len()
            && self.in_flight.is_empty()
        {
            self.program.running = false;
            if self.is_sim() {
                self.cycle_active = false;
                self.pause_run_timer();
                self.status.mode = MachineMode::Idle;
                self.info("program complete");
            } else {
                self.status.mode = MachineMode::Running;
                self.info("program sent; waiting for controller idle");
            }
        }
    }

    fn send_program_line(&mut self) -> bool {
        let source_line = self.program.next_line;
        let mut line = normalize_line(&self.program.lines[self.program.next_line]);
        self.program.next_line += 1;
        let mut sent = false;
        if line == "M1" && self.program.optional_stop {
            line = "M0".to_string();
        }
        if !line.is_empty() {
            self.enqueue_ordered(line.clone(), Some(source_line));
            if self.is_sim() {
                self.apply_simulated_line(&line);
            }
            sent = true;
            if is_program_end_command(&line) {
                self.program.next_line = self.program.lines.len();
            }
        }
        if self.program.pause_after_line == Some(source_line) {
            self.program.pause_after_line = None;
            self.program.running = false;
            self.pause_run_timer();
            self.status.mode = MachineMode::Hold;
            self.realtime("!");
            self.warn(format!("paused after line {}", source_line + 1));
        }
        sent
    }

    fn enqueue_ordered(&mut self, line: String, source_line: Option<usize>) {
        let bytes = line.len() + 1;
        self.status.serial_free = self.status.serial_free.saturating_sub(bytes);
        self.program.total_bytes_in_flight += bytes;
        self.in_flight.push_back(InFlightLine {
            line: line.clone(),
            source_line,
            bytes,
            blocks_stream: is_eeprom_command(&line),
        });
        self.sync_in_flight_lines();
        self.record_console(ConsoleDirection::Tx, &line);
        self.outbox.push(format!("{line}\n"));
    }

    fn enqueue_priority(&mut self, line: String) {
        self.enqueue_ordered(line, None);
    }

    fn realtime(&mut self, command: &str) {
        self.record_console(ConsoleDirection::Tx, command);
        self.outbox.push(command.to_string());
    }

    fn sim_ack_all(&mut self) {
        while let Some(sent) = self.in_flight.pop_front() {
            self.status.serial_free =
                (self.status.serial_free + sent.bytes).min(self.status.serial_size);
            self.program.total_bytes_in_flight = self
                .program
                .total_bytes_in_flight
                .saturating_sub(sent.bytes);
            self.sync_in_flight_lines();
            if sent.line.starts_with("error") {
                self.status.mode = MachineMode::Error;
                self.error(format!("controller error for {}", sent.line));
            }
        }
    }

    fn ack_one(&mut self) -> Option<InFlightLine> {
        let Some(sent) = self.in_flight.pop_front() else {
            return None;
        };
        self.status.serial_free =
            (self.status.serial_free + sent.bytes).min(self.status.serial_size);
        self.program.total_bytes_in_flight = self
            .program
            .total_bytes_in_flight
            .saturating_sub(sent.bytes);
        self.sync_in_flight_lines();
        Some(sent)
    }

    fn sync_in_flight_lines(&mut self) {
        self.program.current_line = self.in_flight.front().and_then(|line| line.source_line);
        self.program.queued_lines = self
            .in_flight
            .iter()
            .filter_map(|line| line.source_line)
            .collect();
    }

    fn effective_serial_free(&self, reported_serial_free: usize) -> usize {
        let accounted_free = self
            .status
            .serial_size
            .saturating_sub(self.program.total_bytes_in_flight);
        reported_serial_free.min(accounted_free)
    }

    fn stream_blocked_by_barrier(&self) -> bool {
        self.in_flight.iter().any(|line| line.blocks_stream)
    }

    fn consume_startup_probe_response(&mut self) -> bool {
        if self.startup_probe_responses_pending == 0 {
            return false;
        }
        self.startup_probe_responses_pending -= 1;
        true
    }

    fn fail_stream(&mut self, error: String) {
        self.program.running = false;
        self.cycle_active = false;
        self.status.mode = MachineMode::Error;
        self.status.last_error = Some(error.clone());
        self.error(error);
    }

    fn next_program_line_needs_barrier(&self) -> bool {
        self.program
            .lines
            .get(self.program.next_line)
            .map(|line| is_eeprom_command(&normalize_line(line)))
            .unwrap_or(false)
    }

    fn next_program_line_serial_bytes(&self) -> usize {
        self.program
            .lines
            .get(self.program.next_line)
            .map(|line| {
                let mut line = normalize_line(line);
                if line == "M1" && self.program.optional_stop {
                    line = "M0".to_string();
                }
                if line.is_empty() {
                    0
                } else {
                    line.len() + 1
                }
            })
            .unwrap_or(0)
    }

    fn next_program_line_is_tool_change(&self) -> bool {
        self.program
            .lines
            .get(self.program.next_line)
            .map(|line| is_tool_change_command(&normalize_line(line)))
            .unwrap_or(false)
    }

    fn start_tool_change_drain(&mut self) {
        let source_line = self.program.next_line;
        let line = normalize_line(&self.program.lines[source_line]);
        self.program.next_line += 1;
        self.program.running = false;
        self.program.tool_change_line = Some(source_line);
        if self.is_sim() || self.status.mode == MachineMode::Idle {
            self.pause_run_timer();
            self.status.mode = MachineMode::Hold;
            self.warn(format!("tool change ready at line {}", source_line + 1));
        } else {
            self.status.mode = MachineMode::Draining;
            self.warn(format!(
                "draining before tool change at line {}",
                source_line + 1
            ));
        }
            self.record_console(ConsoleDirection::Tx, &format!("SKIP {line}"));
    }

    fn apply_status(&mut self, parsed: ParsedStatus) {
        self.status.ready = true;
        self.status.mode = match (self.pending_realtime, parsed.mode) {
            (Some(PendingRealtime::Hold), MachineMode::Hold) => {
                self.pending_realtime = None;
                MachineMode::Hold
            }
            (Some(PendingRealtime::Hold), MachineMode::Running) => MachineMode::Hold,
            (Some(PendingRealtime::Resume), MachineMode::Running) => {
                self.pending_realtime = None;
                MachineMode::Running
            }
            (Some(PendingRealtime::Resume), MachineMode::Hold) => MachineMode::Running,
            (_, mode) => {
                self.pending_realtime = None;
                mode
            }
        };
        self.apply_cycle_status();
        if self.status.mode != MachineMode::Alarm {
            self.status.last_alarm = None;
        }
        if self.status.mode != MachineMode::Error {
            self.status.last_error = None;
        }
        if let Some(axis_count) = parsed.axis_count {
            self.status.axis_count = axis_count.max(3);
        }
        if let Some(wco) = parsed.wco {
            self.status.wco = wco;
        }
        if let Some(wpos) = parsed.wpos {
            self.status.wpos = wpos;
            self.status.mpos = add(wpos, self.status.wco);
        }
        if let Some(mpos) = parsed.mpos {
            self.status.mpos = mpos;
            self.status.wpos = sub(mpos, self.status.wco);
        }
        if let Some(feed_rate) = parsed.feed_rate {
            self.status.feed_rate = feed_rate;
        }
        if let Some(spindle_speed) = parsed.spindle_speed {
            self.status.spindle_speed = spindle_speed;
        }
        if let Some(feed_override) = parsed.feed_override {
            self.status.feed_override = feed_override;
        }
        if let Some(rapid_override) = parsed.rapid_override {
            self.status.rapid_override = rapid_override;
        }
        if let Some(spindle_override) = parsed.spindle_override {
            self.status.spindle_override = spindle_override;
        }
        if let Some(planner_free) = parsed.planner_free {
            self.status.planner_free = planner_free;
        }
        if let Some(serial_free) = parsed.serial_free {
            self.status.serial_size = self.status.serial_size.max(serial_free);
            self.status.serial_free = self.effective_serial_free(serial_free);
        }
        if let Some(pins) = parsed.pins {
            self.status.probe = pins.contains('P');
            self.status.pins = pins;
        } else {
            self.status.probe = false;
            self.status.pins.clear();
        }
        if self.program.tool_change_line.is_some() && self.status.mode == MachineMode::Idle {
            self.status.mode = MachineMode::Hold;
        }
        if matches!(
            self.status.mode,
            MachineMode::Idle | MachineMode::Hold | MachineMode::Alarm | MachineMode::Error
        ) && !self.program.running
        {
            self.pause_run_timer();
        }
    }

    fn apply_cycle_status(&mut self) {
        if !self.cycle_active {
            return;
        }
        match self.status.mode {
            MachineMode::Idle => {
                self.program.running = false;
                self.cycle_active = false;
                self.pause_run_timer();
                if self.program.next_line >= self.program.lines.len() && self.in_flight.is_empty() {
                    self.info("program complete");
                }
            }
            MachineMode::Hold | MachineMode::Draining => {
                self.pause_run_timer();
            }
            MachineMode::Running => {
                if self.program.running {
                    self.start_run_timer();
                }
            }
            MachineMode::Alarm | MachineMode::Error | MachineMode::Disconnected => {
                self.program.running = false;
                self.cycle_active = false;
                self.pause_run_timer();
            }
            MachineMode::Connecting | MachineMode::Jogging => {}
        }
    }

    fn is_sim(&self) -> bool {
        self.status.port_name == "<sim>"
    }

    fn supports_axis(&self, axis: char) -> bool {
        if self.status.capabilities.axis_letters.contains(axis) {
            return true;
        }
        axis == 'A' && self.status.axis_count >= 4
    }

    fn apply_simulated_line(&mut self, line: &str) {
        for (letter, value) in parse_words(line) {
            match letter {
                'X' => self.status.wpos.x = value,
                'Y' => self.status.wpos.y = value,
                'Z' => self.status.wpos.z = value,
                'A' => self.status.wpos.a = value,
                'F' => self.status.feed_rate = value,
                'S' => self.status.spindle_speed = value,
                _ => {}
            }
        }
        self.status.mpos = add(self.status.wpos, self.status.wco);
    }

    fn info(&mut self, text: impl Into<String>) {
        self.message(MessageLevel::Info, text);
    }

    fn warn(&mut self, text: impl Into<String>) {
        self.message(MessageLevel::Warn, text);
    }

    fn error(&mut self, text: impl Into<String>) {
        self.message(MessageLevel::Error, text);
    }

    fn message(&mut self, level: MessageLevel, text: impl Into<String>) {
        self.messages.push(UiMessage {
            level,
            text: text.into(),
            at_ms: now_ms(),
        });
    }

    fn record_console(&mut self, direction: ConsoleDirection, line: &str) {
        let line = if direction == ConsoleDirection::Rx {
            annotate_grbl_setting(line)
        } else {
            line.to_string()
        };
        self.console.push(ConsoleEntry {
            direction,
            line,
            at_ms: now_ms(),
        });
        if self.console.len() > 1000 {
            let excess = self.console.len() - 1000;
            self.console.drain(0..excess);
        }
    }
}

impl Default for Machine {
    fn default() -> Self {
        Self::new()
    }
}

fn normalize_line(line: &str) -> String {
    strip_comments(line).trim().to_ascii_uppercase()
}

fn annotate_grbl_setting(line: &str) -> String {
    let Some((key, _value)) = line.strip_prefix('$').and_then(|rest| rest.split_once('=')) else {
        return line.to_string();
    };
    let Ok(key) = key.parse::<u32>() else {
        return line.to_string();
    };
    let Some(description) = grbl_setting_description(key) else {
        return line.to_string();
    };
    format!("{line} # {description}")
}

fn grbl_setting_description(key: u32) -> Option<String> {
    if let Some((axis, description)) = axis_setting_description(key) {
        return Some(format!("{axis} {description}"));
    }
    let description = match key {
        0 => "step pulse, usec",
        1 => "step idle delay, msec",
        2 => "step port invert mask",
        3 => "direction port invert mask",
        4 => "step enable invert",
        5 => "limit pins invert",
        6 => "probe pin invert",
        10 => "status report mask",
        11 => "junction deviation, mm",
        12 => "arc tolerance, mm",
        13 => "report inches",
        20 => "soft limits enable",
        21 => "hard limits enable",
        22 => "homing cycle enable",
        23 => "homing direction invert mask",
        24 => "homing feed, mm/min",
        25 => "homing seek, mm/min",
        26 => "homing debounce, msec",
        27 => "homing pull-off, mm",
        30 => "max spindle speed, rpm",
        31 => "min spindle speed, rpm",
        32 => "laser mode enable",
        _ => return None,
    };
    Some(description.to_string())
}

fn axis_setting_description(key: u32) -> Option<(char, &'static str)> {
    const AXES: [char; 9] = ['X', 'Y', 'Z', 'A', 'B', 'C', 'U', 'V', 'W'];
    let (base, description) = match key {
        100..=108 => (100, "steps/mm"),
        110..=118 => (110, "max rate, mm/min"),
        120..=128 => (120, "acceleration, mm/sec^2"),
        130..=138 => (130, "max travel, mm"),
        _ => return None,
    };
    AXES.get((key - base) as usize).copied().map(|axis| (axis, description))
}

fn parse_words(line: &str) -> Vec<(char, f64)> {
    let line = normalize_line(line);
    let bytes = line.as_bytes();
    let mut words = Vec::new();
    let mut i = 0;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if !ch.is_ascii_alphabetic() {
            i += 1;
            continue;
        }
        i += 1;
        let start = i;
        while i < bytes.len() {
            let next = bytes[i] as char;
            if next.is_ascii_digit() || matches!(next, '.' | '-' | '+') {
                i += 1;
            } else {
                break;
            }
        }
        if let Ok(value) = line[start..i].parse::<f64>() {
            words.push((ch, value));
        }
    }

    words
}

fn is_eeprom_command(line: &str) -> bool {
    let line = normalize_line(line);
    if line.is_empty() {
        return false;
    }
    if line.starts_with('$') {
        return line.contains('=') || line.starts_with("$N") || line.starts_with("$RST=");
    }

    let words = parse_words(&line);
    let mut has_g10 = false;
    let mut g28_1 = false;
    let mut g30_1 = false;
    let mut l_value = None;
    for (letter, value) in words {
        if letter == 'G' && approx(value, 10.0) {
            has_g10 = true;
        } else if letter == 'G' && approx(value, 28.1) {
            g28_1 = true;
        } else if letter == 'G' && approx(value, 30.1) {
            g30_1 = true;
        } else if letter == 'L' {
            l_value = Some(value.round() as i32);
        }
    }
    g28_1 || g30_1 || (has_g10 && matches!(l_value, Some(2 | 20)))
}

fn is_tool_change_command(line: &str) -> bool {
    parse_words(line)
        .iter()
        .any(|(letter, value)| *letter == 'M' && approx(*value, 6.0))
}

fn is_program_end_command(line: &str) -> bool {
    parse_words(line)
        .iter()
        .any(|(letter, value)| *letter == 'M' && (approx(*value, 2.0) || approx(*value, 30.0)))
}

fn describe_controller_error(error: &str, failed_line: Option<&InFlightLine>) -> String {
    match failed_line.and_then(|line| line.source_line.map(|source_line| (source_line, line))) {
        Some((source_line, line)) => format!("{error} at line {}: {}", source_line + 1, line.line),
        None => error.to_string(),
    }
}

fn approx(a: f64, b: f64) -> bool {
    (a - b).abs() < 0.0001
}

fn strip_comments(line: &str) -> String {
    let mut out = String::new();
    let mut paren_depth = 0;
    for ch in line.chars() {
        if ch == ';' && paren_depth == 0 {
            break;
        }
        if ch == '(' {
            paren_depth += 1;
            continue;
        }
        if ch == ')' && paren_depth > 0 {
            paren_depth -= 1;
            continue;
        }
        if paren_depth == 0 {
            out.push(ch);
        }
    }
    out
}

fn split_csv(text: &str) -> Vec<String> {
    text.split(',')
        .map(str::trim)
        .filter(|part| !part.is_empty())
        .map(str::to_string)
        .collect()
}

fn axis_letter(axis: Axis) -> &'static str {
    match axis {
        Axis::X => "X",
        Axis::Y => "Y",
        Axis::Z => "Z",
        Axis::A => "A",
    }
}

fn add(a: Vec4, b: Vec4) -> Vec4 {
    Vec4 {
        x: a.x + b.x,
        y: a.y + b.y,
        z: a.z + b.z,
        a: a.a + b.a,
    }
}

fn sub(a: Vec4, b: Vec4) -> Vec4 {
    Vec4 {
        x: a.x - b.x,
        y: a.y - b.y,
        z: a.z - b.z,
        a: a.a - b.a,
    }
}

fn axis_value(v: Vec4, axis: Axis) -> f64 {
    match axis {
        Axis::X => v.x,
        Axis::Y => v.y,
        Axis::Z => v.z,
        Axis::A => v.a,
    }
}

fn now_ms() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis())
        .unwrap_or_default()
}

fn is_state_update_console_entry(entry: &ConsoleEntry) -> bool {
    let line = entry.line.trim();
    line == "?" || (line.starts_with('<') && line.ends_with('>'))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn completed_program_restarts_from_beginning() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\n".to_string())
            .unwrap();
        machine.run_program().unwrap();
        assert_eq!(machine.program.next_line, 2);
        assert!(!machine.program.running);
        machine.run_program().unwrap();
        assert_eq!(machine.program.next_line, 2);
    }

    #[test]
    fn serialized_snapshot_includes_toolpath_timing_arrays() {
        let mut machine = Machine::new();
        machine
            .load_gcode_text("test.nc".to_string(), "G1 F60 X1\nG1 X3\n".to_string())
            .unwrap();
        let value = serde_json::to_value(machine.snapshot()).unwrap();
        let toolpath = &value["program"]["toolpath"];
        assert_eq!(
            toolpath["line_estimated_seconds"].as_array().unwrap().len(),
            2
        );
        assert_eq!(
            toolpath["remaining_seconds_by_line"]
                .as_array()
                .unwrap()
                .len(),
            3
        );
        assert!(toolpath["remaining_seconds_by_line"][0].as_f64().unwrap() > 0.0);
    }

    #[test]
    fn mdi_is_allowed_while_running() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G4 P1\n".to_string())
            .unwrap();
        machine.program.running = true;
        machine.status.mode = MachineMode::Running;
        machine.send_mdi("m3 s12000".to_string()).unwrap();
        assert_eq!(machine.status.spindle_speed, 12000.0);
    }

    #[test]
    fn jog_to_xy_sends_absolute_xy_jog() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.status.wpos.z = -3.0;

        machine.jog_to_xy(12.3456, -7.0, 250.0).unwrap();

        assert_eq!(machine.status.wpos.x, 12.3456);
        assert_eq!(machine.status.wpos.y, -7.0);
        assert_eq!(machine.status.wpos.z, -3.0);
        assert!(machine.console.iter().any(|entry| {
            entry.direction == ConsoleDirection::Tx && entry.line == "$J=G90X12.346Y-7.000F250.000"
        }));
    }

    #[test]
    fn real_jog_to_xy_waits_for_controller_position_feedback() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");

        machine.jog_to_xy(12.0, -7.0, 250.0).unwrap();

        assert_eq!(machine.status.wpos.x, 0.0);
        assert_eq!(machine.status.wpos.y, 0.0);
        assert_eq!(
            machine.take_outbox(),
            vec!["$J=G90X12.000Y-7.000F250.000\n".to_string()]
        );
    }

    #[test]
    fn held_jog_starts_with_one_increment_then_extends_until_cancelled() {
        let mut machine = Machine::new();
        machine.connect_sim();

        machine.start_jog(Axis::X, -1, 1.0, 600.0).unwrap();

        assert_eq!(machine.status.mode, MachineMode::Jogging);
        assert_eq!(machine.take_outbox(), vec!["$J=G91X-1.000F600.000\n"]);

        machine.extend_jog(Axis::X, -1, 100).unwrap();
        assert_eq!(machine.take_outbox(), vec!["$J=G90X-2.000F600.000\n"]);

        machine.stop_jog().unwrap();

        assert_eq!(machine.status.mode, MachineMode::Idle);
        assert_eq!(
            machine.take_outbox(),
            vec!["\u{85}".to_string(), "$J=G90X-2.000F600.000\n".to_string()]
        );
    }

    #[test]
    fn controller_status_updates_snapshot() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.receive_controller_line(
            "<Idle|MPos:2.000,3.000,4.000,5.000|WCO:1.000,1.000,1.000,1.000|FS:300,6000|Bf:14,126|Pn:P>",
        );
        assert_eq!(machine.status.axis_count, 4);
        assert_eq!(machine.status.wpos.x, 1.0);
        assert_eq!(machine.status.wpos.a, 4.0);
        assert_eq!(machine.status.probe, true);
        assert_eq!(machine.status.serial_free, 126);
    }

    #[test]
    fn real_mdi_waits_for_controller_position_feedback() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");

        machine.send_mdi("G0 X5".to_string()).unwrap();

        assert_eq!(machine.status.wpos.x, 0.0);
        assert_eq!(machine.take_outbox(), vec!["G0 X5\n".to_string()]);
    }

    #[test]
    fn grblhal_info_updates_capabilities() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.receive_controller_line("[VER:1.1f.20240506:]");
        machine.receive_controller_line("[OPT:VNHS,100,1024,3,0]");
        machine.receive_controller_line("[AXS:3:XYZ]");
        machine.receive_controller_line("[NEWOPT:ENUMS,RT+,HOME,NOPROBE,ES,SED,RTC,TMC=7]");
        machine.receive_controller_line("[FIRMWARE:grblHAL]");
        machine.receive_controller_line("[DRIVER:RP2040]");
        machine.receive_controller_line("[DRIVER VERSION:240408]");
        machine.receive_controller_line("[BOARD:BTT SKR Pico 1.0]");
        machine.receive_controller_line("[SIGNALS:HSE]");
        machine.receive_controller_line("[NVS STORAGE:*FLASH]");
        machine.receive_controller_line("[AUX IO:2,0,0,0]");
        machine.receive_controller_line("[PLUGIN:Trinamic v0.16]");
        machine.receive_controller_line("[PLUGIN:Trinamic v0.16]");

        let capabilities = &machine.status.capabilities;
        assert_eq!(capabilities.firmware.as_deref(), Some("grblHAL"));
        assert_eq!(capabilities.version.as_deref(), Some("1.1f"));
        assert_eq!(capabilities.build.as_deref(), Some("20240506"));
        assert_eq!(capabilities.axis_letters, "XYZ");
        assert_eq!(machine.status.axis_count, 3);
        assert!(capabilities.newopt_flags.contains(&"RT+".to_string()));
        assert_eq!(capabilities.driver.as_deref(), Some("RP2040"));
        assert_eq!(capabilities.driver_version.as_deref(), Some("240408"));
        assert_eq!(capabilities.board.as_deref(), Some("BTT SKR Pico 1.0"));
        assert_eq!(capabilities.signals.as_deref(), Some("HSE"));
        assert_eq!(capabilities.nvs_storage.as_deref(), Some("*FLASH"));
        assert_eq!(capabilities.aux_io.as_deref(), Some("2,0,0,0"));
        assert_eq!(capabilities.plugins, vec!["Trinamic v0.16".to_string()]);
    }

    #[test]
    fn grblhal_six_axis_info_updates_axis_count_and_letters() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.receive_controller_line("[OPT:VNMSL,35,512,6,0]");
        machine.receive_controller_line("[AXS:6:XYZABC]");
        assert_eq!(machine.status.axis_count, 6);
        assert_eq!(machine.status.capabilities.axis_letters, "XYZABC");
    }

    #[test]
    fn controller_error_stops_program() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.program.running = true;
        machine.status.mode = MachineMode::Running;
        machine.receive_controller_line("error:2");
        assert!(!machine.program.running);
        assert_eq!(machine.status.mode, MachineMode::Error);
        assert_eq!(machine.status.last_error, Some("error:2".to_string()));
    }

    #[test]
    fn controller_error_reports_failed_source_line() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG2 X2 I0\n".to_string())
            .unwrap();

        machine.run_program().unwrap();
        machine.receive_controller_line("ok");
        machine.receive_controller_line("error:24");

        assert!(!machine.program.running);
        assert_eq!(machine.status.mode, MachineMode::Error);
        assert_eq!(
            machine.status.last_error.as_deref(),
            Some("error:24 at line 2: G2 X2 I0")
        );
    }

    #[test]
    fn command_state_guards_reject_invalid_program_actions() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\n".to_string())
            .unwrap();

        machine.status.mode = MachineMode::Error;
        assert!(machine.run_program().is_err());

        machine.status.mode = MachineMode::Idle;
        assert!(machine.pause_program().is_err());
        assert!(machine.resume_program().is_err());
        assert!(machine.drain_program().is_err());

        machine.status.mode = MachineMode::Hold;
        assert!(machine.single_step().is_err());
    }

    #[test]
    fn hold_is_allowed_when_motion_is_still_in_flight() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\n".to_string())
            .unwrap();
        machine.run_program().unwrap();
        machine.program.running = false;
        machine.status.mode = MachineMode::Idle;

        machine.pause_program().unwrap();

        assert_eq!(machine.status.mode, MachineMode::Hold);
        assert!(machine.take_outbox().contains(&"!".to_string()));
    }

    #[test]
    fn run_or_resume_uses_current_backend_mode() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\n".to_string())
            .unwrap();

        machine.run_or_resume_program().unwrap();
        assert_eq!(machine.status.mode, MachineMode::Running);
        let _ = machine.take_outbox();

        machine.pause_program().unwrap();
        assert_eq!(machine.status.mode, MachineMode::Hold);
        machine.receive_controller_line("<Run|MPos:0.000,0.000,0.000|Bf:15,128>");
        assert_eq!(machine.status.mode, MachineMode::Hold);

        machine.program.next_line = 1;
        machine.run_or_resume_program().unwrap();
        assert_eq!(machine.status.mode, MachineMode::Running);
        machine.receive_controller_line("<Hold|MPos:0.000,0.000,0.000|Bf:15,128>");
        assert_eq!(machine.status.mode, MachineMode::Running);
        assert!(machine.take_outbox().contains(&"~".to_string()));
    }

    #[test]
    fn drain_does_not_fake_controller_mode_when_running() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.program.running = false;
        machine.status.mode = MachineMode::Running;

        machine.drain_program().unwrap();

        assert_eq!(machine.status.mode, MachineMode::Running);
        assert!(!machine.program.running);
    }

    #[test]
    fn drain_accepts_hold_even_after_controller_acked_sender_queue() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\n".to_string())
            .unwrap();

        machine.run_program().unwrap();
        machine.receive_controller_line("ok");
        machine.receive_controller_line("ok");
        machine.status.mode = MachineMode::Hold;
        machine.program.running = false;
        assert!(machine.in_flight.is_empty());

        machine.drain_program().unwrap();

        assert_eq!(machine.status.mode, MachineMode::Hold);
        assert!(!machine.program.running);
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "~"));
    }

    #[test]
    fn stop_is_allowed_for_alarm_recovery() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.status.mode = MachineMode::Alarm;
        machine.status.last_alarm = Some("ALARM:1".to_string());

        machine.stop_program().unwrap();

        assert_eq!(machine.status.mode, MachineMode::Idle);
        assert_eq!(machine.status.last_alarm, None);
        assert_eq!(machine.status.last_error, None);
        assert!(machine.outbox.iter().any(|command| command == "\u{18}"));
    }

    #[test]
    fn normal_status_clears_stale_fault_messages() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.status.last_alarm = Some("ALARM:1".to_string());
        machine.status.last_error = Some("error:2".to_string());

        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|WCO:0.000,0.000,0.000>");

        assert_eq!(machine.status.mode, MachineMode::Idle);
        assert_eq!(machine.status.last_alarm, None);
        assert_eq!(machine.status.last_error, None);
    }

    #[test]
    fn startup_probe_error_does_not_mark_classic_grbl_connection_failed() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.queue_startup_probe();
        assert_eq!(
            machine.take_outbox(),
            vec!["$I+\n".to_string(), "$I\n".to_string()]
        );

        machine.receive_controller_line("error:2");
        machine.receive_controller_line("[VER:1.1h.20190830:]");
        machine.receive_controller_line("[OPT:V,15,128]");
        machine.receive_controller_line("ok");
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");

        assert_eq!(machine.status.mode, MachineMode::Idle);
        assert_eq!(machine.status.last_error, None);
        assert_eq!(machine.status.capabilities.version.as_deref(), Some("1.1h"));
        assert!(machine.messages.iter().any(|message| {
            message.level == MessageLevel::Warn && message.text == "startup probe response: error:2"
        }));
    }

    #[test]
    fn connection_failed_clears_probe_outbox_and_reports_error() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/missing".to_string());
        machine.queue_startup_probe();

        machine.connection_failed("/dev/missing".to_string(), "open failed".to_string());

        assert_eq!(machine.status.mode, MachineMode::Disconnected);
        assert!(!machine.status.ready);
        assert_eq!(machine.status.port_name, "/dev/missing");
        assert_eq!(machine.status.last_error.as_deref(), Some("open failed"));
        assert!(machine.take_outbox().is_empty());
    }

    #[test]
    fn non_probe_error_still_marks_machine_error() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");

        machine.receive_controller_line("error:2");

        assert_eq!(machine.status.mode, MachineMode::Error);
        assert_eq!(machine.status.last_error, Some("error:2".to_string()));
    }

    #[test]
    fn run_from_line_sets_next_line() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\nG0 X3\n".to_string())
            .unwrap();
        machine.run_from_line(1).unwrap();
        assert_eq!(machine.program.next_line, 1);
        machine.single_step().unwrap();
        assert_eq!(machine.status.wpos.x, 2.0);
    }

    #[test]
    fn real_stream_reports_current_and_queued_source_lines() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\n\nG0 X2\nG0 X3\n".to_string())
            .unwrap();

        machine.run_program().unwrap();

        assert_eq!(machine.program.next_line, 4);
        assert_eq!(machine.program.current_line, Some(0));
        assert_eq!(machine.program.queued_lines, vec![0, 2, 3]);

        machine.receive_controller_line("ok");
        assert_eq!(machine.program.current_line, Some(2));
        assert_eq!(machine.program.queued_lines, vec![2, 3]);

        machine.receive_controller_line("ok");
        machine.receive_controller_line("ok");
        assert_eq!(machine.program.current_line, None);
        assert!(machine.program.queued_lines.is_empty());
    }

    #[test]
    fn loading_gcode_is_rejected_while_real_stream_has_in_flight_lines() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("first.nc".to_string(), "G0 X1\nG0 X2\n".to_string())
            .unwrap();

        machine.run_program().unwrap();
        let original_queued = machine.program.queued_lines.clone();
        let original_in_flight = machine.program.total_bytes_in_flight;

        let error = machine
            .load_gcode_text("second.nc".to_string(), "G0 X99\n".to_string())
            .unwrap_err();

        assert_eq!(error, "cannot load G-code while a program is active");
        assert_eq!(machine.program.filename.as_deref(), Some("first.nc"));
        assert_eq!(
            machine.program.lines,
            vec!["G0 X1".to_string(), "G0 X2".to_string()]
        );
        assert_eq!(machine.program.queued_lines, original_queued);
        assert_eq!(machine.program.total_bytes_in_flight, original_in_flight);
        assert!(machine.program.running);
    }

    #[test]
    fn run_from_line_is_rejected_while_held_with_in_flight_lines() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("first.nc".to_string(), "G0 X1\nG0 X2\nG0 X3\n".to_string())
            .unwrap();

        machine.run_program().unwrap();
        let original_next_line = machine.program.next_line;
        machine.pause_program().unwrap();
        assert!(!machine.program.running);
        assert!(!machine.in_flight.is_empty());

        let error = machine.run_from_line(2).unwrap_err();

        assert_eq!(error, "cannot change start line while running");
        assert_eq!(machine.program.next_line, original_next_line);
    }

    #[test]
    fn real_stream_stops_queueing_after_program_end() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nM30\nG0 X99\n".to_string())
            .unwrap();

        machine.run_program().unwrap();

        assert_eq!(
            machine.take_outbox(),
            vec!["~".to_string(), "G0 X1\n".to_string(), "M30\n".to_string()]
        );
        assert_eq!(machine.program.next_line, 3);
        assert_eq!(machine.program.queued_lines, vec![0, 1]);

        machine.receive_controller_line("ok");
        machine.receive_controller_line("ok");

        assert!(!machine.program.running);
        assert_eq!(machine.status.mode, MachineMode::Running);
        assert!(machine.program.queued_lines.is_empty());
        machine.pause_program().unwrap();
        assert_eq!(machine.status.mode, MachineMode::Hold);
        assert!(machine.take_outbox().contains(&"!".to_string()));

        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        assert_eq!(machine.status.mode, MachineMode::Idle);
        assert!(machine.take_outbox().is_empty());
    }

    #[test]
    fn real_stream_never_exceeds_reported_serial_free_space_under_burst_load() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,32>");
        machine.status.serial_size = 32;
        let program = (0..40)
            .map(|index| format!("G1 X{index}"))
            .collect::<Vec<_>>()
            .join("\n");
        machine
            .load_gcode_text("stress.nc".to_string(), program)
            .unwrap();

        machine.run_program().unwrap();
        assert!(machine.program.total_bytes_in_flight <= machine.status.serial_size);

        let mut guard = 0;
        while machine.program.running {
            guard += 1;
            assert!(guard < 100, "stream did not finish");
            assert!(machine.program.total_bytes_in_flight <= machine.status.serial_size);
            assert_eq!(
                machine.status.serial_free + machine.program.total_bytes_in_flight,
                machine.status.serial_size
            );
            machine.receive_controller_line("ok");
        }

        assert!(machine.program.queued_lines.is_empty());
        assert_eq!(machine.program.total_bytes_in_flight, 0);
        assert_eq!(machine.status.serial_free, machine.status.serial_size);
    }

    #[test]
    fn status_reported_serial_free_space_is_capped_by_unacked_bytes() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,32>");
        machine.status.serial_size = 32;
        machine
            .load_gcode_text(
                "test.nc".to_string(),
                ["G0 X123456789", "G0 X987654321"].join("\n"),
            )
            .unwrap();

        machine.run_program().unwrap();
        assert!(machine.program.total_bytes_in_flight > 0);

        machine.receive_controller_line("<Run|MPos:0.000,0.000,0.000|Bf:15,32>");

        assert_eq!(
            machine.status.serial_free,
            machine
                .status
                .serial_size
                .saturating_sub(machine.program.total_bytes_in_flight)
        );
    }

    #[test]
    fn real_stream_rejects_line_that_cannot_fit_serial_buffer() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,16>");
        machine.status.serial_size = 16;
        machine
            .load_gcode_text(
                "too-long.nc".to_string(),
                "G1 X12345678901234567890\n".to_string(),
            )
            .unwrap();

        machine.run_program().unwrap();

        assert!(!machine.program.running);
        assert_eq!(machine.status.mode, MachineMode::Error);
        assert_eq!(
            machine.status.last_error.as_deref(),
            Some("line 1 is too long for the controller serial buffer")
        );
        assert_eq!(machine.program.total_bytes_in_flight, 0);
        assert_eq!(machine.take_outbox(), vec!["~".to_string()]);
    }

    #[test]
    fn simulator_stops_at_program_end() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nM2\nG0 X99\n".to_string())
            .unwrap();

        machine.run_program().unwrap();

        assert_eq!(machine.status.wpos.x, 1.0);
        assert_eq!(machine.program.next_line, 3);
        assert!(!machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "G0 X99"));
    }

    #[test]
    fn eeprom_commands_barrier_real_streaming() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text(
                "test.nc".to_string(),
                "G0 X1\nG10 L20 P1 X0\nG0 X2\n".to_string(),
            )
            .unwrap();

        machine.run_program().unwrap();
        let mut outbox = machine.take_outbox();
        assert_eq!(outbox, vec!["~".to_string(), "G0 X1\n".to_string()]);
        assert_eq!(machine.program.next_line, 1);
        assert_eq!(machine.program.queued_lines, vec![0]);

        machine.receive_controller_line("ok");
        outbox = machine.take_outbox();
        assert_eq!(outbox, vec!["G10 L20 P1 X0\n".to_string()]);
        assert_eq!(machine.program.next_line, 2);
        assert_eq!(machine.program.queued_lines, vec![1]);

        machine.receive_controller_line("ok");
        outbox = machine.take_outbox();
        assert_eq!(outbox, vec!["G0 X2\n".to_string()]);
        assert_eq!(machine.program.next_line, 3);
    }

    #[test]
    fn tool_change_holds_after_prior_real_stream_lines_are_acked() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nT2 M6\nG0 X2\n".to_string())
            .unwrap();

        machine.run_program().unwrap();
        let mut outbox = machine.take_outbox();
        assert_eq!(outbox, vec!["~".to_string(), "G0 X1\n".to_string()]);
        assert_eq!(machine.program.next_line, 1);
        assert!(machine.program.tool_change_line.is_none());

        machine.receive_controller_line("ok");
        outbox = machine.take_outbox();
        assert!(outbox.is_empty());
        assert_eq!(machine.program.next_line, 2);
        assert_eq!(machine.program.tool_change_line, Some(1));
        assert!(!machine.program.running);
        assert_eq!(machine.status.mode, MachineMode::Draining);

        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        assert_eq!(machine.status.mode, MachineMode::Hold);
        assert!(!machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "T2 M6"));

        machine.resume_program().unwrap();
        outbox = machine.take_outbox();
        assert_eq!(outbox, vec!["~".to_string(), "G0 X2\n".to_string()]);
        assert!(machine.program.tool_change_line.is_none());
    }

    #[test]
    fn simulator_holds_at_tool_change_without_sending_m6() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nM06\nG0 X2\n".to_string())
            .unwrap();

        machine.run_program().unwrap();

        assert_eq!(machine.status.wpos.x, 1.0);
        assert_eq!(machine.program.next_line, 2);
        assert_eq!(machine.program.tool_change_line, Some(1));
        assert_eq!(machine.status.mode, MachineMode::Hold);
        assert!(!machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "M06"));
    }

    #[test]
    fn detects_eeprom_commands_that_need_stream_barriers() {
        assert!(is_eeprom_command("G10 L20 P1 X0"));
        assert!(is_eeprom_command("g10l2p1x0"));
        assert!(is_eeprom_command("G28.1"));
        assert!(is_eeprom_command("G30.1"));
        assert!(is_eeprom_command("$100=250"));
        assert!(is_eeprom_command("$N0=G54"));
        assert!(is_eeprom_command("$RST=$"));
        assert!(!is_eeprom_command("G10 L1 P1 X0"));
        assert!(!is_eeprom_command("G28"));
        assert!(!is_eeprom_command("$G"));
    }

    #[test]
    fn detects_tool_change_commands() {
        assert!(is_tool_change_command("M6"));
        assert!(is_tool_change_command("m06"));
        assert!(is_tool_change_command("T2M6"));
        assert!(!is_tool_change_command("M60"));
        assert!(!is_tool_change_command("(M6)"));
    }

    #[test]
    fn streaming_strips_comments_and_skips_empty_lines() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text(
                "test.nc".to_string(),
                "(setup)\nG0 X1 ; move to one\n; comment\nG0 X2 (done)\n".to_string(),
            )
            .unwrap();
        machine.run_program().unwrap();
        let sent: Vec<&str> = machine
            .console
            .iter()
            .filter(|entry| entry.direction == ConsoleDirection::Tx)
            .map(|entry| entry.line.as_str())
            .collect();
        assert!(sent.contains(&"G0 X1"));
        assert!(sent.contains(&"G0 X2"));
        assert!(!sent.iter().any(|line| line.contains("COMMENT")));
        assert!(!sent.iter().any(|line| line.contains("SETUP")));
    }

    #[test]
    fn comment_lines_do_not_join_adjacent_streamed_lines() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text(
                "test.nc".to_string(),
                "G1 X1\n(after first move)\nG1 X2\n".to_string(),
            )
            .unwrap();

        machine.run_program().unwrap();

        assert_eq!(
            machine.take_outbox(),
            vec!["~".to_string(), "G1 X1\n".to_string(), "G1 X2\n".to_string()]
        );
    }

    #[test]
    fn real_stream_waits_for_controller_position_feedback() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\n".to_string())
            .unwrap();

        machine.run_program().unwrap();

        assert_eq!(machine.status.wpos.x, 0.0);
    }

    #[test]
    fn simulator_applies_compact_lowercase_words() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "g0x1.5y-2z3f120\n".to_string())
            .unwrap();
        machine.run_program().unwrap();
        assert_eq!(machine.status.wpos.x, 1.5);
        assert_eq!(machine.status.wpos.y, -2.0);
        assert_eq!(machine.status.wpos.z, 3.0);
        assert_eq!(machine.status.feed_rate, 120.0);
        assert!(machine.console.iter().any(
            |entry| entry.direction == ConsoleDirection::Tx && entry.line == "G0X1.5Y-2Z3F120"
        ));
    }

    #[test]
    fn optional_stop_toggle_changes_m1_behavior() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "M1\n".to_string())
            .unwrap();
        machine.single_step().unwrap();
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "M0"));

        machine
            .load_gcode_text("test.nc".to_string(), "M1\n".to_string())
            .unwrap();
        machine.toggle_optional_stop();
        machine.single_step().unwrap();
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "M1"));
    }

    #[test]
    fn single_step_skips_comments_and_blank_lines_without_waiting() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text(
                "test.nc".to_string(),
                "(setup)\n\n; another comment\nG0 X4\n".to_string(),
            )
            .unwrap();

        machine.single_step().unwrap();

        assert_eq!(machine.program.next_line, 4);
        assert_eq!(machine.status.wpos.x, 4.0);
        assert!(machine.console.iter().any(
            |entry| entry.direction == ConsoleDirection::Tx && entry.line == "G0 X4"
        ));
    }

    #[test]
    fn real_single_step_waits_for_previous_ack() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\n".to_string())
            .unwrap();

        machine.single_step().unwrap();
        assert!(machine.single_step().is_err());
        assert_eq!(machine.program.next_line, 1);

        machine.receive_controller_line("ok");
        machine.single_step().unwrap();
        assert_eq!(machine.program.next_line, 2);
    }

    #[test]
    fn pause_after_line_holds_after_requested_source_line() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\nG0 X2\nG0 X3\n".to_string())
            .unwrap();
        machine.set_pause_after_line(Some(1)).unwrap();
        machine.run_program().unwrap();
        assert_eq!(machine.program.next_line, 2);
        assert!(!machine.program.running);
        assert_eq!(machine.status.mode, MachineMode::Hold);
        assert_eq!(machine.program.pause_after_line, None);
        assert_eq!(machine.status.wpos.x, 2.0);
        assert!(!machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "G0 X3"));
    }

    #[test]
    fn pause_after_line_can_be_cleared() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine
            .load_gcode_text("test.nc".to_string(), "G0 X1\n".to_string())
            .unwrap();
        machine.set_pause_after_line(Some(0)).unwrap();
        machine.set_pause_after_line(None).unwrap();
        machine.run_program().unwrap();
        assert_eq!(machine.program.next_line, 1);
        assert_eq!(machine.status.mode, MachineMode::Idle);
    }

    #[test]
    fn console_records_tx_and_rx() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.send_mdi("M3 S1000".to_string()).unwrap();
        machine.receive_controller_line("ok");
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "M3 S1000"));
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Rx && entry.line == "ok"));
    }

    #[test]
    fn settings_dump_is_visible_in_console() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.send_mdi("$$".to_string()).unwrap();
        machine.receive_controller_line("$0=10");
        machine.receive_controller_line("$1=25");
        machine.receive_controller_line("ok");

        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Tx && entry.line == "$$"));
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Rx
                && entry.line == "$0=10 # step pulse, usec"));
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Rx
                && entry.line == "$1=25 # step idle delay, msec"));
    }

    #[test]
    fn settings_dump_axis_lines_are_annotated_in_console() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.receive_controller_line("$100=250.000");
        machine.receive_controller_line("$103=80.000");
        machine.receive_controller_line("$110=5000.000");
        machine.receive_controller_line("$999=1");

        assert!(machine.console.iter().any(|entry| {
            entry.direction == ConsoleDirection::Rx && entry.line == "$100=250.000 # X steps/mm"
        }));
        assert!(machine.console.iter().any(|entry| {
            entry.direction == ConsoleDirection::Rx && entry.line == "$103=80.000 # A steps/mm"
        }));
        assert!(machine.console.iter().any(|entry| {
            entry.direction == ConsoleDirection::Rx
                && entry.line == "$110=5000.000 # X max rate, mm/min"
        }));
        assert!(machine
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Rx && entry.line == "$999=1"));
    }

    #[test]
    fn hidden_status_updates_do_not_crowd_useful_console_lines_out_of_snapshot() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.settings.hide_state_updates = true;
        machine.receive_controller_line("$0=10");
        for _ in 0..300 {
            machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000>");
        }

        let snapshot = machine.snapshot();

        assert!(snapshot
            .console
            .iter()
            .any(|entry| entry.direction == ConsoleDirection::Rx
                && entry.line == "$0=10 # step pulse, usec"));
        assert!(!snapshot.console.iter().any(|entry| entry.line.starts_with('<')));
    }

    #[test]
    fn overrides_emit_realtime_commands() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.adjust_feed_override(10).unwrap();
        machine.adjust_feed_override(1).unwrap();
        machine.adjust_feed_override(0).unwrap();
        assert_eq!(machine.status.feed_override, 100.0);
        assert_eq!(
            machine.outbox,
            vec![
                "\u{91}".to_string(),
                "\u{93}".to_string(),
                "\u{90}".to_string()
            ]
        );

        machine.set_rapid_override(25).unwrap();
        assert_eq!(machine.status.rapid_override, 25.0);
        assert!(machine.outbox.contains(&"\u{97}".to_string()));
    }

    #[test]
    fn real_overrides_display_controller_reported_values() {
        let mut machine = Machine::new();
        machine.status.port_name = "/dev/ttyUSB0".to_string();
        machine.status.ready = true;
        machine.status.mode = MachineMode::Idle;

        machine.adjust_feed_override(10).unwrap();
        machine.adjust_spindle_override(-10).unwrap();
        machine.set_rapid_override(25).unwrap();

        assert_eq!(machine.status.feed_override, 100.0);
        assert_eq!(machine.status.spindle_override, 100.0);
        assert_eq!(machine.status.rapid_override, 100.0);

        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Ov:200,25,10>");
        assert_eq!(machine.status.feed_override, 200.0);
        assert_eq!(machine.status.rapid_override, 25.0);
        assert_eq!(machine.status.spindle_override, 10.0);
    }

    #[test]
    fn work_position_is_allowed_while_idle_or_held() {
        let mut machine = Machine::new();
        machine.connect_sim();
        machine.status.mode = MachineMode::Running;
        assert!(machine.set_work_position(Vec4::default()).is_err());

        machine.status.mode = MachineMode::Hold;
        machine
            .set_work_position(Vec4 {
                x: 1.0,
                y: 2.0,
                z: 3.0,
                a: 0.0,
            })
            .unwrap();
        assert_eq!(machine.status.wpos.x, 1.0);
    }

    #[test]
    fn classic_grbl_work_position_omits_unsupported_a_axis() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:0.000,0.000,0.000|Bf:15,128>");

        machine
            .set_work_position(Vec4 {
                x: 1.0,
                y: 2.0,
                z: 3.0,
                a: 4.0,
            })
            .unwrap();

        assert_eq!(
            machine.take_outbox(),
            vec!["G10L20P1X1.000Y2.000Z3.000\n".to_string()]
        );
    }

    #[test]
    fn real_work_position_waits_for_controller_position_feedback() {
        let mut machine = Machine::new();
        machine.connect_port("/dev/ttyUSB0".to_string());
        machine.receive_controller_line("<Idle|MPos:10.000,20.000,30.000|Bf:15,128>");

        machine
            .set_work_position(Vec4 {
                x: 1.0,
                y: 2.0,
                z: 3.0,
                a: 0.0,
            })
            .unwrap();

        assert_eq!(machine.status.wpos.x, 10.0);
        assert_eq!(machine.status.wpos.y, 20.0);
        assert_eq!(machine.status.wpos.z, 30.0);
    }

    #[test]
    fn a_axis_work_position_is_sent_when_controller_supports_a() {
        let mut machine = Machine::new();
        machine.connect_sim();

        machine
            .set_work_position(Vec4 {
                x: 1.0,
                y: 2.0,
                z: 3.0,
                a: 4.0,
            })
            .unwrap();

        assert_eq!(
            machine.take_outbox(),
            vec!["G10L20P1X1.000Y2.000Z3.000A4.000\n".to_string()]
        );
    }

    #[test]
    fn settings_load_with_new_defaults_and_are_clamped() {
        let settings: AppSettings = serde_json::from_str(r#"{"jog_increment":0.0,"jog_feed":0.0}"#)
            .expect("settings deserialize");
        assert_eq!(settings.toolpath_zoom, 1.0);
        assert_eq!(settings.left_panel_width, 260.0);
        assert_eq!(settings.sidebar_width, 260.0);
        assert_eq!(settings.show_console, false);
        assert_eq!(settings.hide_state_updates, true);
        assert_eq!(settings.keyboard_shortcuts_enabled, true);
        assert_eq!(settings.keyboard_jog_requires_shift, false);

        let mut machine = Machine::new();
        machine.update_settings(AppSettings {
            jog_increment: 0.0,
            jog_feed: -10.0,
            toolpath_zoom: 99.0,
            left_panel_width: 10.0,
            sidebar_width: 9999.0,
            ..AppSettings::default()
        });
        assert_eq!(machine.settings.jog_increment, 0.001);
        assert_eq!(machine.settings.jog_feed, 1.0);
        assert_eq!(machine.settings.toolpath_zoom, 8.0);
        assert_eq!(machine.settings.left_panel_width, 220.0);
        assert_eq!(machine.settings.sidebar_width, 2000.0);
    }
}
