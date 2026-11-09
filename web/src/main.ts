// main.ts — bootstrap, tick loop, and panel wiring.

import { loadWasm } from "./wasm";
import { newPlant, type PlantHandle } from "./plant";
import { loadDescriptions } from "./descriptions";
import { mountPlant } from "./svg";
import { renderPanel } from "./controls";
import { renderReadout, refreshSensors, type SensorRow } from "./sensors";
import { renderAlarms, type AlarmEntry, type AlarmPanel } from "./alarms";
import { initConsole, type ConsolePanel } from "./console";
import {
    bindReturnKey,
    buildBindings,
    displayPart,
    groupByPrefix,
    type PartBinding,
} from "./bindings";
import { enablePanelSwaps } from "./grid";
import { makeTile, applyMinimized } from "./tile";
import { copyText } from "./clipboard";
import {
    loadPanelLayout,
    savePanelLayout,
    PANEL_LAYOUT_VERSION,
} from "./layout";
import { Sim } from "./sim";
import {
    loadScenarios,
    objectiveStatusText,
    renderScenarioPicker,
    type ObjectiveStatus,
    type ScenarioInfo,
} from "./scenarios";
import { RemoteClient, RemoteError, type AlarmEntryRemote, type ControlEntry, type SensorEntry, type StatusView } from "./remote";
import { renderServerPicker } from "./serverPicker";
import { initLogs, type LogsPanel } from "./logs";
import { confirmControlChange } from "./confirmChange";
import {
    buildRemoteBindings,
    renderRemotePanel,
    renderRemoteReadout,
    refreshRemoteControls,
    refreshRemoteSensors,
} from "./remoteTiles";

const COPY_FEEDBACK_MS = 1500;
// Stepping runs on a wall-clock accumulator so the achieved rate
// tracks Steps/s regardless of frame jitter; after a hidden-tab
// gap at most this much sim time executes before the remainder is
// dropped (the simulation falls behind rather than freezing).
const MAX_CATCHUP_S = 1;
// Throughput accounting: the measured side counts real plant
// steps over a one-second window.
const SPS_WINDOW_MS = 1000;
// The alarm panel's source sensor inside the batched snapshot.
const ALARM_SENSOR_ID = "Plant/Alarms";

