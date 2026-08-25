// wasm.ts — load the wasm blob and the Go polyfill. After
// loadWasm() resolves, window.nsimNewPlant is available and
// newPlant() will succeed.

// Sole polyfill loader: bundling it keeps window.Go available
// under any deploy base, unlike a root-absolute script tag.
import "./wasm_exec.js";

export async function loadWasm(): Promise<void> {
    const go = new window.Go();
    // BASE_URL always ends with "/" and honors Vite's `base`
    // option, so the module resolves under any deploy path.
    const wasmUrl = import.meta.env.BASE_URL + "nsim.wasm";
    const { instance } = await instantiate(wasmUrl, go);
    // Fire-and-forget: Go's main() blocks on `select {}`, so go.run()
    // would never resolve. The export (window.nsimNewPlant) is
    // installed synchronously by js.Global().Set during the wasm
    // `run` export, before go.run() awaits its exit promise. A later
    // runtime crash must not vanish as an unhandled rejection.
    go.run(instance).catch((err: unknown) => {
        console.error("nsim wasm runtime exited:", err);
    });
}

// instantiate prefers streaming compilation (compile while the
// bytes download) and falls back to a buffered fetch for hosts
// that do not serve .wasm with the application/wasm MIME type.
async function instantiate(
    url: string,
    go: { importObject: WebAssembly.Imports },
): Promise<WebAssembly.WebAssemblyInstantiatedSource> {
    try {
        return await WebAssembly.instantiateStreaming(fetch(url), go.importObject);
    } catch {
        const resp = await fetch(url);
        if (!resp.ok) {
            throw new Error(`failed to fetch ${url}: ${resp.status}`);
        }
        const buf = await resp.arrayBuffer();
        return WebAssembly.instantiate(buf, go.importObject);
    }
}
