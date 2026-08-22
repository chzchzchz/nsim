// sim.ts — simulation state machine (start/pause/step/reset).
// Purely TypeScript-side; the wasm-side plant has no notion of
// pause, it only knows how to advance N ticks per call.

import type { PlantHandle } from "./plant";

export interface SimUI {
    startBtn: HTMLButtonElement;
    stepBtn: HTMLButtonElement;
    resetBtn: HTMLButtonElement;
    tickRate: HTMLSelectElement;
    status: HTMLElement;
    pidEl: HTMLElement;
    onReset: () => void;
}

export class Sim {
    plant: PlantHandle;
    running = false;
    ticksPerFrame = 16;
    private factory: () => PlantHandle;
    private ui?: SimUI;

    constructor(factory: () => PlantHandle) {
        this.factory = factory;
        this.plant = factory();
    }

    attachUI(ui: SimUI): void {
        this.ui = ui;
        ui.startBtn.addEventListener("click", () => this.toggle());
        ui.stepBtn.addEventListener("click", () => this.stepOnce());
        ui.resetBtn.addEventListener("click", () => this.reset());
        ui.tickRate.addEventListener("change", () => {
            this.ticksPerFrame = parseInt(ui.tickRate.value, 10) || 1;
        });
        this.refreshStatus();
        this.refreshButtons();
    }

    toggle(): void {
        this.running = !this.running;
        this.refreshStatus();
        this.refreshButtons();
    }

    stepOnce(): void {
        if (this.running) {
            return;
        }
        this.plant.step(this.ticksPerFrame);
    }

    reset(): void {
        this.plant = this.factory();
        this.ui?.onReset();
        this.refreshStatus();
        this.refreshButtons();
    }

    refreshStatus(): void {
        if (!this.ui) {
            return;
        }
        const simMs = this.plant.simTimeMs();
        const sec = (simMs / 1000).toFixed(2);
        this.ui.status.textContent = `${this.running ? "running" : "paused"} @ ${sec} s simulated`;
    }

    private refreshButtons(): void {
        if (!this.ui) {
            return;
        }
        this.ui.startBtn.textContent = this.running ? "⏸ Pause" : "▶ Start";
        (this.ui.stepBtn as HTMLButtonElement).disabled = this.running;
    }
}
