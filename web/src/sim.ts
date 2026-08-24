// sim.ts — simulation state machine (start/pause/step/reset).
// Purely TypeScript-side; the wasm-side plant has no notion of
// pause, it only knows how to advance N ticks per call.

import type { PlantHandle } from "./plant";

export interface SimUI {
    startBtn: HTMLButtonElement;
    stepBtn: HTMLButtonElement;
    resetBtn: HTMLButtonElement;
    fpsInput: HTMLInputElement;
    spsInput: HTMLInputElement;
    status: HTMLElement;
    pidEl: HTMLElement;
    onReset: () => void;
}

const FPS_MIN = 1;
const FPS_MAX = 240;
const SPS_MIN = 0;
const SPS_MAX = 1000000;

export class Sim {
    plant: PlantHandle;
    running = false;
    // Operator-entered targets: interface refresh cadence and
    // simulated milliseconds executed per wall-clock second. The
    // tick loop turns Steps/s into an exact step count via a
    // wall-clock accumulator, so frame jitter never skews speed.
    framesPerSecond = 60;
    stepsPerSecond = 1000;
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
        ui.fpsInput.addEventListener("change", () => {
            this.framesPerSecond = clampInt(
                ui.fpsInput, FPS_MIN, FPS_MAX, this.framesPerSecond,
            );
        });
        ui.spsInput.addEventListener("change", () => {
            this.stepsPerSecond = clampInt(
                ui.spsInput, SPS_MIN, SPS_MAX, this.stepsPerSecond,
            );
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
        // One press advances exactly one simulated millisecond.
        this.plant.step(1);
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

function clampInt(
    input: HTMLInputElement,
    min: number,
    max: number,
    fallback: number,
): number {
    const v = parseInt(input.value, 10);
    const clamped = Number.isNaN(v) ? fallback : Math.min(max, Math.max(min, v));
    input.value = String(clamped);
    return clamped;
}
