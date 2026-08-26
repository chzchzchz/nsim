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
// from the dedicated alarm panel (alarms.ts): each row carries an
// eyes button that sends Reset with that row's ID, and the panel
// header acknowledges all. Rendering them as generic widgets would duplicate that UI
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
// "DieselGenerator<wbr>/<wbr>Generator". The full path is preserved on
// the PartBinding itself; only the heading text is shortened and
// wrapped at slash boundaries.
export function displayPart(prefix: string): PartDisplay {
    const head = prefix + "/";
    return (part: string): string =>
        part.startsWith(head)
            ? part.slice(head.length).replace(/\//g, "/<wbr>")
            : part.replace(/\//g, "/<wbr>");
}

// PartDisplay shortens a full Part path for use as a heading.
// Both controls.ts and sensors.ts accept it so the same shortened
// string is reused for legends, sensor sub-headings, etc.
export type PartDisplay = (part: string) => string;

// Duration of the jump-target highlight, matching the ctl-error
// flash cadence used elsewhere.
export const REF_FLASH_MS = 1500;

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

// crossReferenceHref returns the bridge id targeted by an anchor
// click inside rendered markdown, or "" for ordinary clicks;
// reference links get their default navigation suppressed.
export function crossReferenceHref(ev: Event): string {
    const anchor = (ev.target as HTMLElement | null)?.closest("a");
    const href = anchor?.getAttribute("href") ?? "";
    if (href !== "") {
        ev.preventDefault();
    }
    return href;
}

// openTip renders one description next to its button. Markdown
// links whose href is another bridge id are references: clicking
// one scrolls that element's row into view (or swaps the popup
// to the referenced description if no rendered row exists). The
// popup stays open unless the user explicitly closes it by
// clicking the "?" button again.
function openTip(btn: HTMLButtonElement, text: string): void {
    const tip = document.createElement("span");
    tip.className = "help-text md";
    tip.innerHTML = marked.parse(text, { async: false });
    tip.addEventListener("click", (ev) => {
        const href = crossReferenceHref(ev);
        if (href === "") return;
        if (revealElement(href)) {
            // Jumped to a rendered row and flashed it. Keep the
            // existing popup open and unchanged.
        } else {
            const target = helpFor(href);
            if (target !== undefined) {
                btn.nextElementSibling?.remove();
                openTip(btn, target);
            }
        }
    });
    btn.insertAdjacentElement("afterend", tip);
}

// SCROLL_SETTLE_FALLBACK_MS bounds how long to wait for the smooth
// jump to finish when the browser never fires scrollend (no scroll
// needed, or unsupported).
const SCROLL_SETTLE_FALLBACK_MS = 1200;

// flashRow scrolls a rendered row into view and highlights it
// briefly; both reveal helpers share it. For off-screen targets the
// highlight starts only after the scroll settles: starting it early
// means a long jump from the diagram arrives after the flash
// already expired.
function flashRow(row: HTMLElement): void {
    const start = (): void => {
        row.classList.add("ref-flash");
        window.setTimeout(() => row.classList.remove("ref-flash"), REF_FLASH_MS);
    };
    const rect = row.getBoundingClientRect();
    if (rect.top >= 0 && rect.bottom <= window.innerHeight) {
        start();
        return;
    }
    row.scrollIntoView({ behavior: "smooth", block: "center" });
    // scrollend fires when the smooth jump settles; browsers
    // without it (or jumps that scroll nowhere) fall back to the
    // bounded timer.
    let settled = false;
    const once = (): void => {
        if (settled) return;
        settled = true;
        window.removeEventListener("scrollend", once);
        start();
    };
    window.addEventListener("scrollend", once);
    window.setTimeout(once, SCROLL_SETTLE_FALLBACK_MS);
}

// revealElement jumps the viewport to the rendered row for a bridge
// id and flashes it. Rows tag themselves with data-bridge-id;
// scanning them all is cheap (a few hundred nodes) and avoids
// CSS-escaping the ids, which contain slashes.
export function revealElement(id: string): boolean {
    const rows = Array.from(
        document.querySelectorAll<HTMLElement>("[data-bridge-id]"),
    );
    for (const row of rows) {
        if (row.dataset.bridgeId !== id) continue;
        flashRow(row);
        return true;
    }
    return false;
}

// revealPart jumps to the best rendered row for a diagram part
// (the data-part value on a P&ID component group). Clicking a
// component usually means "operate it", so control rows outrank
// sensor rows, and exact id matches outrank prefix matches because
// diagram symbols can be more granular than binding parts.
export function revealPart(part: string): boolean {
    const prefix = part + "/";
    let best: { row: HTMLElement; score: number } | null = null;
    const rows = Array.from(
        document.querySelectorAll<HTMLElement>("[data-bridge-id]"),
    );
    for (const row of rows) {
        const id = row.dataset.bridgeId ?? "";
        let score: number;
        if (id === part) {
            score = 2;
        } else if (id.startsWith(prefix)) {
            score = 1;
        } else {
            continue;
        }
        if (row.classList.contains("ctl-row")) score += 0.5;
        if (!best || score > best.score) best = { row, score };
    }
    if (!best) return false;
    flashRow(best.row);
    return true;
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
    const bucketFor = (part: string): PartBinding => {
        const b = byPart.get(part);
        if (b) {
            return b;
        }
        const fresh: PartBinding = { part, controls: [], sensors: [] };
        byPart.set(part, fresh);
        return fresh;
    };
    const unknownKinds = new Set<string>();
    const noteKind = (meta: BridgeMeta): void => {
        if (!knownKind(meta.kind)) {
            unknownKinds.add(meta.kind);
        }
    };
    for (const e of fetchMeta(plant, plant.controlKeys())) {
        noteKind(e.meta);
        bucketFor(e.meta.part).controls.push(e);
    }
    for (const e of fetchMeta(plant, plant.sensorKeys())) {
        noteKind(e.meta);
        bucketFor(e.meta.part).sensors.push(e);
    }
    if (unknownKinds.size > 0) {
        console.warn(
            "bridge kinds missing from units.ts display table:",
            Array.from(unknownKinds).sort().join(", "),
        );
    }
    return sortBindings(Array.from(byPart.values()));
}
