# TODO

## Required

- Verify real serial transport against physical Grbl/grblHAL hardware. Partial: Arduino Nano on `/dev/ttyUSB0` was flashed with Grbl 1.1h and passed the Pugsender hardware smoke stream; grblHAL and live CNC motion are still untested.
- Add full grblHAL capability detection beyond axis-count inference. Done for `$I+` reports: firmware, version/build, options, axes, driver, board, signals, storage, aux IO, and plugins are parsed and shown.
- Expand G-code parser/path planner for less-common modal state and more canned cycles. Partial: arcs, unit/distance modes, plane modes, `G4`, `G73`, `G81`-`G89`, `G92`, `G92.1`, `G92.2`, `G98`, `G99`, and `M2`/`M30` program-end handling are previewed.
- Refine keyboard shortcut defaults after machine-side testing. Partial: shortcut repeats are suppressed, global shortcuts can be disabled, and keyboard jogging can require Shift; final defaults still need machine-side testing.
- Add end-to-end Tauri command tests once desktop dependencies are installed in CI/dev. Initial coverage added for Tauri IPC command wiring through simulator load/run/snapshot, plus PTY-backed mock serial streaming and capability detection.
- Validate stream-progress UI against a real controller. Partial: non-hardware coverage tracks current, queued, completed, next, and pause-after source lines, and the backend stream state passed against a physical Grbl 1.1h controller; full UI validation during live machine motion remains untested.
- Protect EEPROM-writing commands in the stream. Done: `$` settings writes, `$N`, `$RST=`, `G10 L2/L20`, `G28.1`, and `G30.1` act as stream barriers until acknowledged.
- Handle program tool changes. Done: `M6`/`M06` drains prior queued motion, skips sending unsupported Grbl tool-change commands, marks the tool-change line, and resumes after the operator continues.
- Improve serial connection failure UX. Done: failed port opens clear pending probes, return to disconnected state, and report the open error in machine status.
- Find Grbl automatically. Done: auto-connect probes remembered, USB, and other serial ports for Grbl/grblHAL responses before opening the matching controller.
- Harden command availability in the UI. Done: run/hold/resume/stop/drain/single-step, MDI, jogging, work offsets, overrides, and connect controls are disabled unless the current machine state can accept them.
- Harden backend command state validation. Done: IPC commands reject invalid run/hold/resume/drain/single-step state transitions, while stop/reset remains available for active, alarm, and error recovery.
- Stress serial transmit accounting. Done: streaming now fits each normalized line exactly against reported serial free space, rejects lines too large for the controller buffer, and has burst-load coverage with a deliberately tiny RX buffer.
- Prevent unsafe program replacement. Done: loading a new G-code file is rejected while a program is running, held, draining, jogging, paused at tool change, or has unacknowledged in-flight lines; the UI disables file loading in the same states.
- Flush pending serial writes before soft reset. Done: the serial transport now owns a clearable pending-command queue, and any Ctrl-X outbox clears queued G-code before the reset is sent.
- Verify release packaging. Done: Tauri bundle icons are generated/configured, and `npm run tauri build` produces `.deb`, `.rpm`, and `.AppImage` packages.
- Prevent unsafe run-from-line changes. Done: run-from-line is rejected while a program is active, held, draining, jogging, paused at tool change, or has in-flight lines; the UI disables the same controls in those states.
- Fix Tauri dev startup. Done: Vite runs on the strict `1420` port configured as Tauri's `devUrl`, and `npm run tauri dev` reaches the launched app.
- Improve controller status visibility. Done: the UI shows raw Grbl pin-state letters, planner free slots, serial free space, and active modal codes in the status panel.
- Clear stale fault banners after recovery. Done: stop/reset clears latched alarm/error fields, and normal controller status reports clear stale fault messages.
- Fix large-file UI lag. Done: program rows are virtualized, toolpath canvas drawing is batched, successful commands no longer force a duplicate full snapshot fetch, and serial status events send lightweight live snapshots instead of re-sending all G-code lines/toolpath points. `/nas/cad/celesta/case-parts.nc` profiles at about 23.6k lines, 24.7k toolpath points, and under 100 ms backend read/split/toolpath time.

## Nice-to-have

- File browser with tab completion and previews. Done: keyboard path opener has directory/file completion and bounded source previews, native open dialog remains available, last G-code path is remembered, and loading immediately updates the program/toolpath preview.
- Cycle time estimate. Done: preview estimates are calculated from modal feed/rapid movement and shown in the status panel.
- Run-to-line and pause-after-line. Done: run-to-line and pause-after-line are available in the program header.
- Program search and line actions. Done: the program view has text search with previous/next navigation, match highlighting, and per-line controls for run-from and pause-after.
- Toolpath inspection. Done: preview shows bounding-box dimensions and colors completed, active, queued, and pending movement from source-line metadata.
- Persistent UI layout and toolpath zoom. Done: console visibility, toolpath zoom, and right-side panel width are persisted.
- Richer continuous keyboard jogging. Done: held arrow/PageUp/PageDown keys and held jog buttons repeat jog commands.
