// layout.ts — persistence for dashboard panel arrangement in
// localStorage. One versioned JSON blob stores tile display order
// and which tiles are minimized. On load the blob is validated
// against the panels that currently exist: a different schema
// version or a name-set mismatch (panels added/removed/renamed
// upstream) discards the saved layout so the UI falls back to its
// defaults rather than rendering half-matching state.

export const PANEL_LAYOUT_VERSION = 1;

const STORAGE_KEY = "nsim-panel-layout";

export interface PanelLayout {
    version: number;
    order: string[];
    minimized: string[];
}

// loadPanelLayout returns the saved layout only when it parses,
// carries the current schema version, and its order names exactly
// the set of panels passed in (same names, same count). Stale
// minimized entries are tolerated individually and filtered out.
export function loadPanelLayout(panels: string[]): PanelLayout | null {
    let raw: string | null;
    try {
        raw = localStorage.getItem(STORAGE_KEY);
    } catch {
        return null;
    }
    if (raw === null) {
        return null;
    }
    let parsed: unknown;
    try {
        parsed = JSON.parse(raw);
    } catch {
        return null;
    }
    if (typeof parsed !== "object" || parsed === null) {
        return null;
    }
    const p = parsed as Partial<PanelLayout>;
    if (p.version !== PANEL_LAYOUT_VERSION) {
        return null;
    }
    if (
        !isStringArray(p.order) ||
        !isStringArray(p.minimized) ||
        !sameNames(p.order, panels)
    ) {
        return null;
    }
    const known = new Set(panels);
    return {
        version: PANEL_LAYOUT_VERSION,
        order: p.order,
        minimized: p.minimized.filter((m) => known.has(m)),
    };
}

export function savePanelLayout(layout: PanelLayout): void {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
    } catch {
        // Private-browsing quotas and disabled storage leave the
        // session arrangement in memory only.
    }
}

function isStringArray(v: unknown): v is string[] {
    return Array.isArray(v) && v.every((e) => typeof e === "string");
}

function sameNames(a: string[], b: string[]): boolean {
    if (a.length !== b.length) {
        return false;
    }
    const x = [...a].sort();
    const y = [...b].sort();
    return x.every((v, i) => v === y[i]);
}
