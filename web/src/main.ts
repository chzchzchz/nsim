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
import { applyMinimized } from "./tile";
import { buildTile } from "./tile";
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
import { RemoteError } from "./remote";
import { renderServerPicker } from "./serverPicker";
import { RemoteController } from "./remoteController";

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

    // Remote controller is constructed before any scenario wiring
    // runs so synchronously-invoked picker code never hits a TDZ.
    // (Previously `let remote*` lived far below while the auto-open
    // picker read it at startup; see remoteController.ts header.)
    const remote = new RemoteController(
        {
            tilesEl,
            alarmsEl,
            consoleEl,
            logsEl,
            statusEl,
            startBtn,
            stepBtn,
            resetBtn,
            spsInput,
            spsEl,
            writeModeBtn,
            pidEl,
        },
        {
            scenarioNameEl,
            onDisconnected: () => {
                lastSvgHtml = mountSvg(pidEl, sim.plant);
                rebuildTiles();
                initAlarms();
                initConsolePanel();
            },
        },
    );

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
            buildTile(
                tilesEl,
                group.prefix,
                (ctlHost) => renderPanel(ctlHost, group.bindings, sim.plant, displayPart(group.prefix)),
                (senHost) => renderReadout(senHost, group.bindings, sim.plant, displayPart(group.prefix)),
                sensorRows,
            );
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
        if (remote.client) {
            try {
                const state = await remote.client.state();
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
        if (remote.client) {
            if (!remote.writable) return;
            if (id === null) return; // no free play remotely
            // Set the title only after the server confirms the
            // reset; otherwise a failed reset would show a
            // scenario that is not running. Surface failures in
            // the status line instead of failing silently.
            void remote.applyScenario(id).then((ok) => {
                if (!ok) {
                    // Append rather than replace: statusEl is the
                    // live sim-status line (paused/running), and
                    // overwriting it with a failure note would
                    // destroy the connection context.
                    statusEl.textContent +=
                        " (scenario reset failed — see console)";
                    return;
                }
                const title =
                    scenarios.find((s) => s.id === id)?.title ?? "";
                scenarioNameEl.textContent = title;
            });
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
    // openServerPicker is shared by the header button and the
    // scenario dialog's footer entry point. The server dialog has
    // its own overlay, so scenario list -> server pick never stacks.
    function openServerPicker(): void {
        const current = remote.connected ? remote.client?.base ?? null : null;
        renderServerPicker(document.body, current, {
            onConnected(base) {
                if (remote.connected) remote.disconnect();
                remote.connect(base);
            },
            onDisconnected() {
                remote.disconnect();
            },
        });
    }
    function openScenarioPicker(): void {
        if (remote.client) {
            // Remote: fetch the server catalog, hide free play.
            // A catalog failure must not silently swallow the
            // click; surface it in the status line.
            const client = remote.client;
            void client
                .scenarios()
                .then((catalog) => {
                    scenarios = catalog;
                    renderScenarioPicker(document.body, scenarios, {
                        onStart: applyScenarioChoice,
                        onServer: openServerPicker,
                    });
                })
                .catch((err: unknown) => {
                    const msg =
                        err instanceof Error ? err.message : String(err);
                    statusEl.textContent = `scenario catalog failed: ${msg}`;
                });
            return;
        }
        renderScenarioPicker(document.body, scenarios, {
            onStart: applyScenarioChoice,
            onServer: openServerPicker,
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
        if (remote.connected) {
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

    // ---- Remote helpers (thin glue; state lives in RemoteController) ----

    if (serverBtn) {
        serverBtn.addEventListener("click", openServerPicker);
        writeModeBtn.addEventListener("click", () => {
            remote.toggleWritable();
        });
    }

    // Header lifecycle: capture handlers run before Sim's
    // bubble listeners (attached by attachUI), so stopping
    // propagation here prevents double-acting in remote mode.
    startBtn.addEventListener("click", async (ev) => {
        if (remote.client && remote.connected) {
            ev.stopImmediatePropagation();
            ev.preventDefault();
            if (!remote.writable) return;
            await remote.togglePauseResume();
        }
    }, { capture: true });
    stepBtn.addEventListener("click", async (ev) => {
        if (remote.client && remote.connected) {
            ev.stopImmediatePropagation();
            ev.preventDefault();
            if (!remote.writable) return;
            await remote.step();
        }
    }, { capture: true });
    resetBtn.addEventListener("click", async (ev) => {
        if (remote.client && remote.connected) {
            ev.stopImmediatePropagation();
            ev.preventDefault();
            if (!remote.writable) return;
            await remote.reset();
        }
    }, { capture: true });
    spsInput.addEventListener("change", async (ev) => {
        if (remote.client && remote.connected) {
            ev.stopImmediatePropagation();
            if (!remote.writable) return;
            const n = parseInt(spsInput.value, 10);
            if (Number.isNaN(n) || n <= 0) return;
            await remote.setSpeed(n);
        }
    }, { capture: true });

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
// is worse than surfacing the error. TRUST: the SVG comes only from
// the local wasm plant (never the remote server), so the
// innerHTML sink in pidViewport.ts is safe; remote SVG would need
// sanitization first (see sanitize.ts / pidViewport.ts).
function mountSvg(pidEl: HTMLElement, plant: PlantHandle): string {
    const svg = plant.getSvg();
    if (typeof svg !== "string") {
        throw new Error(`failed to render svg: ${(svg as Error).message}`);
    }
    mountPlant(pidEl, svg);
    return svg;
}


