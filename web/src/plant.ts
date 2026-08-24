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
    controlValuesJSON(): string | Error;
    sensorJSON(id: string): string | Error;
    sensorValuesJSON(): string | Error;
    submitCommands(text: string): Error | null;
    commandQueueRev(): number;
    commandQueueJSON(): string | Error;
    commandLogJSON(): string | Error;
    getSvg(): string | Error;
    simTimeMs(): number;
    toJSON(): string;
    // objectiveJSON snapshots the active scenario's end-state bounds:
    // {active,id,title,done,detail}.
    objectiveJSON(): string;
}

declare global {
    interface Window {
        Go: any;
        // nsimNewPlant is the only plant factory wasm export. The
        // optional scenario id selects a catalog starting state
        // (see scenarios.ts); omitting it yields the default
        // full-load plant.
        nsimNewPlant: (scenarioId?: string) => PlantHandle;
        // nsimScenarios returns the JSON snapshot of the operator
        // evolution catalog rendered by the selection screen.
        nsimScenarios: () => string;
    }
}

export function newPlant(scenarioId?: string): PlantHandle {
    if (typeof window === "undefined" || !window.nsimNewPlant) {
        throw new Error("nsim wasm not loaded; call loadWasm() first");
    }
    return window.nsimNewPlant(scenarioId);
}