async function main(): Promise<void> {
    await loadWasm();
    await loadDescriptions();

    const pidEl = document.getElementById("pid") as HTMLElement;
    const tilesEl = document.getElementById("tiles") as HTMLElement;
    const alarmsEl = document.getElementById("alarms") as HTMLElement;
    const consoleEl = document.getElementById("console") as HTMLElement;
    const startBtn = document.getElementById("btn-start") as HTMLButtonElement;
    const stepBtn = document.getElementById("btn-step") as HTMLButtonElement;
    const resetBtn = document.getElementById("btn-reset") as HTMLButtonElement;
    const fpsInput = document.getElementById("fps-input") as HTMLInputElement;
    const spsInput = document.getElementById("sps-input") as HTMLInputElement;
    const copyBtn = document.getElementById("btn-copy") as HTMLButtonElement;
    const scenarioBtn = document.getElementById("btn-scenario") as HTMLButtonElement;
    const scenarioNameEl = document.getElementById("scenario-name") as HTMLElement;
    const statusEl = document.getElementById("sim-status") as HTMLElement;
    const spsEl = document.getElementById("sps-indicator") as HTMLElement;
    const serverBtn = document.getElementById("btn-server") as HTMLButtonElement;
    const writeModeBtn = document.getElementById("btn-write-mode") as HTMLButtonElement;
    const logsEl = document.getElementById("logs") as HTMLElement;

    // The factory closure reads the current selection at reset time,
    // so both the initial plant and every later Reset rebuild the
    // chosen scenario's starting state.
    let currentScenarioId = "";
    const sim = new Sim(() => newPlant(currentScenarioId));

    // Backtick returns to the view shared by a control jump (e.g.
    // clicking a P&ID component); see bindings.ts.
    bindReturnKey();

    // Drag a tile by its prefix heading to swap it with a
    // neighbour; see grid.ts for the gesture thresholds.
    enablePanelSwaps(tilesEl);

    // Initial SVG mount + tiles. Each top-level prefix gets its
    // own tile; the tile splits into Controls | Sensors columns.
    const initialSvg = mountSvg(pidEl, sim.plant);
    let lastSvgHtml = initialSvg;

    let sensorRows: SensorRow[] = [];
    const rebuildTiles = (): void => {
        const bindings = buildBindings(sim.plant);
        const groups = groupByPrefix(bindings);
        // Saved arrangements only apply when they name exactly the
        // panels that exist now; anything else falls back to the
        // alphabetical default.
        const layout = loadPanelLayout(groups.map((g) => g.prefix));
        const ordered = layout
            ? layout.order.map(
                  (name) => groups.find((g) => g.prefix === name)!,
              )
            : groups;
        tilesEl.innerHTML = "";
        sensorRows = [];
        for (const group of ordered) {
            tilesEl.appendChild(renderTile(group.prefix, group.bindings, sim.plant, sensorRows));
        }
        if (layout) {
            const minimized = new Set(layout.minimized);
            for (const el of Array.from(tilesEl.children) as HTMLElement[]) {
                if (minimized.has(el.dataset.panel ?? "")) {
                    applyMinimized(el, true);
                }
            }
        }
    };
    rebuildTiles();
    tilesEl.addEventListener("panel-layout-changed", () =>
        persistTileLayout(tilesEl),
    );

    // Alarm panel: fetches the "alarm"-kind sensor and renders
    // structured rows; click a row to acknowledge it.
    let alarmPanel: AlarmPanel | null = null;
    function initAlarms(): void {
        alarmsEl.innerHTML = "";
        const raw = sim.plant.sensorJSON(ALARM_SENSOR_ID);
        const initial: AlarmEntry[] =
            typeof raw === "string"
                ? (JSON.parse(raw) as AlarmEntry[])
                : [];
        alarmPanel = renderAlarms(alarmsEl, sim.plant, initial);
    }
    initAlarms();

    // Command console: submit nsimrun-style command lines that
    // fire relative to the simulation clock.
    let consolePanel: ConsolePanel | null = null;
    function initConsolePanel(): void {
        consolePanel = initConsole(consoleEl, sim.plant);
    }
    initConsolePanel();

    copyBtn.title = "Copy plant state JSON to clipboard";
    copyBtn.addEventListener("click", async () => {
        if (remoteClient) {
            try {
                const state = await remoteClient.state();
                const ok = await copyText(JSON.stringify(state));
                copyBtn.textContent = ok ? "✓" : "!";
            } catch {
                copyBtn.textContent = "!";
            }
            setTimeout(() => { copyBtn.textContent = "📋"; }, COPY_FEEDBACK_MS);
            return;
        }
        const json = sim.plant.toJSON();
        const ok = await copyText(json);
        copyBtn.textContent = ok ? "✓" : "!";
        setTimeout(() => { copyBtn.textContent = "📋"; }, COPY_FEEDBACK_MS);
    });

    sim.attachUI({
        startBtn,
        stepBtn,
        resetBtn,
        fpsInput,
        spsInput,
        status: statusEl,
        pidEl,
        onReset: () => {
            lastSvgHtml = mountSvg(pidEl, sim.plant);
            rebuildTiles();
            initAlarms();
            initConsolePanel();
        },
    });

    // Scenario selection: the catalog ships inside the wasm binary.
    // The picker opens over the paused default dashboard at startup
    // and again from the header button; confirming a choice swaps in
    // the scenario's starting plant through the same reset path the
    // Reset button uses.
    let scenarios: ScenarioInfo[] = [];
    try {
        scenarios = loadScenarios();
    } catch (err) {
        console.error("scenario catalog unavailable:", err);
        scenarioBtn.disabled = true;
    }
    function applyScenarioChoice(id: string | null): void {
        if (remoteClient) {
            if (!remoteWritable) return;
            if (id === null) return; // no free play remotely
            void remoteClient.reset({ scenario: id }).then(() => {
                lastRemoteRunId = -1;
            }).catch((err) => {
                console.error("reset failed:", err);
            });
            const title = scenarios.find((s) => s.id === id)?.title ?? "";
            scenarioNameEl.textContent = title;
            return;
        }
        // Switching lineups mid-run would conflate two exercises, so
        // the switch pauses the clock; Start resumes the new scenario.
        sim.running = false;
        currentScenarioId = id ?? "";
        const title = id
            ? scenarios.find((s) => s.id === id)?.title ?? ""
            : "";
        scenarioNameEl.textContent = title;
        sim.reset();
    }
    function openScenarioPicker(): void {
        if (remoteClient) {
            // Remote: fetch the server catalog, hide free play.
            void remoteClient.scenarios().then((catalog) => {
                scenarios = catalog;
                renderScenarioPicker(document.body, scenarios, {
                    onStart: applyScenarioChoice,
                });
            });
            return;
        }
        renderScenarioPicker(document.body, scenarios, {
            onStart: applyScenarioChoice,
        });
    }
    scenarioBtn.addEventListener("click", openScenarioPicker);
    if (scenarios.length > 0) {
        openScenarioPicker();
    }

    // Live objective banner: polls the wasm-side end-state bounds so
    // the operator sees which bounds are still unmet. An identical
    // payload from the previous frame skips parse and DOM work.
    const objectiveEl = document.getElementById("objective") as HTMLElement;
    let lastObjectiveRaw = "";
    function refreshObjective(): void {
        const raw = sim.plant.objectiveJSON();
        if (typeof raw !== "string" || raw === lastObjectiveRaw) {
            return;
        }
        lastObjectiveRaw = raw;
        const status = JSON.parse(raw) as ObjectiveStatus;
        if (!status.active) {
            objectiveEl.hidden = true;
            return;
        }
        const objectiveText = scenarios.find(
            (s) => s.id === status.id,
        )?.objective ?? "";
        objectiveEl.hidden = false;
        objectiveEl.classList.toggle("met", status.done);
        const titleEl = objectiveEl.querySelector(".objective-title");
        const statusEl2 = objectiveEl.querySelector(".objective-status");
        if (titleEl) {
            titleEl.textContent = `${status.title}: ${objectiveText}`;
        }
        if (statusEl2) {
            statusEl2.textContent = objectiveStatusText(
                status.done, status.detail, status.metMs);
            let again = objectiveEl.querySelector(".objective-again");
            if (status.done && !again) {
                again = document.createElement("button");
                again.className = "objective-again";
                again.textContent = "Choose next";
                again.addEventListener("click", openScenarioPicker);
                objectiveEl.appendChild(again);
            } else if (!status.done && again) {
                again.remove();
            }
        }
    }

    let lastSvg = performance.now();
    let lastUiRefreshMs = 0;
    // Wall-clock stepping accumulator: elapsed real time converts
    // to Steps/s debt, executed in whole steps each frame.
    let lastStepWallMs = performance.now();
    let stepDebt = 0;
    // Throughput window state; updated by updateStepRate below.
    let windowStartMs = performance.now();
    let spsWindowSteps = 0;
    // Sim time at the last read-out refresh; plant state only
    // changes when the clock advances, so paused idle frames skip
    // the refresh path entirely.
    let lastRefreshedMs = -1;
    let lastTickError = "";
    function tick(): void {
        // Reschedule first: an exception in the body must never
        // kill the render loop permanently.
        requestAnimationFrame(tick);
        if (remoteConnected) {
            // Remote owns the clock; don't step the local plant
            // or overwrite the remote status line.
            return;
        }
        try {
            const nowMs = performance.now();
            if (sim.running) {
                const elapsedS = Math.min(
                    (nowMs - lastStepWallMs) / 1000, MAX_CATCHUP_S);
                stepDebt += elapsedS * sim.stepsPerSecond;
                const dueSteps = Math.floor(stepDebt);
                if (dueSteps > 0) {
                    sim.plant.step(dueSteps);
                    spsWindowSteps += dueSteps;
                    stepDebt -= dueSteps;
                }
            }
            lastStepWallMs = nowMs;
            const simMs = sim.plant.simTimeMs();
            // Paused single-steps bypass the throttle so the
            // read-out reacts immediately; while running the
            // panels refresh at most once per frame-interval
            // regardless of how many steps each frame executes.
            const uiDue = nowMs - lastUiRefreshMs >=
                1000 / sim.framesPerSecond;
            if (simMs !== lastRefreshedMs && (uiDue || !sim.running)) {
                lastUiRefreshMs = nowMs;
                lastRefreshedMs = simMs;
                const snap = refreshSensors(sensorRows, sim.plant);
                // Refresh the command console list from the wasm queue.
                consolePanel?.refresh();
                // Refresh the alarm panel from the same batched
                // snapshot; no separate crossing for Plant/Alarms.
                const alarmJson = snap?.values[ALARM_SENSOR_ID];
                if (alarmJson !== undefined) {
                    alarmPanel?.refresh(alarmJson);
                }
                refreshObjective();
                if (performance.now() - lastSvg > 1000) {
                    const svg = sim.plant.getSvg();
                    // Swap only when the diagram actually changed so
                    // idle periods do not re-parse an identical DOM.
                    if (typeof svg === "string" && svg !== lastSvgHtml) {
                        mountPlant(pidEl, svg);
                        lastSvgHtml = svg;
                    }
                    lastSvg = performance.now();
                }
                lastTickError = "";
            }
            sim.refreshStatus();
            updateStepRate(nowMs);
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            if (msg !== lastTickError) {
                lastTickError = msg;
                console.error("tick failed:", err);
            }
        }
    }
    requestAnimationFrame(tick);

    // updateStepRate reports actual plant steps per wall-clock
    // second against the requested rate (ticks per frame at the
    // nominal frame cadence), refreshed once per second. A large
    // gap means the browser cannot finish the requested ticks in
    // a frame budget and the simulation falls behind real time.
    function updateStepRate(nowMs: number): void {
        if (!sim.running) {
            spsEl.textContent = "steps/s: —";
            spsWindowSteps = 0;
            windowStartMs = nowMs;
            return;
        }
        const elapsedMs = nowMs - windowStartMs;
        if (elapsedMs < SPS_WINDOW_MS) {
            return;
        }
        const actualSps = Math.round(spsWindowSteps / (elapsedMs / 1000));
        spsEl.textContent = `steps/s: ${actualSps} / ${sim.stepsPerSecond}`;
        spsWindowSteps = 0;
        windowStartMs = nowMs;
    }

    // ---- Remote mode ----
    // Remote-only UI is visible/active only while connected.
    // In local mode none of it renders; disconnect restores
    // local exactly. The server is already shipped; this is
    // the client half.
    let remoteClient: RemoteClient | null = null;
    let remoteConnected = false;
    let remoteWritable = false;
    let remotePollTimer: ReturnType<typeof setTimeout> | null = null;
    let remoteLastStatus: StatusView | null = null;
    let remoteLastSeq = -1;
    let remoteOwnSeq = -1;
    let lastRemoteRunId = -1;
    let remoteFailures = 0;
    let remoteSensorRows: SensorRow[] = [];
    let logsPanel: LogsPanel | null = null;
    let remoteAlarmPanel: { refresh(json: string): void } | null = null;

    const REMOTE_POLL_MS = 1000;

    function setRemoteVisible(visible: boolean): void {
        serverBtn.style.display = visible ? "" : "none";
        writeModeBtn.style.display = visible && remoteConnected ? "" : "none";
        logsEl.style.display = visible ? "" : "none";
        // hidden attribute keeps them out of tab order when
        // not connected.
        serverBtn.toggleAttribute("hidden", !visible);
        writeModeBtn.toggleAttribute("hidden", !(visible && remoteConnected));
        logsEl.toggleAttribute("hidden", !visible);
    }

    function applyWritable(): void {
        writeModeBtn.textContent = remoteWritable ? "✏️ Writable" : "🔒 Read-only";
        writeModeBtn.title = remoteWritable
            ? "Writable — control changes ask for a comment"
            : "Read-only — enable to change controls";
        logsPanel?.setWritable(remoteWritable);
        setWriteDisabled(!remoteWritable);
    }

    function setWriteDisabled(disabled: boolean): void {
        // Direct DOM toggle, no rebuild: control widgets only.
        const nodes = Array.from(tilesEl.querySelectorAll<HTMLElement>(
            "[data-bridge-id] input, [data-bridge-id] select, [data-bridge-id] button"));
        for (const el of nodes) {
            if (el.closest(".ctl-row")) {
                (el as HTMLInputElement).disabled = disabled;
            }
        }
        startBtn.disabled = disabled;
        stepBtn.disabled = disabled || !remoteLastStatus?.paused;
        resetBtn.disabled = disabled;
        spsInput.disabled = disabled;
    }

    function connectRemote(base: string): void {
        remoteClient = new RemoteClient(base);
        remoteConnected = true;
        remoteWritable = false;
        lastRemoteRunId = -1;
        remoteLastSeq = -1;
        remoteOwnSeq = -1;
        remoteFailures = 0;
        remoteSensorRows = [];
        spsEl.textContent = "steps/s: —";
        scenarioNameEl.textContent = "";
        setRemoteVisible(true);
        startRemotePoll(base);
        applyWritable();
        statusEl.textContent = `connected: ${base}`;
    }

    function disconnectRemote(): void {
        remoteConnected = false;
        remoteWritable = false;
        remoteClient = null;
        remoteLastStatus = null;
        stopRemotePoll();
        logsPanel = null;
        setRemoteVisible(false);
        // Restore local mode exactly.
        lastSvgHtml = mountSvg(pidEl, sim.plant);
        rebuildTiles();
        initAlarms();
        initConsolePanel();
        spsEl.textContent = "steps/s: —";
    }

    if (serverBtn) {
        serverBtn.addEventListener("click", () => {
            const current = remoteConnected ? remoteClient?.base ?? null : null;
            renderServerPicker(document.body, current, {
                onConnected(base) {
                    if (remoteConnected) disconnectRemote();
                    connectRemote(base);
                },
                onDisconnected() {
                    disconnectRemote();
                },
            });
        });
        writeModeBtn.addEventListener("click", () => {
            if (!remoteConnected) return;
            remoteWritable = !remoteWritable;
            applyWritable();
        });
    }

    // Header lifecycle: capture handlers run before Sim's
    // bubble listeners (attached by attachUI), so stopping
    // propagation here prevents double-acting in remote mode.
    startBtn.addEventListener("click", async (ev) => {
        if (remoteClient && remoteConnected) {
            ev.stopImmediatePropagation();
            ev.preventDefault();
            if (!remoteWritable) return;
            try {
                if (remoteLastStatus?.paused) await remoteClient.resume();
                else await remoteClient.pause();
            } catch (err) {
                console.error("pause/resume failed:", err);
            }
        }
    }, { capture: true });
    stepBtn.addEventListener("click", async (ev) => {
        if (remoteClient && remoteConnected) {
            ev.stopImmediatePropagation();
            ev.preventDefault();
            if (!remoteWritable) return;
            if (remoteLastStatus && !remoteLastStatus.paused) return;
            try { await remoteClient.step(); } catch (err) { console.error(err); }
        }
    }, { capture: true });
    resetBtn.addEventListener("click", async (ev) => {
        if (remoteClient && remoteConnected) {
            ev.stopImmediatePropagation();
            ev.preventDefault();
            if (!remoteWritable) return;
            try {
                await remoteClient.reset({});
                lastRemoteRunId = -1;
            } catch (err) { console.error(err); }
        }
    }, { capture: true });
    spsInput.addEventListener("change", async (ev) => {
        if (remoteClient && remoteConnected) {
            ev.stopImmediatePropagation();
            if (!remoteWritable) return;
            const n = parseInt(spsInput.value, 10);
            if (Number.isNaN(n) || n <= 0) return;
            try { await remoteClient.speed(n); } catch (err) { console.error(err); }
        }
    }, { capture: true });



    // Scenario picker in remote mode: Reset button already
    // remapped; the picker is remapped in applyScenarioChoice.

    let remotePollGen = 0;

    function startRemotePoll(_base: string): void {
        stopRemotePoll();
        const gen = ++remotePollGen;
        const tickPoll = async (): Promise<void> => {
            if (gen !== remotePollGen || !remoteConnected) return;
            remotePollTimer = setTimeout(() => { void tickPoll(); }, REMOTE_POLL_MS);
            if (!remoteClient || !remoteConnected) return;
            try {
                await pollOnce();
                remoteFailures = 0;
            } catch (err) {
                remoteFailures++;
                const msg = err instanceof RemoteError ? err.message : String(err);
                statusEl.textContent = `unreachable — retrying… (${msg})`;
                if (remoteFailures === 3 && remoteClient) {
                    // One re-probe distinguishes server-down
                    // from one slow request.
                    const alive = await remoteClient.livez();
                    if (!alive) {
                        statusEl.textContent = "server unreachable — retrying…";
                    }
                }
            }
        };
        void tickPoll();
    }

    function stopRemotePoll(): void {
        remotePollGen++;
        if (remotePollTimer !== null) {
            clearTimeout(remotePollTimer);
            remotePollTimer = null;
        }
    }

    let remotePollInFlight = false;

    async function pollOnce(): Promise<void> {
        if (!remoteClient || remotePollInFlight) return;
        remotePollInFlight = true;
        const client = remoteClient;
        try {
            const status = await client.status();
            if (remoteClient !== client) return;
        const prev = remoteLastStatus;
        remoteLastStatus = status;

        // Header.
        const sec = (status.simTimeMs / 1000).toFixed(2);
        const state = status.paused ? "paused" : "running";
        statusEl.textContent = `${state} @ ${sec} s simulated — ${status.siteName}`;
        startBtn.textContent = status.paused ? "▶ Start" : "⏸ Pause";
        startBtn.disabled = !remoteWritable;
        stepBtn.disabled = !status.paused || !remoteWritable;
        resetBtn.disabled = !remoteWritable;
        spsInput.disabled = !remoteWritable;

        // runId rebuild: build tiles once, then feed them.
        if (status.runId !== lastRemoteRunId) {
            lastRemoteRunId = status.runId;
            remoteLastSeq = -1;
            remoteOwnSeq = -1;
            logsEl.innerHTML = "";
            logsPanel = initLogs(logsEl, client);
            logsPanel.setWritable(remoteWritable);
            await buildRemoteTiles();
            if (remoteClient !== client) return;
            // Full log history on (re)connect / runId change.
            // Cursor = max(latestSeq, last appended seq) so
            // entries landing mid-fetch aren't skipped.
            const all = await client.logs(0);
            if (remoteClient !== client) return;
            logsPanel.append(all);
            const last = all.length ? all[all.length - 1].seq : status.latestSeq;
            remoteLastSeq = Math.max(status.latestSeq, last);
        }

        // Ticks: cheap value refresh, never a full rebuild.
        const timeMoved = prev === null || status.simTimeMs !== prev.simTimeMs;
        const [controls, sensors, alarms] = await Promise.all([
            client.controls().catch(() => null),
            client.sensors().catch(() => null),
            client.alarms().catch(() => null),
        ]);
        if (remoteClient !== client) return;
        if (controls !== null) {
            const values: Record<string, string> = {};
            for (const c of controls) values[c.key] = c.value;
            refreshRemoteControls(tilesEl, values);
        }
        if (sensors !== null) {
            const values: Record<string, string> = {};
            for (const s of sensors) values[s.key] = s.value;
            refreshRemoteSensors(remoteSensorRows, values);
        }
        if (alarms !== null && remoteAlarmPanel) {
            remoteAlarmPanel.refresh(JSON.stringify(alarms));
        }
        void timeMoved;
        await pollLogs(client, status);
        } finally {
            remotePollInFlight = false;
        }
    }

    async function pollLogs(client: RemoteClient, status: StatusView): Promise<void> {
        if (!logsPanel) return;
        if (status.latestSeq <= remoteLastSeq) return;
        const entries = await client.logs(remoteLastSeq + 1);
        if (remoteClient !== client) return;
        if (entries.length === 0) {
            remoteLastSeq = status.latestSeq;
            return;
        }
        // Foreign = entries above what we've seen *and* what
        // this browser caused (its own writes returned a seq).
        // Scheduling ops the server logged for others also
        // count as foreign once they appear.
        const threshold = Math.max(remoteLastSeq, remoteOwnSeq);
        const foreign = entries.filter((e) => e.seq > threshold);
        logsPanel.append(entries);
        if (foreign.length > 0) {
            logsPanel.noteForeign(foreign.length, foreign[foreign.length - 1]);
        }
        const newest = entries[entries.length - 1].seq;
        remoteLastSeq = Math.max(status.latestSeq, newest);
        remoteOwnSeq = Math.max(remoteOwnSeq, newest);
    }

    async function buildRemoteTiles(): Promise<void> {
        if (!remoteClient) return;
        const client = remoteClient;
        const [controls, sensors] = await Promise.all([
            client.controls().catch(() => [] as ControlEntry[]),
            client.sensors().catch(() => [] as SensorEntry[]),
        ]);
        if (!remoteClient) return;
        const bindings = buildRemoteBindings(controls, sensors);
        const groups = groupByPrefix(bindings);
        const layout = loadPanelLayout(groups.map((g) => g.prefix));
        const ordered = layout
            ? layout.order.map((name) => groups.find((g) => g.prefix === name)!).filter(Boolean)
            : groups;
        tilesEl.innerHTML = "";
        remoteSensorRows = [];
        const controlValues: Record<string, string> = {};
        for (const c of controls) controlValues[c.key] = c.value;
        for (const group of ordered as ReturnType<typeof groupByPrefix>) {
            const { root, body: tileBody } = makeTile(group.prefix || "(other)");
            const ctlCol = document.createElement("div");
            const ctlH = document.createElement("h3");
            ctlH.textContent = "Controls";
            ctlCol.appendChild(ctlH);
            const ctlHost = document.createElement("div");
            ctlCol.appendChild(ctlHost);
            renderRemotePanel(
                ctlHost,
                group.bindings,
                displayPart(group.prefix),
                controlValues,
                () => remoteWritable,
                async (key, value, comment) => {
                    const res = await client.putControl(key, value, comment);
                    remoteOwnSeq = Math.max(remoteOwnSeq, res.seq);
                    logsPanel?.onOwnSeq(res.seq);
                    return { seq: res.seq };
                },
            );
            const senCol = document.createElement("div");
            const senH = document.createElement("h3");
            senH.textContent = "Sensors";
            senCol.appendChild(senH);
            const senHost = document.createElement("div");
            senCol.appendChild(senHost);
            remoteSensorRows.push(
                ...renderRemoteReadout(senHost, group.bindings, displayPart(group.prefix)),
            );
            tileBody.appendChild(ctlCol);
            tileBody.appendChild(senCol);
            tilesEl.appendChild(root);
        }
        if (layout) {
            const minimized = new Set(layout.minimized);
            for (const el of Array.from(tilesEl.children) as HTMLElement[]) {
                if (minimized.has(el.dataset.panel ?? "")) {
                    applyMinimized(el, true);
                }
            }
        }
        // Seed sensor values immediately.
        const sensorValues: Record<string, string> = {};
        for (const s of sensors) sensorValues[s.key] = s.value;
        refreshRemoteSensors(remoteSensorRows, sensorValues);
        refreshRemoteControls(tilesEl, controlValues);
        // Alarm panel in remote mode reads GET /alarms.
        // remote.ts already maps the core.Alarm wire shape
        // into the panel shape (severity words, time_s).
        alarmsEl.innerHTML = "";
        const alarms = await client.alarms().catch(() => [] as AlarmEntryRemote[]);
        const shim = {
            sensorJSON: (_id: string) => JSON.stringify(alarms),
            setControl: (id: string, value: string): Error | null => {
                if (!remoteWritable) return new Error("read-only");
                // Same confirm+comment flow as control tiles.
                void (async () => {
                    const dialog = await confirmControlChange(id, "(current)", value);
                    if (!dialog.submitted || dialog.comment === undefined) return;
                    const res = await client.putControl(id, value, dialog.comment);
                    remoteOwnSeq = Math.max(remoteOwnSeq, res.seq);
                    logsPanel?.onOwnSeq(res.seq);
                })().catch((err) => console.error(err));
                return null;
            },
        };
        remoteAlarmPanel = renderAlarms(alarmsEl, shim as never, alarms);

        // Remote console: re-initialize the console section
        // to talk to the server instead of the local plant.
        consoleEl.innerHTML = "";
        const consoleSubmit = document.createElement("button");
        consoleSubmit.className = "btn";
        consoleSubmit.textContent = "Submit";
        consoleSubmit.id = "btn-cmd-submit";
        const consoleText = document.createElement("textarea");
        consoleText.id = "cmd-input";
        consoleText.rows = 3;
        consoleText.spellcheck = false;
        consoleText.placeholder = "TimeOffsetMs Part.Label Value  (one per line)";
        const consoleList = document.createElement("div");
        consoleList.id = "cmd-list";
        const consoleError = document.createElement("div");
        consoleError.id = "cmd-error";
        consoleEl.append(consoleText, consoleSubmit, consoleList, consoleError);
        consoleSubmit.disabled = !remoteWritable;
        consoleSubmit.addEventListener("click", () => {
            const text = consoleText.value.trim();
            if (!text) return;
            void client.submitCommandsText(text).then(() => {
                consoleText.value = "";
                consoleError.textContent = "";
            }).catch((err) => {
                consoleError.textContent = err instanceof RemoteError ? err.message : String(err);
            });
        });
    }
}

