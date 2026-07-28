export type Axis = "X" | "Y" | "Z" | "A";
export type MachineMode =
  | "Disconnected"
  | "Connecting"
  | "Idle"
  | "Jogging"
  | "Running"
  | "Hold"
  | "Draining"
  | "Alarm"
  | "Error";

export interface Vec4 {
  x: number;
  y: number;
  z: number;
  a: number;
}

export interface ControllerStatus {
  port_name: string;
  ready: boolean;
  mode: MachineMode;
  axis_count: number;
  capabilities: ControllerCapabilities;
  wpos: Vec4;
  mpos: Vec4;
  wco: Vec4;
  feed_rate: number;
  spindle_speed: number;
  feed_override: number;
  rapid_override: number;
  spindle_override: number;
  planner_free: number;
  serial_free: number;
  serial_size: number;
  probe: boolean;
  pins: string;
  active_codes: string;
  last_error: string | null;
  last_alarm: string | null;
}

export interface ControllerCapabilities {
  firmware: string | null;
  version: string | null;
  build: string | null;
  opt_flags: string[];
  newopt_flags: string[];
  axis_letters: string;
  driver: string | null;
  driver_version: string | null;
  board: string | null;
  signals: string | null;
  nvs_storage: string | null;
  aux_io: string | null;
  plugins: string[];
}

export interface ProgramState {
  filename: string | null;
  lines: string[];
  next_line: number;
  current_line: number | null;
  queued_lines: number[];
  running: boolean;
  optional_stop: boolean;
  pause_after_line: number | null;
  tool_change_line: number | null;
  run_started_line: number;
  total_bytes_in_flight: number;
  running_elapsed_seconds: number;
  estimated_seconds?: number;
  remaining_seconds?: number;
  toolpath: {
    points: Vec4[];
    point_source_lines: Array<number | null>;
    min: Vec4;
    max: Vec4;
    estimated_seconds: number;
    line_estimated_seconds: number[];
    remaining_seconds_by_line: number[];
  };
}

export interface UiMessage {
  level: "info" | "warn" | "error";
  text: string;
  at_ms: number;
}

export interface ConsoleEntry {
  direction: "tx" | "rx";
  line: string;
  at_ms: number;
}

export interface AppSettings {
  jog_increment: number;
  jog_feed: number;
  rapid_jog: boolean;
  toolpath_zoom: number;
  left_panel_width: number;
  sidebar_width: number;
  show_console: boolean;
  hide_state_updates: boolean;
  keyboard_shortcuts_enabled: boolean;
  keyboard_jog_requires_shift: boolean;
  mdi_history: string[];
  last_port: string | null;
  last_gcode_path: string | null;
  saved_wpos: Vec4;
}

export interface Snapshot {
  status: ControllerStatus;
  program: ProgramState;
  messages: UiMessage[];
  console: ConsoleEntry[];
  settings: AppSettings;
}
