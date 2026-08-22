// wasm.ts — load the wasm blob and the Go polyfill. After
// loadWasm() resolves, window.nsimNewPlant is available and
// newPlant() will succeed.

import "./wasm_exec.js";  // pulls in the Go runtime polyfill

export async function loadWasm(): Promise<void> {
    const go = new (window as any).Go();
    const resp = await fetch("/nsim.wasm");
    if (!resp.ok) {
        throw new Error(`failed to fetch nsim.wasm: ${resp.status}`);
    }
    const buf = await resp.arrayBuffer();
    const { instance } = await WebAssembly.instantiate(buf, go.importObject);
    await go.run(instance);
}
