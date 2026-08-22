// bindings.ts — map control / sensor keys to SVG part groups
// by their Part string. Also dispatches UI widget construction
// for each control and overlays for each sensor.

import type { PlantHandle } from "./plant";

export interface PartBinding {
    part: string;
    controls: string[];
    sensors: string[];
}

export function buildBindings(plant: PlantHandle): PartBinding[] {
    const byPart = new Map<string, PartBinding>();
    for (const k of plant.controlKeys()) {
        const part = k.slice(0, k.lastIndexOf("/"));
        if (!byPart.has(part)) {
            byPart.set(part, { part, controls: [], sensors: [] });
        }
        byPart.get(part)!.controls.push(k);
    }
    for (const k of plant.sensorKeys()) {
        const part = k.slice(0, k.lastIndexOf("/"));
        if (!byPart.has(part)) {
            byPart.set(part, { part, controls: [], sensors: [] });
        }
        byPart.get(part)!.sensors.push(k);
    }
    return Array.from(byPart.values()).sort((a, b) =>
        a.part.localeCompare(b.part),
    );
}
