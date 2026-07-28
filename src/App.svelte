<script lang="ts">
  import { afterUpdate, onMount, tick } from "svelte";
  import { invoke } from "@tauri-apps/api/core";
  import { listen } from "@tauri-apps/api/event";
  import { open } from "@tauri-apps/plugin-dialog";
  import type { AppSettings, Axis, ConsoleEntry, ControllerStatus, Snapshot, UiMessage } from "./app";

  interface FileCandidate {
    path: string;
    label: string;
    is_dir: boolean;
  }

  interface GcodeFilePreview {
    path: string;
    filename: string;
    size_bytes: number;
    line_count: number;
    preview_lines: string[];
  }

  interface ProgramProgress {
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
    estimated_seconds: number;
    remaining_seconds: number;
  }

  interface LiveSnapshot {
    status: ControllerStatus;
    program: ProgramProgress;
    messages: UiMessage[];
    console: ConsoleEntry[];
    settings: AppSettings;
  }

  interface ToolpathMenu {
    screenX: number;
    screenY: number;
    workX: number;
    workY: number;
  }

  interface ToolpathHover {
    workX: number;
    workY: number;
  }

  let snapshot: Snapshot | null = null;
  let mdi = "";
  let jogIncrement = 1;
  let jogFeed = 300;
  let rapidJog = false;
  let toolpathZoom = 1;
  let toolpathPanX = 0;
  let toolpathPanY = 0;
  let leftPanelWidth = 260;
  let sidebarWidth = 260;
  let keyboardShortcutsEnabled = true;
  let keyboardJogRequiresShift = false;
  let ports: string[] = [];
  let selectedPort = "";
  let initializedSettings = false;
  let mdiHistoryIndex: number | null = null;
  let commandError = "";
  let droEditAxis: Axis | null = null;
  let droExpression = "";
  let droError = "";
  let toolpathMenu: ToolpathMenu | null = null;
  let toolpathHover: ToolpathHover | null = null;
  let showFileOpen = false;
  let filePath = "";
  let fileCandidates: FileCandidate[] = [];
  let selectedFileCandidate = 0;
  let filePreview: GcodeFilePreview | null = null;
  let filePreviewError = "";
  let filePreviewRequest = 0;
  let showShortcuts = false;
  let hideStateUpdates = true;
  let autoScrollGcode = true;
  let runLine = 1;
  let pauseAfterLine = 1;
  let programSearch = "";
  let selectedSearchMatch = 0;
  let hoveredProgramLine: number | null = null;
  let remainingDisplay = 0;
  let toolpathRangeX = "-";
  let toolpathRangeY = "-";
  let toolpathRangeZ = "-";
  let toolpathRangeA = "-";
  let aAxisAvailable = false;
  let mdiInput: HTMLInputElement;
  let droInput: HTMLInputElement;
  let filePathInput: HTMLInputElement;
  let programSearchInput: HTMLInputElement;
  let workArea: HTMLElement;
  let baseCanvas: HTMLCanvasElement;
  let canvas: HTMLCanvasElement;
  let consoleLines: HTMLDivElement;
  let programListViewport: HTMLDivElement;
  let programScrollTop = 0;
  let programViewportHeight = 480;
  const programLineHeight = 24;
  const programOverscan = 40;
  let searchMatchSet = new Set<number>();
  let lastBaseDrawKey = "";
  let lastOverlayDrawKey = "";
  let lastConsoleKey = "";
  let depthLayerKey = "";
  let depthLayerCanvas: HTMLCanvasElement | null = null;
  let lastScrollTarget: number | null = null;
  let loadedProgramKey = "";
  let jogTimer: number | null = null;
  let activeJogKey: string | null = null;
  let toolpathDrag: { pointerId: number; x: number; y: number } | null = null;
  const minSidebarWidth = 260;
  const maxSidebarWidth = 2000;
  const minLeftPanelWidth = 220;
  const maxLeftPanelWidth = 700;

  function remainingFromLineTimes(lineTimes: number[]) {
    const remaining = new Array(lineTimes.length + 1).fill(0);
    let total = 0;
    for (let index = lineTimes.length - 1; index >= 0; index -= 1) {
      total += Number(lineTimes[index]) || 0;
      remaining[index] = total;
    }
    return remaining;
  }

  function normalizeProgramTiming(program: Snapshot["program"]) {
    const toolpath = program.toolpath;
    if (toolpath.remaining_seconds_by_line.length > 0 || toolpath.line_estimated_seconds.length === 0) {
      return program;
    }
    return {
      ...program,
      toolpath: {
        ...toolpath,
        remaining_seconds_by_line: remainingFromLineTimes(toolpath.line_estimated_seconds)
      }
    };
  }

  function applySnapshot(next: Snapshot) {
    const nextProgramKey = programIdentity(next);
    const programChanged = nextProgramKey !== loadedProgramKey;
    snapshot = {
      ...next,
      program: normalizeProgramTiming(next.program)
    };
    loadedProgramKey = nextProgramKey;
    if (programChanged && snapshot.program.lines.length > 0) {
      fitToolpathToView();
    }
    applySettings(snapshot.settings);
  }

  function applyLiveSnapshot(next: LiveSnapshot) {
    if (!snapshot) {
      void refresh();
      return;
    }
    snapshot = {
      ...snapshot,
      status: next.status,
      program: normalizeProgramTiming({
        ...snapshot.program,
        ...next.program
      }),
      messages: next.messages,
      console: next.console,
      settings: next.settings
    };
    applySettings(next.settings);
  }

  function programIdentity(next: Snapshot) {
    const lines = next.program.lines;
    return [
      next.program.filename ?? "",
      lines.length,
      lines[0] ?? "",
      lines[lines.length - 1] ?? "",
      next.program.toolpath.points.length
    ].join("\u001f");
  }

  function applySettings(settings: AppSettings) {
    if (!initializedSettings) {
      jogIncrement = settings.jog_increment;
      jogFeed = settings.jog_feed;
      rapidJog = settings.rapid_jog;
      leftPanelWidth = settings.left_panel_width;
      sidebarWidth = settings.sidebar_width;
      hideStateUpdates = settings.hide_state_updates;
      keyboardShortcutsEnabled = settings.keyboard_shortcuts_enabled;
      keyboardJogRequiresShift = settings.keyboard_jog_requires_shift;
      selectedPort = settings.last_port ?? selectedPort;
      initializedSettings = true;
    }
  }

  async function refresh() {
    applySnapshot(await invoke<Snapshot>("get_snapshot"));
  }

  async function command(name: string, args: Record<string, unknown> = {}, refreshAfter = true) {
    try {
      commandError = "";
      await invoke(name, args);
      if (refreshAfter) {
        await refresh();
      }
    } catch (error) {
      commandError = String(error);
      await refresh();
    }
  }

  async function refreshPorts() {
    ports = await invoke<string[]>("list_ports");
    if (!selectedPort && ports.length > 0) {
      selectedPort = ports[0];
    }
  }

  async function connectSelectedPort() {
    if (!selectedPort) return;
    await command("connect_port", { portName: selectedPort });
  }

  async function connectAuto() {
    await refreshPorts();
    await command("auto_connect");
    await refreshPorts();
  }

  async function saveUiSettings() {
    if (!snapshot) return;
    await invoke("save_settings", {
      settings: {
        ...snapshot.settings,
        jog_increment: Number(jogIncrement),
        jog_feed: Number(jogFeed),
        rapid_jog: rapidJog,
        toolpath_zoom: Number(toolpathZoom),
        left_panel_width: Number(leftPanelWidth),
        sidebar_width: Number(sidebarWidth),
        show_console: true,
        hide_state_updates: hideStateUpdates,
        keyboard_shortcuts_enabled: keyboardShortcutsEnabled,
        keyboard_jog_requires_shift: keyboardJogRequiresShift,
        saved_wpos: snapshot.status.wpos
      }
    });
    await refresh();
  }

  async function setHideStateUpdates(hidden: boolean) {
    hideStateUpdates = hidden;
    await saveUiSettings();
  }

  async function adjustToolpathZoom(factor: number) {
    toolpathZoom = Math.min(8, Math.max(0.25, toolpathZoom * factor));
    await saveUiSettings();
    invalidateToolpath();
  }

  async function resetToolpathZoom() {
    fitToolpathToView();
    await saveUiSettings();
  }

  function fitToolpathToView() {
    toolpathZoom = 1;
    toolpathPanX = 0;
    toolpathPanY = 0;
    depthLayerKey = "";
    invalidateToolpath();
  }

  async function disconnect() {
    await command("disconnect");
  }

  async function drainProgram() {
    if (!canDrain) return;
    await command("drain_program");
  }

  async function runOrResumeProgram() {
    await command("run_or_resume_program");
  }

  async function setRunLine() {
    if (!canChangeStartLine) return;
    await command("run_from_line", { line: Math.max(0, Number(runLine) - 1) });
  }

  async function setPauseAfterLine() {
    await command("set_pause_after_line", { line: Math.max(0, Number(pauseAfterLine) - 1) });
  }

  async function setRunLineTo(index: number) {
    if (!canChangeStartLine) return;
    runLine = index + 1;
    await command("run_from_line", { line: index });
  }

  async function setPauseAfterLineTo(index: number) {
    pauseAfterLine = index + 1;
    await command("set_pause_after_line", { line: index });
  }

  async function clearPauseAfterLine() {
    await command("set_pause_after_line", { line: null });
  }

  async function submitMdi() {
    if (!canMdi) return;
    const line = mdi.trim();
    if (!line) return;
    mdi = "";
    mdiHistoryIndex = null;
    await command("send_mdi", { line });
  }

  function mdiKeydown(event: KeyboardEvent) {
    const history = snapshot?.settings.mdi_history ?? [];
    if (event.key === "Escape") {
      event.preventDefault();
      mdiHistoryIndex = null;
      mdiInput?.blur();
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      if (history.length === 0) return;
      mdiHistoryIndex = mdiHistoryIndex === null ? history.length - 1 : Math.max(0, mdiHistoryIndex - 1);
      mdi = history[mdiHistoryIndex] ?? "";
    } else if (event.key === "ArrowDown") {
      event.preventDefault();
      if (mdiHistoryIndex === null) return;
      mdiHistoryIndex += 1;
      if (mdiHistoryIndex >= history.length) {
        mdiHistoryIndex = null;
        mdi = "";
      } else {
        mdi = history[mdiHistoryIndex] ?? "";
      }
    }
  }

  async function startMdiWith(letter: string) {
    if (!canMdi) return;
    mdi = letter;
    mdiHistoryIndex = null;
    await tick();
    mdiInput?.focus();
    mdiInput?.setSelectionRange(mdi.length, mdi.length);
  }

  function toggleShortcuts() {
    toolpathMenu = null;
    showShortcuts = !showShortcuts;
  }

  function releaseKeyboardFocus(element: HTMLElement | null | undefined = document.activeElement as HTMLElement | null) {
    element?.blur();
  }

  function closeFileOpen() {
    releaseKeyboardFocus(filePathInput);
    showFileOpen = false;
  }

  async function openGcodeFile() {
    if (!canLoadGcode) return;
    closeFileOpen();
    try {
      const selected = await open({
        title: "Open G-code",
        defaultPath: snapshot?.settings.last_gcode_path ?? undefined,
        multiple: false,
        filters: [
          { name: "G-code", extensions: ["nc", "gcode", "tap", "cnc", "ngc", "txt"] }
        ]
      });
      if (typeof selected === "string") {
        await command("load_gcode_path", { path: selected });
        releaseKeyboardFocus();
      }
    } catch (error) {
      commandError = String(error);
    }
  }

  async function showPathOpen() {
    if (!canLoadGcode) return;
    filePath = snapshot?.settings.last_gcode_path ?? filePath;
    showFileOpen = true;
    await refreshFileCandidates();
    await tick();
    filePathInput?.focus();
    filePathInput?.select();
  }

  async function refreshFileCandidates() {
    fileCandidates = await invoke<FileCandidate[]>("complete_gcode_path", { partial: filePath });
    selectedFileCandidate = Math.min(selectedFileCandidate, Math.max(0, fileCandidates.length - 1));
    await previewCurrentPath();
  }

  async function filePathInputChanged() {
    selectedFileCandidate = 0;
    await refreshFileCandidates();
  }

  async function chooseFileCandidate(candidate: FileCandidate) {
    filePath = candidate.path;
    if (candidate.is_dir) {
      await refreshFileCandidates();
      await tick();
      filePathInput?.focus();
      return;
    }
    await loadPathFromInput();
  }

  async function selectFileCandidate(index: number) {
    selectedFileCandidate = Math.min(Math.max(0, index), Math.max(0, fileCandidates.length - 1));
    const candidate = fileCandidates[selectedFileCandidate];
    if (candidate) {
      filePath = candidate.path;
    }
    await previewCurrentPath();
  }

  async function previewCurrentPath() {
    const candidate = fileCandidates[selectedFileCandidate];
    const path = (candidate && !candidate.is_dir ? candidate.path : filePath).trim();
    const request = ++filePreviewRequest;
    filePreview = null;
    filePreviewError = "";
    if (!path) return;
    try {
      const preview = await invoke<GcodeFilePreview>("preview_gcode_path", { path });
      if (request === filePreviewRequest) {
        filePreview = preview;
      }
    } catch (error) {
      if (request === filePreviewRequest) {
        filePreviewError = String(error);
      }
    }
  }

  async function loadPathFromInput() {
    if (!canLoadGcode) return;
    const path = filePath.trim();
    if (!path) return;
    closeFileOpen();
    await command("load_gcode_path", { path });
  }

  async function filePathKeydown(event: KeyboardEvent) {
    if (event.key === "Escape") {
      event.preventDefault();
      closeFileOpen();
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      await selectFileCandidate(selectedFileCandidate + 1);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      await selectFileCandidate(selectedFileCandidate - 1);
      return;
    }
    if (event.key === "Tab") {
      event.preventDefault();
      const candidate = fileCandidates[selectedFileCandidate] ?? fileCandidates[0];
      if (candidate) {
        filePath = candidate.path;
        await refreshFileCandidates();
      }
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      const candidate = fileCandidates[selectedFileCandidate];
      if (candidate) {
        await chooseFileCandidate(candidate);
      } else {
        await loadPathFromInput();
      }
    }
  }

  async function jogToToolpathMenu() {
    if (!toolpathMenu) return;
    await command("jog_to_xy", {
      x: toolpathMenu.workX,
      y: toolpathMenu.workY,
      feed: Number(jogFeed)
    });
    toolpathMenu = null;
  }

  async function setWorkXyToToolpathMenu() {
    if (!toolpathMenu || !snapshot) return;
    await command("set_work_position", {
      values: {
        ...snapshot.status.wpos,
        x: toolpathMenu.workX,
        y: toolpathMenu.workY
      }
    });
    toolpathMenu = null;
  }

  async function startDroEdit(axis: Axis, expression = "") {
    if (!canSetWorkPosition) return;
    droEditAxis = axis;
    droExpression = expression;
    droError = "";
    await tick();
    droInput?.focus();
    droInput?.setSelectionRange(droExpression.length, droExpression.length);
  }

  function closeDroEdit() {
    releaseKeyboardFocus(droInput);
    droEditAxis = null;
    droExpression = "";
    droError = "";
  }

  async function submitDroEdit() {
    if (!snapshot || !droEditAxis) return;
    const result = evaluateDroExpression(droExpression, droEditAxis);
    if (result === null || !Number.isFinite(result)) {
      droError = "invalid expression";
      return;
    }
    const axisKey = droEditAxis.toLowerCase() as "x" | "y" | "z" | "a";
    await command("set_work_position", {
      values: {
        ...snapshot.status.wpos,
        [axisKey]: result
      }
    });
    closeDroEdit();
  }

  function evaluateDroExpression(expression: string, axis: Axis) {
    if (!snapshot) return null;
    const trimmed = expression.trim();
    if (!trimmed) return null;
    const context = {
      X: snapshot.status.wpos.x,
      Y: snapshot.status.wpos.y,
      Z: snapshot.status.wpos.z,
      A: snapshot.status.wpos.a
    };
    return parseAxisWordAssignment(trimmed, context)
      ?? parseNumericExpression(trimmed, context)
      ?? parseNumericExpression(`${axis}${trimmed}`, context);
  }

  function parseAxisWordAssignment(expression: string, context: Record<Axis, number>) {
    const axis = expression[0]?.toUpperCase() as Axis;
    if (axis !== "X" && axis !== "Y" && axis !== "Z" && axis !== "A") return null;
    const rest = expression.slice(1).trimStart();
    if (!/^(?:-?(?:\d|\.)|$)/.test(rest)) return null;
    if (!rest) return context[axis];
    return parseNumericExpression(rest, context);
  }

  function parseNumericExpression(expression: string, context: Record<Axis, number>) {
    let index = 0;

    function skipSpace() {
      while (/\s/.test(expression[index] ?? "")) index += 1;
    }

    function parseExpression(): number | null {
      let value = parseTerm();
      if (value === null) return null;
      while (true) {
        skipSpace();
        const operator = expression[index];
        if (operator !== "+" && operator !== "-") break;
        index += 1;
        const right = parseTerm();
        if (right === null) return null;
        value = operator === "+" ? value + right : value - right;
      }
      return value;
    }

    function parseTerm(): number | null {
      let value = parseFactor();
      if (value === null) return null;
      while (true) {
        skipSpace();
        const operator = expression[index];
        if (operator !== "*" && operator !== "/") break;
        index += 1;
        const right = parseFactor();
        if (right === null) return null;
        if (operator === "/" && right === 0) return null;
        value = operator === "*" ? value * right : value / right;
      }
      return value;
    }

    function parseFactor(): number | null {
      skipSpace();
      const operator = expression[index];
      if (operator === "+" || operator === "-") {
        index += 1;
        const value = parseFactor();
        if (value === null) return null;
        return operator === "-" ? -value : value;
      }
      if (operator === "(") {
        index += 1;
        const value = parseExpression();
        skipSpace();
        if (expression[index] !== ")") return null;
        index += 1;
        return value;
      }
      const axis = operator?.toUpperCase() as Axis;
      if (axis === "X" || axis === "Y" || axis === "Z" || axis === "A") {
        index += 1;
        return context[axis];
      }
      const match = expression.slice(index).match(/^(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?/i);
      if (!match) return null;
      index += match[0].length;
      return Number(match[0]);
    }

    const value = parseExpression();
    skipSpace();
    if (value === null || index !== expression.length) return null;
    return value;
  }

  function axisAvailable(axis: Axis) {
    if (axis !== "A") return true;
    return (snapshot?.status.capabilities.axis_letters.includes("A") ?? false)
      || (snapshot?.status.axis_count ?? 3) >= 4;
  }

  function startJog(axis: Axis, direction: number, key: string) {
    if (!canJog) return;
    if (activeJogKey === key) return;
    stopJog();
    activeJogKey = key;
    void command("start_jog", {
      axis,
      direction,
      step: Number(jogIncrement),
      feed: rapidJog ? Number(jogFeed) * 4 : Number(jogFeed)
    }, false);
    const periodMs = 100;
    jogTimer = window.setInterval(() => {
      void command("extend_jog", { axis, direction, periodMs }, false);
    }, periodMs);
  }

  function stopJog(key: string | null = null) {
    if (key !== null && activeJogKey !== key) return;
    if (jogTimer !== null) {
      window.clearInterval(jogTimer);
      jogTimer = null;
    }
    if (activeJogKey !== null) void command("stop_jog", {}, false);
    activeJogKey = null;
  }

  function jogForKey(key: string): { axis: Axis; direction: number } | null {
    if (key === "arrowleft") return { axis: "X", direction: -1 };
    if (key === "arrowright") return { axis: "X", direction: 1 };
    if (key === "arrowdown") return { axis: "Y", direction: -1 };
    if (key === "arrowup") return { axis: "Y", direction: 1 };
    if (key === "pagedown") return { axis: "Z", direction: -1 };
    if (key === "pageup") return { axis: "Z", direction: 1 };
    return null;
  }

  function isTextEntryTarget(target: EventTarget | null) {
    if (!(target instanceof HTMLElement)) return false;
    if (!target.isConnected || target.getClientRects().length === 0) return false;
    return target.isContentEditable || ["INPUT", "SELECT", "TEXTAREA"].includes(target.tagName);
  }

  function unmodifiedKey(event: KeyboardEvent) {
    return !event.ctrlKey && !event.altKey && !event.metaKey;
  }

  function handleMachineShortcut(event: KeyboardEvent, key: string) {
    if (event.repeat || !keyboardShortcutsEnabled || !unmodifiedKey(event)) return false;
    if (isTextEntryTarget(event.target)) return false;
    if (key === "r" && connected) {
      event.preventDefault();
      event.stopPropagation();
      void command("run_or_resume_program");
      return true;
    }
    if (key === "h" && connected) {
      event.preventDefault();
      event.stopPropagation();
      void command("pause_program");
      return true;
    }
    return false;
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === "Escape" && droEditAxis) {
      closeDroEdit();
      return;
    }
    if (event.key === "Escape" && toolpathMenu) {
      toolpathMenu = null;
      return;
    }
    const key = event.key.toLowerCase();
    if (handleMachineShortcut(event, key)) return;
    if (isTextEntryTarget(event.target) || !keyboardShortcutsEnabled) return;
    if (!event.repeat) {
      if ((key === "g" || key === "m") && unmodifiedKey(event)) {
        event.preventDefault();
        void startMdiWith(event.key.toUpperCase());
        return;
      }
      const axisKey = key.toUpperCase() as Axis;
      if ((axisKey === "X" || axisKey === "Y" || axisKey === "Z" || axisKey === "A")
        && canSetWorkPosition
        && unmodifiedKey(event)) {
        event.preventDefault();
        void startDroEdit(axisKey, axisKey);
        return;
      }
      if (key === "o") showPathOpen();
      if (key === "/") {
        event.preventDefault();
        programSearchInput?.focus();
      }
      if (key === "f3") {
        event.preventDefault();
        selectSearchMatch(event.shiftKey ? selectedSearchMatch - 1 : selectedSearchMatch + 1);
      }
      if (key === "?") toggleShortcuts();
    }
    const jogMove = jogForKey(key);
    if (jogMove) {
      if (!canJog) return;
      if (keyboardJogRequiresShift && !event.shiftKey) return;
      event.preventDefault();
      startJog(jogMove.axis, jogMove.direction, key);
    }
  }

  function keyup(event: KeyboardEvent) {
    stopJog(event.key.toLowerCase());
  }

  $: searchMatches = matchingProgramLines(snapshot?.program.lines ?? [], programSearch);
  $: visibleLineStart = Math.max(0, Math.floor(programScrollTop / programLineHeight) - programOverscan);
  $: visibleLineCount = Math.ceil(programViewportHeight / programLineHeight) + programOverscan * 2;
  $: visibleLineEnd = Math.min(snapshot?.program.lines.length ?? 0, visibleLineStart + visibleLineCount);
  $: selectedSearchLine = searchMatches[selectedSearchMatch] ?? -1;
  $: visibleProgramRows = programRows(snapshot, visibleLineStart, visibleLineEnd, searchMatchSet, selectedSearchLine);
  $: if (selectedSearchMatch >= searchMatches.length && searchMatches.length > 0) {
    selectedSearchMatch = searchMatches.length - 1;
  }
  $: if (searchMatches.length === 0 && selectedSearchMatch !== 0) {
    selectedSearchMatch = 0;
  }
  $: connected = snapshot?.status.ready ?? false;
  $: mode = snapshot?.status.mode ?? "Disconnected";
  $: programLoaded = (snapshot?.program.lines.length ?? 0) > 0;
  $: programRunning = snapshot?.program.running ?? false;
  $: connectionBusy = mode === "Connecting";
  $: canConnect = !connectionBusy;
  $: canRun = connected && programLoaded && !programRunning && mode === "Idle";
  $: canHold = connected && (programRunning || mode === "Running" || (snapshot?.program.total_bytes_in_flight ?? 0) > 0);
  $: canResume = connected && (mode === "Hold" || mode === "Draining" || snapshot?.program.tool_change_line !== null);
  $: canRunOrResume = canRun || canResume;
  $: canStop = connected && (programRunning || mode === "Running" || mode === "Jogging" || mode === "Hold" || mode === "Draining" || mode === "Alarm" || mode === "Error");
  $: canDrain = connected && (programRunning || mode === "Running" || mode === "Hold");
  $: canSingle = connected && programLoaded && !programRunning && mode === "Idle";
  $: canMdi = connected;
  $: canJog = connected && (mode === "Idle" || mode === "Jogging");
  $: canSetWorkPosition = connected && (mode === "Idle" || mode === "Hold");
  $: canOverride = connected;
  $: canLoadGcode =
    !programRunning
    && (snapshot?.program.total_bytes_in_flight ?? 0) === 0
    && snapshot?.program.tool_change_line === null
    && !["Running", "Jogging", "Hold", "Draining"].includes(mode);
  $: canChangeStartLine = connected && programLoaded && canLoadGcode;
  $: aAxisAvailable = axisAvailable("A");
  $: toolpathRangeX = toolpathProgramRange(snapshot, "x");
  $: toolpathRangeY = toolpathProgramRange(snapshot, "y");
  $: toolpathRangeZ = toolpathProgramRange(snapshot, "z");
  $: toolpathRangeA = toolpathProgramRange(snapshot, "a");

  function matchingProgramLines(lines: string[], query: string) {
    const needle = query.trim().toLowerCase();
    if (!needle) return [];
    return lines
      .map((line, index) => ({ line, index }))
      .filter(({ line }) => line.toLowerCase().includes(needle))
      .map(({ index }) => index);
  }

  function programRows(
    current: Snapshot | null,
    start: number,
    end: number,
    matches: Set<number>,
    selectedLine: number
  ) {
    if (!current) return [];
    return current.program.lines.slice(start, end).map((line, offset) => {
      const index = start + offset;
      const queued = current.program.queued_lines.includes(index);
      return {
        line,
        index,
        active: current.program.current_line === index,
        queued: current.program.current_line !== index && queued,
        sent: index < current.program.next_line && current.program.current_line !== index && !queued,
        next: index === current.program.next_line && !current.program.running,
        pause: current.program.pause_after_line === index,
        toolChange: current.program.tool_change_line === index,
        match: matches.has(index),
        selectedMatch: selectedLine === index
      };
    });
  }

  function selectSearchMatch(index: number) {
    if (searchMatches.length === 0) return;
    selectedSearchMatch = (index + searchMatches.length) % searchMatches.length;
    scrollProgramLineIntoView(searchMatches[selectedSearchMatch], "nearest");
  }

  function toggleGcodeAutoScroll() {
    autoScrollGcode = !autoScrollGcode;
    if (autoScrollGcode) {
      lastScrollTarget = null;
      scrollProgramLineIntoView(latestProgramLineTarget(), "center");
    }
  }

  function programSearchKeydown(event: KeyboardEvent) {
    if (event.key === "Enter") {
      event.preventDefault();
      selectSearchMatch(selectedSearchMatch + (event.shiftKey ? -1 : 1));
    } else if (event.key === "Escape") {
      event.preventDefault();
      programSearch = "";
      selectedSearchMatch = 0;
    }
  }

  onMount(() => {
    refresh();
    refreshPorts();
    const blur = () => stopJog();
    window.addEventListener("keydown", keydown);
    window.addEventListener("keyup", keyup);
    window.addEventListener("blur", blur);
    const resizeObserver = new ResizeObserver(updateProgramViewport);
    const canvasResizeObserver = new ResizeObserver(() => {
      invalidateToolpath();
    });
    tick().then(() => {
      updateProgramViewport();
      if (programListViewport) {
        resizeObserver.observe(programListViewport);
      }
      if (baseCanvas) {
        canvasResizeObserver.observe(baseCanvas);
      }
    });
    let unlistenSnapshot: (() => void) | undefined;
    listen<Snapshot>("machine:snapshot", (event) => {
      applySnapshot(event.payload);
    }).then((unlisten) => {
      unlistenSnapshot = unlisten;
    });
    let unlistenLive: (() => void) | undefined;
    listen<LiveSnapshot>("machine:live", (event) => {
      applyLiveSnapshot(event.payload);
    }).then((unlisten) => {
      unlistenLive = unlisten;
    });
    return () => {
      window.removeEventListener("keydown", keydown);
      window.removeEventListener("keyup", keyup);
      window.removeEventListener("blur", blur);
      resizeObserver.disconnect();
      canvasResizeObserver.disconnect();
      stopJog();
      unlistenSnapshot?.();
      unlistenLive?.();
    };
  });

  afterUpdate(() => {
    const baseDrawKey = toolpathBaseDrawKey();
    if (baseDrawKey !== lastBaseDrawKey) {
      lastBaseDrawKey = baseDrawKey;
      drawToolpathBase();
      lastOverlayDrawKey = "";
    }
    const overlayDrawKey = toolpathOverlayDrawKey();
    if (overlayDrawKey !== lastOverlayDrawKey) {
      lastOverlayDrawKey = overlayDrawKey;
      drawToolpathOverlay();
    }
    if (autoScrollGcode) {
      const scrollTarget = latestProgramLineTarget();
      if (scrollTarget !== lastScrollTarget) {
        lastScrollTarget = scrollTarget;
        scrollProgramLineIntoView(scrollTarget, "center");
      }
    }
    const consoleKey = snapshot?.console[0]?.at_ms.toString() ?? "";
    if (consoleLines && consoleKey !== lastConsoleKey) {
      lastConsoleKey = consoleKey;
      consoleLines.scrollTop = 0;
    }
  });

  $: searchMatchSet = new Set(searchMatches);

  function updateProgramViewport() {
    if (!programListViewport) return;
    programScrollTop = programListViewport.scrollTop;
    programViewportHeight = programListViewport.clientHeight || programViewportHeight;
  }

  function currentProgramLineTarget() {
    if (!snapshot) return null;
    return searchMatches[selectedSearchMatch] ?? snapshot.program.current_line ?? snapshot.program.next_line;
  }

  function latestProgramLineTarget() {
    if (!snapshot) return null;
    return snapshot.program.current_line ?? snapshot.program.next_line;
  }

  function toolpathBaseDrawKey() {
    if (!snapshot) return "";
    const points = snapshot.program.toolpath.points;
    return [
      points.length,
      snapshot.program.filename ?? "",
      toolpathZoom.toFixed(3),
      toolpathPanX.toFixed(1),
      toolpathPanY.toFixed(1),
      sidebarWidth.toFixed(0)
    ].join("|");
  }

  function toolpathOverlayDrawKey() {
    if (!snapshot) return "";
    const current = snapshot.program.current_line ?? -1;
    const queued = snapshot.program.queued_lines.join(",");
    const wpos = snapshot.status.wpos;
    return [
      snapshot.program.next_line,
      current,
      queued,
      hoveredProgramLine ?? -1,
      wpos.x.toFixed(3),
      wpos.y.toFixed(3)
    ].join("|");
  }

  function clampSidebarWidth(width: number) {
    return Math.min(maxSidebarWidth, Math.max(minSidebarWidth, width));
  }

  function clampLeftPanelWidth(width: number) {
    return Math.min(maxLeftPanelWidth, Math.max(minLeftPanelWidth, width));
  }

  function startLeftPanelResize(event: PointerEvent) {
    if (!workArea) return;
    event.preventDefault();
    const bounds = workArea.getBoundingClientRect();
    const move = (moveEvent: PointerEvent) => {
      leftPanelWidth = clampLeftPanelWidth(moveEvent.clientX - bounds.left);
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      void saveUiSettings();
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop, { once: true });
    move(event);
  }

  function leftPanelResizeKeydown(event: KeyboardEvent) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    if (event.key === "ArrowLeft") {
      leftPanelWidth = clampLeftPanelWidth(leftPanelWidth - 24);
    } else if (event.key === "ArrowRight") {
      leftPanelWidth = clampLeftPanelWidth(leftPanelWidth + 24);
    } else if (event.key === "Home") {
      leftPanelWidth = minLeftPanelWidth;
    } else if (event.key === "End") {
      leftPanelWidth = maxLeftPanelWidth;
    }
    void saveUiSettings();
  }

  function startSidebarResize(event: PointerEvent) {
    if (!workArea) return;
    event.preventDefault();
    const bounds = workArea.getBoundingClientRect();
    const move = (moveEvent: PointerEvent) => {
      sidebarWidth = clampSidebarWidth(bounds.right - moveEvent.clientX);
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      void saveUiSettings();
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop, { once: true });
    move(event);
  }

  function sidebarResizeKeydown(event: KeyboardEvent) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    if (event.key === "ArrowLeft") {
      sidebarWidth = clampSidebarWidth(sidebarWidth + 24);
    } else if (event.key === "ArrowRight") {
      sidebarWidth = clampSidebarWidth(sidebarWidth - 24);
    } else if (event.key === "Home") {
      sidebarWidth = minSidebarWidth;
    } else if (event.key === "End") {
      sidebarWidth = maxSidebarWidth;
    }
    void saveUiSettings();
  }

  function invalidateToolpath() {
    lastBaseDrawKey = "";
    lastOverlayDrawKey = "";
    drawToolpathBase();
    drawToolpathOverlay();
  }

  function resizeCanvasForDisplay(target: HTMLCanvasElement) {
    const rect = target.getBoundingClientRect();
    const width = Math.max(1, rect.width);
    const height = Math.max(1, rect.height);
    const dpr = Math.max(1, window.devicePixelRatio || 1);
    const pixelWidth = Math.round(width * dpr);
    const pixelHeight = Math.round(height * dpr);
    if (target.width !== pixelWidth || target.height !== pixelHeight) {
      target.width = pixelWidth;
      target.height = pixelHeight;
    }
    return { width, height, dpr };
  }

  function resizeToolpathCanvases() {
    if (!baseCanvas || !canvas) return { width: 0, height: 0, dpr: 1 };
    const size = resizeCanvasForDisplay(baseCanvas);
    const pixelWidth = baseCanvas.width;
    const pixelHeight = baseCanvas.height;
    if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
      canvas.width = pixelWidth;
      canvas.height = pixelHeight;
    }
    return size;
  }

  function toolpathTransform(width: number, height: number) {
    const preview = toolpathPreviewBounds();
    const { min, max } = preview;
    const spanX = Math.max(max.x - min.x, 1);
    const spanY = Math.max(max.y - min.y, 1);
    const margin = 18;
    const baseScale = Math.min((width - margin * 2) / spanX, (height - margin * 2) / spanY);
    const scale = baseScale * toolpathZoom;
    const centerX = (min.x + max.x) / 2;
    const centerY = (min.y + max.y) / 2;
    return {
      ...preview,
      map: (x: number, y: number) => [
        width / 2 + toolpathPanX + (x - centerX) * scale,
        height / 2 + toolpathPanY - (y - centerY) * scale
      ],
      unmap: (x: number, y: number) => ({
        x: centerX + (x - width / 2 - toolpathPanX) / scale,
        y: centerY - (y - height / 2 - toolpathPanY) / scale
      })
    };
  }

  function openToolpathMenu(event: MouseEvent) {
    if (!snapshot || !canvas || snapshot.program.toolpath.points.length < 2) return;
    event.preventDefault();
    event.stopPropagation();
    const rect = canvas.getBoundingClientRect();
    const transform = toolpathTransform(rect.width, rect.height);
    const work = transform.unmap(event.clientX - rect.left, event.clientY - rect.top);
    toolpathMenu = {
      screenX: Math.min(event.clientX, window.innerWidth - 190),
      screenY: Math.min(event.clientY, window.innerHeight - 118),
      workX: work.x,
      workY: work.y
    };
  }

  function updateToolpathHover(event: PointerEvent) {
    if (toolpathDrag) {
      toolpathPanX += event.clientX - toolpathDrag.x;
      toolpathPanY += event.clientY - toolpathDrag.y;
      toolpathDrag = { pointerId: toolpathDrag.pointerId, x: event.clientX, y: event.clientY };
    }
    if (!snapshot || !canvas || snapshot.program.toolpath.points.length < 2) {
      toolpathHover = null;
      return;
    }
    updateToolpathHoverAt(event.clientX, event.clientY);
  }

  function updateToolpathHoverAt(clientX: number, clientY: number) {
    if (!snapshot || !canvas || snapshot.program.toolpath.points.length < 2) {
      toolpathHover = null;
      return;
    }
    const rect = canvas.getBoundingClientRect();
    const transform = toolpathTransform(rect.width, rect.height);
    const work = transform.unmap(clientX - rect.left, clientY - rect.top);
    toolpathHover = {
      workX: work.x,
      workY: work.y
    };
  }

  function startToolpathPan(event: PointerEvent) {
    if (event.button !== 0 || !canvas) return;
    event.preventDefault();
    toolpathMenu = null;
    toolpathDrag = { pointerId: event.pointerId, x: event.clientX, y: event.clientY };
    canvas.setPointerCapture(event.pointerId);
  }

  function stopToolpathPan(event: PointerEvent) {
    if (toolpathDrag?.pointerId !== event.pointerId) return;
    toolpathDrag = null;
    if (canvas.hasPointerCapture(event.pointerId)) {
      canvas.releasePointerCapture(event.pointerId);
    }
  }

  function zoomToolpath(event: WheelEvent) {
    if (!snapshot || !canvas || snapshot.program.toolpath.points.length < 2) return;
    event.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const pointerX = event.clientX - rect.left;
    const pointerY = event.clientY - rect.top;
    const before = toolpathTransform(rect.width, rect.height).unmap(pointerX, pointerY);
    const factor = Math.exp(-event.deltaY * 0.001);
    toolpathZoom = Math.min(16, Math.max(0.1, toolpathZoom * factor));
    const after = toolpathTransform(rect.width, rect.height).map(before.x, before.y);
    toolpathPanX += pointerX - after[0];
    toolpathPanY += pointerY - after[1];
    updateToolpathHoverAt(event.clientX, event.clientY);
  }

  function drawToolpathBase() {
    if (!snapshot || !baseCanvas) return;
    const ctx = baseCanvas.getContext("2d");
    if (!ctx) return;
    const { width, height, dpr } = resizeToolpathCanvases();
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, width, height);
    ctx.fillStyle = "#000000";
    ctx.fillRect(0, 0, width, height);

    const points = snapshot.program.toolpath.points;
    if (points.length < 2) {
      return;
    }

    const transform = toolpathTransform(width, height);

    drawDepthLayer(ctx, points, width, height, transform);
  }

  function drawToolpathOverlay() {
    if (!snapshot || !canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const { width, height, dpr } = resizeToolpathCanvases();
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, width, height);

    const points = snapshot.program.toolpath.points;
    if (points.length < 2) {
      drawCrosshair(ctx, width / 2, height / 2, "#7f8791");
      return;
    }

    const transform = toolpathTransform(width, height);

    if (hoveredProgramLine !== null) {
      drawLineToolpathHighlight(ctx, points, hoveredProgramLine, transform);
    }

    if (snapshot.program.current_line !== null) {
      const currentIndex = lastPointIndexForLine(snapshot.program.current_line);
      const current = points[currentIndex] ?? snapshot.status.wpos;
      const [px, py] = transform.map(current.x, current.y);
      drawCrosshair(ctx, px, py, "#f0c36a");
    }

    const [cx, cy] = transform.map(snapshot.status.wpos.x, snapshot.status.wpos.y);
    drawCrosshair(ctx, cx, cy, "#62c3a6");
  }

  function drawDepthLayer(
    ctx: CanvasRenderingContext2D,
    points: Snapshot["program"]["toolpath"]["points"],
    width: number,
    height: number,
    transform: ReturnType<typeof toolpathTransform>
  ) {
    const key = depthLayerCacheKey(width, height, points);
    if (!depthLayerCanvas || depthLayerKey !== key) {
      depthLayerCanvas = renderDepthLayer(width, height, points, transform);
      depthLayerKey = key;
    }
    ctx.drawImage(depthLayerCanvas, 0, 0, width, height);
  }

  function depthLayerCacheKey(
    width: number,
    height: number,
    points: Snapshot["program"]["toolpath"]["points"]
  ) {
    const first = points[0];
    const last = points[points.length - 1];
    return [
      snapshot?.program.filename ?? "",
      points.length,
      width.toFixed(0),
      height.toFixed(0),
      toolpathZoom.toFixed(4),
      toolpathPanX.toFixed(1),
      toolpathPanY.toFixed(1),
      first ? `${first.x.toFixed(3)},${first.y.toFixed(3)},${first.z.toFixed(3)}` : "",
      last ? `${last.x.toFixed(3)},${last.y.toFixed(3)},${last.z.toFixed(3)}` : ""
    ].join("|");
  }

  function renderDepthLayer(
    width: number,
    height: number,
    points: Snapshot["program"]["toolpath"]["points"],
    transform: ReturnType<typeof toolpathTransform>
  ) {
    const layer = document.createElement("canvas");
    layer.width = Math.max(1, Math.round(width));
    layer.height = Math.max(1, Math.round(height));
    const layerContext = layer.getContext("2d");
    if (!layerContext) return layer;

    const zMin = transform.min.z;
    const zMax = transform.max.z;
    const zSpan = Math.max(zMax - zMin, 0.000001);
    const bucketCount = 64;
    const buckets = Array.from({ length: bucketCount }, () => new Path2D());

    for (let index = Math.max(1, transform.firstSegmentIndex); index < points.length; index += 1) {
      const previous = points[index - 1];
      const point = points[index];
      const [x1, y1] = transform.map(previous.x, previous.y);
      const [x2, y2] = transform.map(point.x, point.y);
      const depth = (zMax - point.z) / zSpan;
      const bucket = Math.round(Math.min(1, Math.max(0, depth)) * (bucketCount - 1));
      buckets[bucket].moveTo(x1, y1);
      buckets[bucket].lineTo(x2, y2);
    }

    layerContext.globalCompositeOperation = "lighten";
    layerContext.lineWidth = 1.35;
    layerContext.lineCap = "round";
    layerContext.lineJoin = "round";
    for (let bucket = 0; bucket < buckets.length; bucket += 1) {
      const value = Math.round(64 + (bucket / (bucketCount - 1)) * 191);
      layerContext.strokeStyle = `rgb(${value}, ${value}, ${value})`;
      layerContext.stroke(buckets[bucket]);
    }
    layerContext.globalCompositeOperation = "source-over";
    return layer;
  }

  function drawLineToolpathHighlight(
    ctx: CanvasRenderingContext2D,
    points: Snapshot["program"]["toolpath"]["points"],
    sourceLine: number,
    transform: ReturnType<typeof toolpathTransform>
  ) {
    if (!snapshot) return;
    const sources = snapshot.program.toolpath.point_source_lines;
    const path = new Path2D();
    let hasPath = false;
    for (let index = Math.max(1, transform.firstSegmentIndex); index < points.length; index += 1) {
      if (sources[index] !== sourceLine) continue;
      const previous = points[index - 1];
      const point = points[index];
      const [x1, y1] = transform.map(previous.x, previous.y);
      const [x2, y2] = transform.map(point.x, point.y);
      path.moveTo(x1, y1);
      path.lineTo(x2, y2);
      hasPath = true;
    }
    if (!hasPath) return;
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    ctx.strokeStyle = "#ff3333";
    ctx.lineWidth = 2.5;
    ctx.stroke(path);
  }

  function toolpathPreviewBounds() {
    const points = snapshot?.program.toolpath.points ?? [];
    const axes = ["x", "y", "z", "a"] as const;
    const min = { x: 0, y: 0, z: 0, a: 0 };
    const max = { x: 0, y: 0, z: 0, a: 0 };
    if (points.length === 0) {
      return { min, max, firstSegmentIndex: 1 };
    }

    const firstPointIndex = firstPreviewPointIndex(points);
    const first = points[firstPointIndex] ?? points[0];
    for (const axis of axes) {
      min[axis] = first[axis];
      max[axis] = first[axis];
    }

    for (let index = firstPointIndex; index < points.length; index += 1) {
      const point = points[index];
      for (const axis of axes) {
        min[axis] = Math.min(min[axis], point[axis]);
        max[axis] = Math.max(max[axis], point[axis]);
      }
    }

    return { min, max, firstSegmentIndex: firstPointIndex + 1 };
  }

  function firstPreviewPointIndex(points: Snapshot["program"]["toolpath"]["points"]) {
    const epsilon = 0.000001;
    for (let index = 1; index < points.length; index += 1) {
      const previous = points[index - 1];
      const point = points[index];
      if (Math.abs(point.x - previous.x) > epsilon || Math.abs(point.y - previous.y) > epsilon) {
        return index;
      }
    }
    return 0;
  }

  function lastPointIndexForLine(sourceLine: number) {
    if (!snapshot) return 0;
    const lines = snapshot.program.toolpath.point_source_lines;
    for (let index = lines.length - 1; index >= 0; index -= 1) {
      if (lines[index] === sourceLine) return index;
    }
    return 0;
  }

  function drawCrosshair(ctx: CanvasRenderingContext2D, x: number, y: number, color: string) {
    ctx.strokeStyle = color;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x - 8, y);
    ctx.lineTo(x + 8, y);
    ctx.moveTo(x, y - 8);
    ctx.lineTo(x, y + 8);
    ctx.stroke();
  }

  function formatDuration(seconds: number) {
    const total = Math.max(0, Math.round(seconds));
    const minutes = Math.floor(total / 60);
    const remainingSeconds = total % 60;
    if (minutes >= 60) {
      const hours = Math.floor(minutes / 60);
      return `${hours}h ${minutes % 60}m`;
    }
    return `${minutes}m ${remainingSeconds.toString().padStart(2, "0")}s`;
  }

  function formatBytes(bytes: number) {
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${bytes} B`;
  }

  function formatMm(value: number) {
    return `${value.toFixed(3)} mm`;
  }

  function remainingBreakdown(current: Snapshot | null) {
    if (!current) {
      return {
        displayed: 0,
        live: undefined as number | undefined,
        local: 0,
        lineRemaining: 0,
        elapsedRemaining: 0,
        tableLength: 0,
        lineTimeLength: 0,
        toolTotal: 0,
        line: 0,
        startLine: 0,
        elapsed: 0,
        mode: "-",
        ready: false,
        filename: "-",
        lines: 0,
        points: 0,
        keys: "-"
      };
    }
    const remaining = current.program.toolpath.remaining_seconds_by_line;
    const line = current.program.current_line ?? current.program.next_line;
    const boundedLine = remaining.length > 0 ? Math.min(line, remaining.length - 1) : 0;
    const lineRemaining = remaining[boundedLine] ?? 0;
    const cycleActive = ["Running", "Hold", "Draining"].includes(current.status.mode)
      || current.program.running
      || current.program.total_bytes_in_flight > 0;
    const startLine = remaining.length > 0
      ? Math.min(current.program.run_started_line ?? 0, remaining.length - 1)
      : 0;
    const elapsedRemaining = cycleActive
      ? Math.max(0, (remaining[startLine] ?? 0) - current.program.running_elapsed_seconds)
      : 0;
    const localRemaining = Math.max(lineRemaining, elapsedRemaining);
    const liveRemaining = current.program.remaining_seconds;
    const displayed = liveRemaining === undefined || (liveRemaining <= 0 && localRemaining > 0)
      ? localRemaining
      : Math.max(liveRemaining, localRemaining);
    return {
      displayed,
      local: localRemaining
    };
  }

  $: remainingData = remainingBreakdown(snapshot);
  $: remainingDisplay = remainingData.displayed;

  function toolpathProgramRange(current: Snapshot | null, axis: "x" | "y" | "z" | "a") {
    if (!current) return "-";
    const { min, max } = current.program.toolpath;
    return `${min[axis].toFixed(3)} .. ${max[axis].toFixed(3)} mm`;
  }

  function scrollCurrentProgramLineIntoView() {
    scrollProgramLineIntoView(currentProgramLineTarget(), "center");
  }

  function scrollProgramLineIntoView(targetLine: number | null, mode: "nearest" | "center") {
    if (targetLine === null || !programListViewport) return;
    const top = targetLine * programLineHeight;
    const bottom = top + programLineHeight;
    if (mode === "center") {
      const scrollTop = top - (programListViewport.clientHeight - programLineHeight) / 2;
      programListViewport.scrollTop = Math.max(0, scrollTop);
      updateProgramViewport();
      return;
    }
    if (top < programListViewport.scrollTop) {
      programListViewport.scrollTop = top;
      updateProgramViewport();
      return;
    }
    if (bottom > programListViewport.scrollTop + programListViewport.clientHeight) {
      programListViewport.scrollTop = bottom - programListViewport.clientHeight;
      updateProgramViewport();
      return;
    }
  }
</script>

{#if snapshot}
  <main>
    <section class="toolbar">
      <button on:click={connectAuto} disabled={!canConnect}>AUTO</button>
      <select bind:value={selectedPort} aria-label="Serial port" disabled={!canConnect}>
        <option value="">No serial port</option>
        {#each ports as port}
          <option value={port}>{port}</option>
        {/each}
      </select>
      <button on:click={refreshPorts} disabled={!canConnect}>PORTS</button>
      <button on:click={connectSelectedPort} disabled={!selectedPort || !canConnect}>CONNECT</button>
      <button on:click|stopPropagation={showPathOpen} disabled={!canLoadGcode}>OPEN</button>
      <button on:click|stopPropagation={openGcodeFile} disabled={!canLoadGcode}>BROWSE</button>
      <button on:click={runOrResumeProgram} disabled={!canRunOrResume}>{canResume ? "RESUME" : "RUN"}</button>
      <button on:click={() => command("pause_program")} disabled={!canHold}>HOLD</button>
      <button on:click={() => command("stop_program")} disabled={!canStop}>STOP</button>
      <button on:click={drainProgram} disabled={!canDrain}>DRAIN</button>
      <button on:click={() => command("single_step")} disabled={!canSingle}>SINGLE</button>
      <button on:click={() => command("toggle_optional_stop")}>
        {snapshot.program.optional_stop ? "DISABLE M1" : "ENABLE M1"}
      </button>
      <button on:click={toggleShortcuts}>?</button>
      <button on:click={() => command("unlock_alarm")} disabled={!connected}>UNLOCK</button>
      <button on:click={disconnect} disabled={!connected && !connectionBusy}>DISCONNECT</button>
    </section>

    {#if showFileOpen}
      <section class="file-open">
        <form on:submit|preventDefault={loadPathFromInput}>
          <span>Path</span>
          <input
            bind:this={filePathInput}
            bind:value={filePath}
            on:input={filePathInputChanged}
            on:keydown={filePathKeydown}
            autocomplete="off"
            spellcheck="false"
          />
          <button disabled={!canLoadGcode}>Load</button>
          <button type="button" on:click={closeFileOpen}>Close</button>
        </form>
        <div class="file-candidates">
          {#each fileCandidates as candidate, index}
            <button
              type="button"
              class:selected={index === selectedFileCandidate}
              on:click={() => selectFileCandidate(index)}
              on:dblclick={() => chooseFileCandidate(candidate)}
            >
              <span>{candidate.is_dir ? "DIR" : "GCODE"}</span>
              {candidate.label}
            </button>
          {/each}
        </div>
        <div class="file-preview">
          {#if filePreview}
            <div class="file-preview-head">
              <strong>{filePreview.filename}</strong>
              <span>{filePreview.line_count} lines - {formatBytes(filePreview.size_bytes)}</span>
            </div>
            <pre>{filePreview.preview_lines.join("\n")}</pre>
          {:else if filePreviewError}
            <span>{filePreviewError}</span>
          {/if}
        </div>
      </section>
    {/if}

    <section
      class="app-grid"
      bind:this={workArea}
      style:grid-template-columns={`${leftPanelWidth}px 8px minmax(0, 1fr) 8px ${sidebarWidth}px`}
    >
      <section class="left-panel">
        <section class="dro">
          <div
            class="mode"
            class:mode-red={snapshot.status.mode === "Disconnected" || snapshot.status.mode === "Alarm" || snapshot.status.mode === "Error"}
            class:mode-orange={snapshot.status.mode === "Connecting" || snapshot.status.mode === "Idle" || snapshot.status.mode === "Jogging" || snapshot.status.mode === "Hold" || snapshot.status.mode === "Draining"}
            class:mode-green={snapshot.status.mode === "Running"}
          >
            {snapshot.status.mode}
          </div>
          <div class="port">{snapshot.status.port_name}</div>
          <div class="coords">
            <button class:editing={droEditAxis === "X"} disabled={!canSetWorkPosition} on:click={() => startDroEdit("X")}>X {snapshot.status.wpos.x.toFixed(3)}</button>
            <button class:editing={droEditAxis === "Y"} disabled={!canSetWorkPosition} on:click={() => startDroEdit("Y")}>Y {snapshot.status.wpos.y.toFixed(3)}</button>
            <button class:editing={droEditAxis === "Z"} disabled={!canSetWorkPosition} on:click={() => startDroEdit("Z")}>Z {snapshot.status.wpos.z.toFixed(3)}</button>
            <button class:editing={droEditAxis === "A"} disabled={!canSetWorkPosition} on:click={() => startDroEdit("A")}>A {snapshot.status.wpos.a.toFixed(3)}</button>
          </div>
          {#if droEditAxis}
            <form class="dro-editor" on:submit|preventDefault={submitDroEdit}>
              <span>{droEditAxis}</span>
              <input
                bind:this={droInput}
                bind:value={droExpression}
                on:input={() => droError = ""}
                on:keydown={(event) => {
                  if (event.key === "Escape") {
                    event.preventDefault();
                    closeDroEdit();
                  }
                }}
                autocomplete="off"
                spellcheck="false"
              />
              <button>SET</button>
              <button type="button" on:click={closeDroEdit}>CANCEL</button>
              {#if droError}
                <strong>{droError}</strong>
              {/if}
            </form>
          {/if}
        </section>

        {#if commandError || snapshot.status.last_alarm || snapshot.status.last_error || snapshot.program.tool_change_line !== null}
          <section class="alert">
            {commandError || snapshot.status.last_alarm || snapshot.status.last_error || (snapshot.program.tool_change_line === null ? "" : `Tool change at line ${snapshot.program.tool_change_line + 1}. Change tool, touch off if needed, then resume.`)}
          </section>
        {/if}

        <div class="jog">
          <button disabled={!canJog} on:pointerdown={() => startJog("Y", 1, "button-y-plus")} on:pointerup={() => stopJog("button-y-plus")} on:pointercancel={() => stopJog("button-y-plus")} on:mouseleave={() => stopJog("button-y-plus")}>Y+</button>
          <button disabled={!canJog} on:pointerdown={() => startJog("X", -1, "button-x-minus")} on:pointerup={() => stopJog("button-x-minus")} on:pointercancel={() => stopJog("button-x-minus")} on:mouseleave={() => stopJog("button-x-minus")}>X-</button>
          <button disabled={!canJog} on:pointerdown={() => startJog("X", 1, "button-x-plus")} on:pointerup={() => stopJog("button-x-plus")} on:pointercancel={() => stopJog("button-x-plus")} on:mouseleave={() => stopJog("button-x-plus")}>X+</button>
          <button disabled={!canJog} on:pointerdown={() => startJog("Y", -1, "button-y-minus")} on:pointerup={() => stopJog("button-y-minus")} on:pointercancel={() => stopJog("button-y-minus")} on:mouseleave={() => stopJog("button-y-minus")}>Y-</button>
          <button disabled={!canJog} on:pointerdown={() => startJog("Z", 1, "button-z-plus")} on:pointerup={() => stopJog("button-z-plus")} on:pointercancel={() => stopJog("button-z-plus")} on:mouseleave={() => stopJog("button-z-plus")}>Z+</button>
          <button disabled={!canJog} on:pointerdown={() => startJog("Z", -1, "button-z-minus")} on:pointerup={() => stopJog("button-z-minus")} on:pointercancel={() => stopJog("button-z-minus")} on:mouseleave={() => stopJog("button-z-minus")}>Z-</button>
          <button disabled={!canJog || !aAxisAvailable} on:pointerdown={() => startJog("A", 1, "button-a-plus")} on:pointerup={() => stopJog("button-a-plus")} on:pointercancel={() => stopJog("button-a-plus")} on:mouseleave={() => stopJog("button-a-plus")}>A+</button>
          <button disabled={!canJog || !aAxisAvailable} on:pointerdown={() => startJog("A", -1, "button-a-minus")} on:pointerup={() => stopJog("button-a-minus")} on:pointercancel={() => stopJog("button-a-minus")} on:mouseleave={() => stopJog("button-a-minus")}>A-</button>
        </div>
        <label>Inc <input type="number" step="0.001" bind:value={jogIncrement} on:change={saveUiSettings} /></label>
        <label>Feed <input type="number" step="1" bind:value={jogFeed} on:change={saveUiSettings} /></label>
        <label><input type="checkbox" bind:checked={rapidJog} on:change={saveUiSettings} /> Rapid</label>
        <label><input type="checkbox" bind:checked={keyboardShortcutsEnabled} on:change={saveUiSettings} /> Keys</label>
        <label><input type="checkbox" bind:checked={keyboardJogRequiresShift} on:change={saveUiSettings} disabled={!keyboardShortcutsEnabled} /> Shift jog</label>
        <div class="status-grid">
          <span>F</span><strong>{snapshot.status.feed_rate.toFixed(0)}</strong>
          <span>S</span><strong>{snapshot.status.spindle_speed.toFixed(0)}</strong>
          <span>Probe</span><strong>{snapshot.status.probe ? "ON" : "off"}</strong>
          <span>Pins</span><strong>{snapshot.status.pins || "-"}</strong>
          <span>Planner</span><strong>{snapshot.status.planner_free}</strong>
          <span>Serial</span><strong>{snapshot.status.serial_free}/{snapshot.status.serial_size}</strong>
          <span>Total</span><strong>{formatDuration(snapshot.program.estimated_seconds ?? snapshot.program.toolpath.estimated_seconds)}</strong>
          <span>Elapsed</span><strong>{formatDuration(snapshot.program.running_elapsed_seconds)}</strong>
          <span>Remain</span><strong>{formatDuration(remainingDisplay)}</strong>
          <span>Codes</span><strong>{snapshot.status.active_codes || "-"}</strong>
        </div>
        <div class="controller">
          <div>
            <span>Controller</span>
            <strong>{snapshot.status.capabilities.firmware ?? "unknown"}</strong>
          </div>
          <div>
            <span>Version</span>
            <strong>{snapshot.status.capabilities.version ?? "-"} {snapshot.status.capabilities.build ?? ""}</strong>
          </div>
          <div>
            <span>Axes</span>
            <strong>{snapshot.status.capabilities.axis_letters || `${snapshot.status.axis_count}`}</strong>
          </div>
          <div>
            <span>Board</span>
            <strong>{snapshot.status.capabilities.board ?? snapshot.status.capabilities.driver ?? "-"}</strong>
          </div>
          {#if snapshot.status.capabilities.newopt_flags.length > 0}
            <div class="cap-list">
              {#each snapshot.status.capabilities.newopt_flags as flag}
                <span>{flag}</span>
              {/each}
            </div>
          {:else if snapshot.status.capabilities.opt_flags.length > 0}
            <div class="cap-list">
              {#each snapshot.status.capabilities.opt_flags as flag}
                <span>{flag}</span>
              {/each}
            </div>
          {/if}
          {#if snapshot.status.capabilities.plugins.length > 0}
            <div class="cap-list">
              {#each snapshot.status.capabilities.plugins as plugin}
                <span>{plugin}</span>
              {/each}
            </div>
          {/if}
        </div>
        <div class="overrides">
          <div class="override-row">
            <span>Feed</span>
            <strong>{snapshot.status.feed_override.toFixed(0)}%</strong>
            <div class="override-buttons">
              <button disabled={!canOverride} on:click={() => command("adjust_feed_override", { delta: -10 })}>-10</button>
              <button disabled={!canOverride} on:click={() => command("adjust_feed_override", { delta: -1 })}>-1</button>
              <button disabled={!canOverride} on:click={() => command("adjust_feed_override", { delta: 0 })}>100</button>
              <button disabled={!canOverride} on:click={() => command("adjust_feed_override", { delta: 1 })}>+1</button>
              <button disabled={!canOverride} on:click={() => command("adjust_feed_override", { delta: 10 })}>+10</button>
            </div>
          </div>
          <div class="override-row rapid">
            <span>Rapid</span>
            <strong>{snapshot.status.rapid_override.toFixed(0)}%</strong>
            <div class="override-buttons">
              <button disabled={!canOverride} on:click={() => command("set_rapid_override", { value: 25 })}>25</button>
              <button disabled={!canOverride} on:click={() => command("set_rapid_override", { value: 50 })}>50</button>
              <button disabled={!canOverride} on:click={() => command("set_rapid_override", { value: 100 })}>100</button>
            </div>
          </div>
          <div class="override-row">
            <span>Spindle</span>
            <strong>{snapshot.status.spindle_override.toFixed(0)}%</strong>
            <div class="override-buttons">
              <button disabled={!canOverride} on:click={() => command("adjust_spindle_override", { delta: -10 })}>-10</button>
              <button disabled={!canOverride} on:click={() => command("adjust_spindle_override", { delta: -1 })}>-1</button>
              <button disabled={!canOverride} on:click={() => command("adjust_spindle_override", { delta: 0 })}>100</button>
              <button disabled={!canOverride} on:click={() => command("adjust_spindle_override", { delta: 1 })}>+1</button>
              <button disabled={!canOverride} on:click={() => command("adjust_spindle_override", { delta: 10 })}>+10</button>
            </div>
          </div>
        </div>
      </section>

      <button
        type="button"
        class="splitter"
        aria-label={`Resize controls panel, currently ${Math.round(leftPanelWidth)} pixels`}
        title="Drag to resize controls panel"
        on:pointerdown={startLeftPanelResize}
        on:keydown={leftPanelResizeKeydown}
      ></button>

      <div class="program">
        <div class="program-head">
          <span>{snapshot.program.filename ?? "No file loaded"}</span>
          <span>{snapshot.program.next_line}/{snapshot.program.lines.length}</span>
          <label>Line <input class="line-input" type="number" min="1" max={Math.max(1, snapshot.program.lines.length)} bind:value={runLine} /></label>
          <button on:click={setRunLine} disabled={!canChangeStartLine}>SET</button>
          <label>Pause <input class="line-input" type="number" min="1" max={Math.max(1, snapshot.program.lines.length)} bind:value={pauseAfterLine} /></label>
          <button on:click={setPauseAfterLine} disabled={!programLoaded}>ARM</button>
          <button on:click={clearPauseAfterLine} disabled={snapshot.program.pause_after_line === null}>CLEAR</button>
          <button class:armed={autoScrollGcode} on:click={toggleGcodeAutoScroll}>AUTO</button>
          <label>Find <input
            bind:this={programSearchInput}
            class="search-input"
            bind:value={programSearch}
            on:input={() => selectedSearchMatch = 0}
            on:keydown={programSearchKeydown}
            autocomplete="off"
            spellcheck="false"
          /></label>
          <button on:click={() => selectSearchMatch(selectedSearchMatch - 1)} disabled={searchMatches.length === 0}>PREV</button>
          <button on:click={() => selectSearchMatch(selectedSearchMatch + 1)} disabled={searchMatches.length === 0}>NEXT</button>
          <span>{searchMatches.length === 0 ? "0/0" : `${selectedSearchMatch + 1}/${searchMatches.length}`}</span>
          {#if snapshot.program.pause_after_line !== null}
            <span class="pause-arm">after {snapshot.program.pause_after_line + 1}</span>
          {/if}
          {#if snapshot.program.tool_change_line !== null}
            <span class="tool-change-arm">tool line {snapshot.program.tool_change_line + 1}</span>
          {/if}
        </div>
        <div class="program-list" bind:this={programListViewport} on:scroll={updateProgramViewport}>
          <div class="program-list-space" style:height={`${snapshot.program.lines.length * programLineHeight}px`}>
            <ol start={visibleLineStart + 1} style:transform={`translateY(${visibleLineStart * programLineHeight}px)`}>
          {#each visibleProgramRows as row}
            {@const index = row.index}
            <li
              data-line={index}
              class:active={row.active}
              class:queued={row.queued}
              class:sent={row.sent}
              class:next={row.next}
              class:pause={row.pause}
              class:toolChange={row.toolChange}
              class:match={row.match}
              class:selectedMatch={row.selectedMatch}
              class:hovered={hoveredProgramLine === index}
              on:mouseenter={() => hoveredProgramLine = index}
              on:mouseleave={() => hoveredProgramLine = null}
            >
              <span class="line-number">{index + 1}</span>
              <span class="line-text">{row.line || " "}</span>
              <span class="line-actions">
                <button on:click|stopPropagation={() => setRunLineTo(index)} disabled={!canChangeStartLine}>FROM</button>
                {#if snapshot.program.pause_after_line === index}
                  <button class="armed" on:click|stopPropagation={clearPauseAfterLine}>PAUSING</button>
                {:else}
                  <button on:click|stopPropagation={() => setPauseAfterLineTo(index)}>PAUSE</button>
                {/if}
              </span>
            </li>
          {/each}
            </ol>
          </div>
        </div>
      </div>

      <button
        type="button"
        class="splitter"
        aria-label={`Resize toolpath panel, currently ${Math.round(sidebarWidth)} pixels`}
        title="Drag to resize toolpath panel"
        on:pointerdown={startSidebarResize}
        on:keydown={sidebarResizeKeydown}
      ></button>

      <aside>
        <div class="toolpath-head">
          <span>Toolpath {toolpathZoom.toFixed(2)}x</span>
          <div>
            <button on:click={() => adjustToolpathZoom(0.8)}>-</button>
            <button on:click={resetToolpathZoom}>FIT</button>
            <button on:click={() => adjustToolpathZoom(1.25)}>+</button>
          </div>
        </div>
        <div class="toolpath-canvas-stack">
          <canvas bind:this={baseCanvas} aria-hidden="true"></canvas>
          <canvas
            bind:this={canvas}
            on:contextmenu={openToolpathMenu}
            on:pointerdown={startToolpathPan}
            on:pointermove={updateToolpathHover}
            on:pointerup={stopToolpathPan}
            on:pointercancel={stopToolpathPan}
            on:pointerleave={() => {
              if (!toolpathDrag) toolpathHover = null;
            }}
            on:wheel|nonpassive={zoomToolpath}
          ></canvas>
        </div>
        <div class="toolpath-meta">
          <span class="toolpath-cursor">
            Cursor
            {#if toolpathHover}
              X {toolpathHover.workX.toFixed(3)} Y {toolpathHover.workY.toFixed(3)}
            {:else}
              -
            {/if}
          </span>
          <span>X {toolpathRangeX}</span>
          <span>Y {toolpathRangeY}</span>
          <span>Z {toolpathRangeZ}</span>
          {#if snapshot.status.axis_count >= 4}
            <span>A {toolpathRangeA}</span>
          {/if}
        </div>
        <div class="toolpath-legend">
          <span class="shallow-run">High Z</span>
          <span class="deep-run">Deep Z</span>
          <span class="active-run">Current line</span>
          <span class="machine-run">Machine position</span>
        </div>
      </aside>
    </section>

    <section class="console">
      <header>
        <strong>Console</strong>
        <label><input type="checkbox" bind:checked={hideStateUpdates} on:change={() => setHideStateUpdates(hideStateUpdates)} /> Hide status</label>
      </header>
      <div class="console-lines" bind:this={consoleLines}>
        {#each snapshot.console as entry}
          <div class={entry.direction}><span>{entry.direction.toUpperCase()}</span>{entry.line}</div>
        {/each}
      </div>
    </section>

    <form class="mdi" on:submit|preventDefault={submitMdi}>
      <span>MDI&gt;</span>
      <input bind:this={mdiInput} bind:value={mdi} on:keydown={mdiKeydown} autocomplete="off" spellcheck="false" disabled={!canMdi} />
      <button disabled={!canMdi || !mdi.trim()}>Send</button>
    </form>

    <section class="messages">
      {#each snapshot.messages as message}
        <div class={message.level}>{message.text}</div>
      {/each}
    </section>

    {#if toolpathMenu}
      <section
        class="toolpath-menu"
        style:left={`${toolpathMenu.screenX}px`}
        style:top={`${toolpathMenu.screenY}px`}
      >
        <div>X {toolpathMenu.workX.toFixed(3)} Y {toolpathMenu.workY.toFixed(3)}</div>
        <button disabled={!canJog} on:click={jogToToolpathMenu}>Jog to here</button>
        <button disabled={!canSetWorkPosition} on:click={setWorkXyToToolpathMenu}>Set work XY here</button>
      </section>
    {/if}

    {#if showShortcuts}
      <section class="modal" role="presentation">
        <div class="sheet" role="dialog" aria-modal="true" aria-label="Keyboard shortcuts">
          <header>
            <strong>Shortcuts</strong>
            <button on:click={() => showShortcuts = false}>Close</button>
          </header>
          <dl>
            <dt>R</dt><dd>Run/resume</dd>
            <dt>H</dt><dd>Feed hold</dd>
            <dt>X/Y/Z/A</dt><dd>Edit DRO axis when idle</dd>
            <dt>O</dt><dd>Open path</dd>
            <dt>Arrow keys</dt><dd>Jog X/Y</dd>
            <dt>Page Up/Down</dt><dd>Jog Z</dd>
            <dt>Shift</dt><dd>Required for jog when enabled</dd>
            <dt>?</dt><dd>Toggle shortcuts</dd>
            <dt>MDI Up/Down</dt><dd>History</dd>
          </dl>
        </div>
      </section>
    {/if}
  </main>
{/if}
