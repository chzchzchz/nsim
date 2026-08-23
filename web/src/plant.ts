// plant.ts — TypeScript wrapper around the Go wasm handle.
// The handle is the object returned by `nsimNewPlant()`, a
// bag of methods implemented in web/wasm/main.go via
// syscall/js. The TypeScript side treats it as opaque except
// for the methods declared below.

export interface BridgeMeta {
    part: string;
    label: string;
    kind: string;
    // enum is present only for Kind === "enum" controls that
    // populate the optional bridge.Control.Enum slice. The
    // array index of each label is the value the Go Set
    // callback receives (matches the iota order in the Go
    // backend).
    enum?: string[];
}

export interface PlantHandle {
    step(ticks: number): void;
    controlKeys(): string[];
    sensorKeys(): string[];
    getMeta(id: string): BridgeMeta | Error;
    setControl(id: string, value: string): Error | null;
    getControlValue(id: string): string | Error;
    sensorJSON(id: string): string | Error;
    sensorValuesJSON(): string | Error;
    submitCommands(text: string): Error | null;
    commandQueueRev(): number;
    commandQueueJSON(): string | Error;
    commandLogJSON(): string | Error;
    getSvg(): string | Error;
    simTimeMs(): number;
    toJSON(): string;
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
