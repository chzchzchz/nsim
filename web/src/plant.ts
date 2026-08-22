// plant.ts — TypeScript wrapper around the Go wasm handle.
// The handle is the object returned by `nsimNewPlant()`, a
// bag of methods implemented in web/wasm/main.go via
// syscall/js. The TypeScript side treats it as opaque except
// for the methods declared below.

export interface BridgeMeta {
    part: string;
    label: string;
    kind: string;
}

export interface PlantHandle {
    step(ticks: number): void;
    controlKeys(): string[];
    sensorKeys(): string[];
    getMeta(id: string): BridgeMeta | Error;
    setControl(id: string, value: string): Error | null;
    sensorJSON(id: string): string | Error;
    getSvg(): string | Error;
    simTimeMs(): number;
}

// nsimNewPlant is the only top-level wasm export. It returns a
// fresh PlantHandle wrapping a *core.Plant constructed by
// core.MakePlant().
declare global {
    interface Window {
        nsimNewPlant: () => PlantHandle;
    }
}

export function newPlant(): PlantHandle {
    if (typeof window === "undefined" || !window.nsimNewPlant) {
        throw new Error("nsim wasm not loaded; call loadWasm() first");
    }
    return window.nsimNewPlant();
}
