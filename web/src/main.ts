// main.ts — bootstrap, tick loop, and panel wiring.

import { loadWasm } from "./wasm";
import { newPlant } from "./plant";
import { loadDescriptions } from "./descriptions";
import { mountPlant } from "./svg";
import { renderPanel } from "./controls";
import { renderReadout, refreshSensors, type SensorRow } from "./sensors";
import { renderAlarms, type AlarmEntry, type AlarmPanel } from "./alarms";
import { initConsole, type ConsolePanel } from "./console";
import { buildBindings, displayPart, groupByPrefix, type PartBinding } from "./bindings";
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

const COPY_FEEDBACK_MS = 1500;
// Stepping runs on a wall-clock accumulator so the achieved rate
// tracks Steps/s regardless of frame jitter; after a hidden-tab
// gap at most this much sim time executes before the remainder is
// dropped (the simulation falls behind rather than freezing).
const MAX_CATCHUP_S = 1;
// Throughput accounting: the measured side counts real plant
// steps over a one-second window.
const SPS_WINDOW_MS = 1000;

async function main(): Promise<void> {
    // Copy wasm_exec.js polyfill into the same origin. We
    // loaded it from /wasm_exec.js via the <script> tag in
    // index.html so it lives on window.Go.

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

    // The factory closure reads the current selection at reset time,
    // so both the initial plant and every later Reset rebuild the
    // chosen scenario's starting state.
    let currentScenarioId = "";
    const sim = new Sim(() => newPlant(currentScenarioId));

    // Drag a tile by its prefix heading to swap it with a
    // neighbour; see grid.ts for the gesture thresholds.
    enablePanelSwaps(tilesEl);

    // Initial SVG mount + tiles. Each top-level prefix gets its
    // own tile; the tile splits into Controls | Sensors columns.
    const initialSvg = sim.plant.getSvg();
    if (typeof initialSvg !== "string") {
        throw new Error(`failed to render initial svg: ${(initialSvg as Error).message}`);
    }
    mountPlant(pidEl, initialSvg);
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
        const raw = sim.plant.sensorJSON("Plant/Alarms");
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
            const svg = sim.plant.getSvg();
            if (typeof svg !== "string") {
                throw new Error(`failed to render svg on reset: ${(svg as Error).message}`);
            }
            mountPlant(pidEl, svg);
            lastSvgHtml = svg;
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
        currentScenarioId = id ?? "";
        const title = id
            ? scenarios.find((s) => s.id === id)?.title ?? ""
            : "";
        scenarioNameEl.textContent = title;
        sim.reset();
    }
    function openScenarioPicker(): void {
        renderScenarioPicker(document.body, scenarios, {
            onStart: applyScenarioChoice,
        });
    }
    scenarioBtn.addEventListener("click", openScenarioPicker);
    if (scenarios.length > 0) {
        openScenarioPicker();
    }

    // Live objective banner: polls the wasm-side end-state bounds so
    // the operator sees which bounds are still unmet.
    const objectiveEl = document.getElementById("objective") as HTMLElement;
    function refreshObjective(): void {
        const raw = sim.plant.objectiveJSON();
        if (typeof raw !== "string") {
            return;
        }
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
            statusEl2.textContent = objectiveStatusText(status.done, status.detail);
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
                refreshSensors(sensorRows, sim.plant);
                // Refresh the command console list from the wasm queue.
                consolePanel?.refresh();
                // Refresh alarm panel from the structured sensor JSON.
                const alarmJson = sim.plant.sensorJSON("Plant/Alarms");
                if (typeof alarmJson === "string") {
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

// renderTile assembles one tile (one top-level prefix) with a
// Controls column and a Sensors column. New rows for the global
// refresh loop are pushed into the shared sensorRows accumulator.
function renderTile(
    prefix: string,
    bindings: PartBinding[],
    plant: import("./plant").PlantHandle,
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
