// main.ts — bootstrap, tick loop, and panel wiring.

import { loadWasm } from "./wasm";
import { newPlant } from "./plant";
import { mountPlant } from "./svg";
import { renderPanel } from "./controls";
import { renderReadout, refreshSensors, type SensorRow } from "./sensors";
import { renderAlarms, type AlarmEntry, type AlarmPanel } from "./alarms";
import { initConsole, type ConsolePanel } from "./console";
import { buildBindings, displayPart, groupByPrefix, type PartBinding } from "./bindings";
import { enablePanelSwaps } from "./grid";
import { makeTile } from "./tile";
import { copyText } from "./clipboard";
import { Sim } from "./sim";

const COPY_FEEDBACK_MS = 1500;

async function main(): Promise<void> {
    // Copy wasm_exec.js polyfill into the same origin. We
    // loaded it from /wasm_exec.js via the <script> tag in
    // index.html so it lives on window.Go.

    await loadWasm();

    const pidEl = document.getElementById("pid") as HTMLElement;
    const tilesEl = document.getElementById("tiles") as HTMLElement;
    const alarmsEl = document.getElementById("alarms") as HTMLElement;
    const consoleEl = document.getElementById("console") as HTMLElement;
    const startBtn = document.getElementById("btn-start") as HTMLButtonElement;
    const stepBtn = document.getElementById("btn-step") as HTMLButtonElement;
    const resetBtn = document.getElementById("btn-reset") as HTMLButtonElement;
    const tickRateSel = document.getElementById("tick-rate") as HTMLSelectElement;
    const copyBtn = document.getElementById("btn-copy") as HTMLButtonElement;
    const statusEl = document.getElementById("sim-status") as HTMLElement;

    const sim = new Sim(newPlant);

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
        tilesEl.innerHTML = "";
        sensorRows = [];
        for (const group of groupByPrefix(bindings)) {
            tilesEl.appendChild(renderTile(group.prefix, group.bindings, sim.plant, sensorRows));
        }
    };
    rebuildTiles();

    // Alarm panel: fetches the "alarm"-kind sensor and renders
    // structured rows; double-click a row to acknowledge it.
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
        tickRate: tickRateSel,
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

    let lastSvg = performance.now();
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
            if (sim.running) {
                sim.plant.step(sim.ticksPerFrame);
            }
            const simMs = sim.plant.simTimeMs();
            if (simMs !== lastRefreshedMs) {
                lastRefreshedMs = simMs;
                refreshSensors(sensorRows, sim.plant);
                // Refresh the command console list from the wasm queue.
                consolePanel?.refresh();
                // Refresh alarm panel from the structured sensor JSON.
                const alarmJson = sim.plant.sensorJSON("Plant/Alarms");
                if (typeof alarmJson === "string") {
                    alarmPanel?.refresh(alarmJson);
                }
                if (performance.now() - lastSvg > 1000) {
                    const svg = sim.plant.getSvg();
                    // Swap only when the diagram actually changed so
                    // idle periods do not re-parse an identical DOM.
                    if (typeof svg === "string" && svg !== lastSvgHtml) {
                        pidEl.innerHTML = svg;
                        lastSvgHtml = svg;
                    }
                    lastSvg = performance.now();
                }
                lastTickError = "";
            }
            sim.refreshStatus();
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            if (msg !== lastTickError) {
                lastTickError = msg;
                console.error("tick failed:", err);
            }
        }
    }
    requestAnimationFrame(tick);
}

main().catch((err) => {
    console.error("nsim web init failed:", err);
    const statusEl = document.getElementById("sim-status");
    if (statusEl) {
        statusEl.textContent = `init failed: ${err}`;
    }
});

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
