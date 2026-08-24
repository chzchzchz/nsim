// bindings.ts — collect bridge metadata (part / label / kind) for
// every control and sensor id, and group them by Part. The UI in
// controls.ts and sensors.ts consumes PartBinding[] to render a
// part-grouped panel instead of a flat list of ambiguous names.

import { marked } from "marked";

import type { BridgeMeta, PlantHandle } from "./plant";
import { helpFor } from "./descriptions";
import { knownKind } from "./units";

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

// isAlarmControl reports whether a bridge id belongs to the alarm
// acknowledgment namespace ("Alarms/…"). Those controls are operated
// from the dedicated alarm panel (alarms.ts): clicking an alarm row
// sends Reset with that row's ID, and the panel header acknowledges
// all. Rendering them as generic widgets would duplicate that UI
// awkwardly - ResetAlarm wants an opaque alarm ID and AcknowledgeAll
// is a one-option enum.
export function isAlarmControl(id: string): boolean {
    return id.startsWith("Alarms/");
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

// Duration of the jump-target highlight, matching the ctl-error
// flash cadence used elsewhere.
const REF_FLASH_MS = 1500;

// makeHelpButton builds the "(?)" toggle that reveals an element's
// description inline; shared by the controls panel, the sensor
// readout table, and the alarm panel. Descriptions are markdown (see
// assets/descriptions and assets/alarms) rendered with marked.
export function makeHelpButton(text: string): HTMLButtonElement {
    const btn = document.createElement("button");
    btn.textContent = "?";
    btn.className = "ctl-help";
    btn.title = "What is this?";
    // Stop propagation so a help click inside a row with its own
    // click behavior (alarm acknowledgment) does not fire both.
    btn.addEventListener("click", (ev) => {
        ev.stopPropagation();
        const prev = btn.nextElementSibling;
        if (
            prev instanceof HTMLElement &&
            prev.classList.contains("help-text")
        ) {
            prev.remove();
            return;
        }
        openTip(btn, text);
    });
    return btn;
}

// openTip renders one description next to its button. Markdown
// links whose href is another bridge id are references: clicking
// one closes this popup, scrolls that element's row into view, and
// highlights it briefly. Ids with no rendered row (for example
// alarm controls, which live only in the alarm panel) fall back to
// swapping the popup to the referenced description.
function openTip(btn: HTMLButtonElement, text: string): void {
    const tip = document.createElement("span");
    tip.className = "help-text md";
    tip.innerHTML = marked.parse(text, { async: false });
    tip.addEventListener("click", (ev) => {
        const anchor = (ev.target as HTMLElement | null)?.closest("a");
        const href = anchor?.getAttribute("href") ?? "";
        if (href === "") return;
        ev.preventDefault();
        btn.nextElementSibling?.remove();
        if (!revealElement(href)) {
            const target = helpFor(href);
            if (target !== undefined) {
                openTip(btn, target);
            }
        }
    });
    btn.insertAdjacentElement("afterend", tip);
}

// revealElement jumps the viewport to the rendered row for a bridge
// id and flashes it. Rows tag themselves with data-bridge-id;
// scanning them all is cheap (a few hundred nodes) and avoids
// CSS-escaping the ids, which contain slashes.
function revealElement(id: string): boolean {
    const rows = Array.from(
        document.querySelectorAll<HTMLElement>("[data-bridge-id]"),
    );
    for (const row of rows) {
        if (row.dataset.bridgeId !== id) continue;
        row.scrollIntoView({ behavior: "smooth", block: "center" });
        row.classList.add("ref-flash");
        window.setTimeout(() => row.classList.remove("ref-flash"), REF_FLASH_MS);
        return true;
    }
    return false;
}

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

// sortBindings orders parts alphabetically and, within each part,
// controls and sensors by id. Go map iteration order is randomized,
// so without this the panel rows shuffle on every rebuild.
export function sortBindings(bindings: PartBinding[]): PartBinding[] {
    const byId = (a: Entry, b: Entry): number =>
        a.id.localeCompare(b.id);
    return bindings
        .map((b) => ({
            part: b.part,
            controls: [...b.controls].sort(byId),
            sensors: [...b.sensors].sort(byId),
        }))
        .sort((a, b) => a.part.localeCompare(b.part));
}

export function buildBindings(plant: PlantHandle): PartBinding[] {
    const byPart = new Map<string, PartBinding>();
    const unknownKinds = new Set<string>();
    const noteKind = (meta: BridgeMeta): void => {
        if (!knownKind(meta.kind)) {
            unknownKinds.add(meta.kind);
        }
    };
    for (const e of fetchMeta(plant, plant.controlKeys())) {
        noteKind(e.meta);
        const b = byPart.get(e.meta.part) ?? {
            part: e.meta.part,
            controls: [],
            sensors: [],
        };
        b.controls.push(e);
        byPart.set(e.meta.part, b);
    }
    for (const e of fetchMeta(plant, plant.sensorKeys())) {
        noteKind(e.meta);
        const b = byPart.get(e.meta.part) ?? {
            part: e.meta.part,
            controls: [],
            sensors: [],
        };
        b.sensors.push(e);
        byPart.set(e.meta.part, b);
    }
    if (unknownKinds.size > 0) {
        console.warn(
            "bridge kinds missing from units.ts display table:",
            Array.from(unknownKinds).sort().join(", "),
        );
    }
    return sortBindings(Array.from(byPart.values()));
}
