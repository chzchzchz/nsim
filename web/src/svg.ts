// svg.ts — mount the SVG document produced by the wasm side
// and provide the lookup helpers the bindings module needs.

export function mountPlant(target: HTMLElement, svgText: string): void {
    target.innerHTML = svgText;
}

export function getPartElements(target: HTMLElement): Map<string, SVGGElement> {
    const map = new Map<string, SVGGElement>();
    for (const g of Array.from(target.querySelectorAll<SVGGElement>("g[data-part]"))) {
        const part = g.getAttribute("data-part");
        if (part) {
            map.set(part, g);
        }
    }
    return map;
}
