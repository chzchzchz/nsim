// bindings.ts — collect bridge metadata (part / label / kind) for
// every control and sensor id, and group them by Part. The UI in
// controls.ts and sensors.ts consumes PartBinding[] to render a
// part-grouped panel instead of a flat list of ambiguous names.

import type { BridgeMeta, PlantHandle } from "./plant";

export interface Entry {
    id: string;
    meta: BridgeMeta;
}

export interface PartBinding {
    part: string;
    controls: Entry[];
    sensors: Entry[];
}

// PrefixGroup bundles the PartBindings that share a top-level
// prefix. The top-level prefix is the leading slash-delimited
// segment of the Part path (e.g. "ReactorBuilding" for
// "ReactorBuilding/Pressurizer/HeatersOn"). The UI renders one
// tile per prefix so the panel can show e.g. ReactorBuilding
// next to TurbineBuilding side by side.
export interface PrefixGroup {
    prefix: string;
    bindings: PartBinding[];
}

// topLevelPrefix extracts the leading slash-delimited segment of
// a part path. An empty or unrooted path returns "" so callers
// can render it under a "(other)" heading instead of crashing.
export function topLevelPrefix(part: string): string {
    const i = part.indexOf("/");
    return i < 0 ? part : part.slice(0, i);
}

// groupByPrefix buckets PartBindings by their top-level prefix
// and returns the buckets in alphabetical order. Bindings within
// each bucket retain the sort order from buildBindings().
export function groupByPrefix(bindings: PartBinding[]): PrefixGroup[] {
    const byPrefix = new Map<string, PartBinding[]>();
    for (const b of bindings) {
        const k = topLevelPrefix(b.part);
        const list = byPrefix.get(k);
        if (list) {
            list.push(b);
        } else {
            byPrefix.set(k, [b]);
        }
    }
    return Array.from(byPrefix.entries())
        .sort((a, b) => a[0].localeCompare(b[0]))
        .map(([prefix, bs]) => ({ prefix, bindings: bs }));
}

// displayPart returns a closure that strips the tile prefix from
// a full part path for display. e.g. with prefix "Outside",
// "Outside/DieselGenerator" becomes "DieselGenerator" and
// "Outside/DieselGenerator/Generator" becomes
// "DieselGenerator/Generator". The full path is preserved on the
// PartBinding itself; only the heading text is shortened.
export function displayPart(prefix: string): PartDisplay {
    const head = prefix + "/";
    return (part: string): string =>
        part.startsWith(head) ? part.slice(head.length) : part;
}

// PartDisplay shortens a full Part path for use as a heading.
// Both controls.ts and sensors.ts accept it so the same shortened
// string is reused for legends, sensor sub-headings, etc.
export type PartDisplay = (part: string) => string;

function fetchMeta(plant: PlantHandle, ids: string[]): Entry[] {
    const out: Entry[] = [];
    for (const id of ids) {
        const meta = plant.getMeta(id);
        if (meta instanceof Error) {
            // Skip unknown ids rather than crashing the whole panel.
            // The wasm side will have already produced a sensible
            // error string; we don't surface it here.
            continue;
        }
        out.push({ id, meta });
    }
    return out;
}

export function buildBindings(plant: PlantHandle): PartBinding[] {
    const byPart = new Map<string, PartBinding>();
    for (const e of fetchMeta(plant, plant.controlKeys())) {
        const b = byPart.get(e.meta.part) ?? {
            part: e.meta.part,
            controls: [],
            sensors: [],
        };
        b.controls.push(e);
        byPart.set(e.meta.part, b);
    }
    for (const e of fetchMeta(plant, plant.sensorKeys())) {
        const b = byPart.get(e.meta.part) ?? {
            part: e.meta.part,
            controls: [],
            sensors: [],
        };
        b.sensors.push(e);
        byPart.set(e.meta.part, b);
    }
    return Array.from(byPart.values()).sort((a, b) =>
        a.part.localeCompare(b.part),
    );
}
