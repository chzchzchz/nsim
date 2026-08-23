// wasm.ts — load the wasm blob and the Go polyfill. After
// loadWasm() resolves, window.nsimNewPlant is available and
// newPlant() will succeed.

import "./wasm_exec.js";  // pulls in the Go runtime polyfill

export async function loadWasm(): Promise<void> {
    const go = new (window as any).Go();
    // BASE_URL always ends with "/" and honors Vite's `base`
    // option, so the module resolves under any deploy path.
    const wasmUrl = import.meta.env.BASE_URL + "nsim.wasm";
    const resp = await fetch(wasmUrl);
    if (!resp.ok) {
        throw new Error(`failed to fetch ${wasmUrl}: ${resp.status}`);
    }
    const buf = await resp.arrayBuffer();
    const { instance } = await WebAssembly.instantiate(buf, go.importObject);
    // Fire-and-forget: Go's main() blocks on `select {}`, so go.run()
    // would never resolve. The export (window.nsimNewPlant) is
    // installed synchronously by js.Global().Set during the wasm
    // `run` export, before go.run() awaits its exit promise. A later
    // runtime crash must not vanish as an unhandled rejection.
    go.run(instance).catch((err: unknown) => {
        console.error("nsim wasm runtime exited:", err);
    });
}