main().catch((err) => {
    console.error("nsim web init failed:", err);
    const statusEl = document.getElementById("sim-status");
    if (statusEl) {
        statusEl.textContent = `init failed: ${err}`;
    }
});

// persistTileLayout snapshots the current grid order and
// minimized flags into localStorage; wired to the change events
// emitted by the swap gesture and the minimize toggles.
function persistTileLayout(tilesEl: HTMLElement): void {
    const tiles = Array.from(
        tilesEl.querySelectorAll<HTMLElement>(":scope > .tile[data-panel]"),
    );
    savePanelLayout({
        version: PANEL_LAYOUT_VERSION,
        order: tiles.map((t) => t.dataset.panel ?? ""),
        minimized: tiles
            .filter((t) => t.classList.contains("minimized"))
            .map((t) => t.dataset.panel ?? ""),
    });
}

// mountSvg renders the current diagram into the mimic host or
// throws: a failed getSvg leaves the diagram silently stale, which
// is worse than surfacing the error.
function mountSvg(pidEl: HTMLElement, plant: PlantHandle): string {
    const svg = plant.getSvg();
    if (typeof svg !== "string") {
        throw new Error(`failed to render svg: ${(svg as Error).message}`);
    }
    mountPlant(pidEl, svg);
    return svg;
}

// renderTile assembles one tile (one top-level prefix) with a
// Controls column and a Sensors column. New rows for the global
// refresh loop are pushed into the shared sensorRows accumulator.
function renderTile(
    prefix: string,
    bindings: PartBinding[],
    plant: PlantHandle,
    sensorRows: SensorRow[],
): HTMLElement {
    const { root: tile, body } = makeTile(prefix || "(other)");

    const ctlCol = document.createElement("div");
    const ctlH = document.createElement("h3");
    ctlH.textContent = "Controls";
    ctlCol.appendChild(ctlH);
    const ctlHost = document.createElement("div");
    ctlCol.appendChild(ctlHost);
    renderPanel(ctlHost, bindings, plant, displayPart(prefix));

    const senCol = document.createElement("div");
    const senH = document.createElement("h3");
    senH.textContent = "Sensors";
    senCol.appendChild(senH);
    const senHost = document.createElement("div");
    senCol.appendChild(senHost);
    sensorRows.push(...renderReadout(senHost, bindings, plant, displayPart(prefix)));

    body.appendChild(ctlCol);
    body.appendChild(senCol);
    return tile;
}
