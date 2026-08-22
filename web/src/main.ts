// main.ts — bootstrap, tick loop, and panel wiring.

import { loadWasm } from "./wasm";
import { newPlant } from "./plant";
import { mountPlant } from "./svg";
import { renderPanel } from "./controls";
import { renderReadout, refreshSensors, type SensorRow } from "./sensors";
import { buildBindings, displayPart, groupByPrefix, type PartBinding } from "./bindings";
import { Sim } from "./sim";

declare global {
    interface Window {
        Go: any;
    }
}

async function main(): Promise<void> {
    // Copy wasm_exec.js polyfill into the same origin. We
    // loaded it from /wasm_exec.js via the <script> tag in
    // index.html so it lives on window.Go.

    await loadWasm();

    const pidEl = document.getElementById("pid") as HTMLElement;
    const tilesEl = document.getElementById("tiles") as HTMLElement;
    const startBtn = document.getElementById("btn-start") as HTMLButtonElement;
    const stepBtn = document.getElementById("btn-step") as HTMLButtonElement;
    const resetBtn = document.getElementById("btn-reset") as HTMLButtonElement;
    const tickRateSel = document.getElementById("tick-rate") as HTMLSelectElement;
    const statusEl = document.getElementById("sim-status") as HTMLElement;

    const sim = new Sim(newPlant);

    // Initial SVG mount + tiles. Each top-level prefix gets its
    // own tile; the tile splits into Controls | Sensors columns.
    const initialSvg = sim.plant.getSvg();
    if (typeof initialSvg !== "string") {
        throw new Error(`failed to render initial svg: ${(initialSvg as Error).message}`);
    }
    mountPlant(pidEl, initialSvg);

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
            rebuildTiles();
        },
    });

    let lastSvg = performance.now();
    function tick(): void {
        if (sim.running) {
            sim.plant.step(sim.ticksPerFrame);
        }
        refreshSensors(sensorRows, sim.plant);
        if (performance.now() - lastSvg > 1000) {
            const svg = sim.plant.getSvg();
            if (typeof svg === "string") {
                pidEl.innerHTML = svg;
            }
            lastSvg = performance.now();
        }
        sim.refreshStatus();
        requestAnimationFrame(tick);
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
    const tile = document.createElement("div");
    tile.className = "tile";
    const h = document.createElement("h2");
    h.textContent = prefix || "(other)";
    tile.appendChild(h);

    const body = document.createElement("div");
    body.className = "tile-body";
    tile.appendChild(body);

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
