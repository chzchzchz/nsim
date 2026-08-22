// main.ts — bootstrap, tick loop, and panel wiring.

import { loadWasm } from "./wasm";
import { newPlant } from "./plant";
import { mountPlant } from "./svg";
import { renderPanel } from "./controls";
import { renderReadout, refreshSensors, type SensorRow } from "./sensors";
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
    const controlsEl = document.getElementById("controls") as HTMLElement;
    const readoutEl = document.getElementById("readout") as HTMLElement;
    const startBtn = document.getElementById("btn-start") as HTMLButtonElement;
    const stepBtn = document.getElementById("btn-step") as HTMLButtonElement;
    const resetBtn = document.getElementById("btn-reset") as HTMLButtonElement;
    const tickRateSel = document.getElementById("tick-rate") as HTMLSelectElement;
    const statusEl = document.getElementById("sim-status") as HTMLElement;

    const sim = new Sim(newPlant);

    // Initial SVG mount + panel + readout.
    const initialSvg = sim.plant.getSvg();
    if (typeof initialSvg !== "string") {
        throw new Error(`failed to render initial svg: ${(initialSvg as Error).message}`);
    }
    mountPlant(pidEl, initialSvg);
    renderPanel(controlsEl, sim.plant);
    let sensorRows = renderReadout(readoutEl, sim.plant);

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
            renderPanel(controlsEl, sim.plant);
            sensorRows = renderReadout(readoutEl, sim.plant);
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
