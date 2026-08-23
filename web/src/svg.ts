// svg.ts — mount the SVG document produced by the wasm side.
// The diagram is stateless markup; part highlighting binds via
// data-part attributes, so remounting never loses handlers.

export function mountPlant(target: HTMLElement, svgText: string): void {
    target.innerHTML = svgText;
}
