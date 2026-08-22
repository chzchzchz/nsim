// controls.ts — build the control panel from PartBinding[]. Each
// bridge.Control becomes a checkbox / number / enum picker driven
// by its Kind; controls are grouped under a part-level heading
// that reads "Part.Label".

import type { Entry, PartBinding, PartDisplay } from "./bindings";
import type { PlantHandle } from "./plant";
import { rangeFor, unitFor } from "./units";

export function renderPanel(
    host: HTMLElement,
    bindings: PartBinding[],
    plant: PlantHandle,
    display: PartDisplay,
): void {
    host.innerHTML = "";
    const controlParts = bindings.filter((b) => b.controls.length > 0);
    if (controlParts.length === 0) {
        host.textContent = "(no controls)";
        return;
    }

    for (const part of controlParts) {
        host.appendChild(renderPart(part, plant, display));
    }
}

function renderPart(
    part: PartBinding,
    plant: PlantHandle,
    display: PartDisplay,
): HTMLElement {
    const fs = document.createElement("fieldset");
    const lg = document.createElement("legend");
    lg.textContent = display(part.part);
    fs.appendChild(lg);

    for (const e of part.controls) {
        fs.appendChild(renderControl(e, plant));
    }
    return fs;
}

function renderControl(e: Entry, plant: PlantHandle): HTMLElement {
    const { kind, label } = e.meta;
    const row = document.createElement("div");
    row.style.display = "flex";
    row.style.alignItems = "center";
    row.style.gap = "6px";
    row.style.margin = "2px 0";

    const text = document.createElement("span");
    text.textContent = label;
    row.appendChild(text);

    // Read the control's current value once so the UI reflects
    // live plant state on initial render. Get returns "0"/"1"
    // for bool, the raw string for enum/string, and the
    // canonical numeric string for everything else.
    const current = plant.getControlValue(e.id);
    const initial = typeof current === "string" ? current : "";

    if (kind === "bool") {
        const cb = document.createElement("input");
        cb.type = "checkbox";
        cb.checked = initial === "1" || initial === "true";
        cb.addEventListener("change", () => {
            plant.setControl(e.id, cb.checked ? "1" : "0");
        });
        row.appendChild(cb);
        return row;
    }

    if (kind === "enum") {
        // When the Go backend populated bridge.Control.Enum we
        // get a labelled <select>; otherwise fall back to a
        // free-text input (preserves the legacy behaviour for
        // enum controls that haven't been annotated yet).
        if (e.meta.enum && e.meta.enum.length > 0) {
            const sel = document.createElement("select");
            for (let i = 0; i < e.meta.enum.length; i++) {
                const opt = document.createElement("option");
                opt.value = String(i);
                opt.textContent = e.meta.enum[i];
                sel.appendChild(opt);
            }
            sel.value = initial;
            sel.addEventListener("change", () => {
                plant.setControl(e.id, sel.value);
            });
            row.appendChild(sel);
        } else {
            const inp = document.createElement("input");
            inp.type = "text";
            inp.size = 8;
            inp.value = initial;
            inp.addEventListener("change", () => {
                plant.setControl(e.id, inp.value);
            });
            row.appendChild(inp);
        }
        return row;
    }

    if (kind === "string") {
        const inp = document.createElement("input");
        inp.type = "text";
        inp.size = 8;
        inp.value = initial;
        inp.addEventListener("change", () => {
            plant.setControl(e.id, inp.value);
        });
        row.appendChild(inp);
        return row;
    }

    // Numeric kinds.
    const num = document.createElement("input");
    num.type = "number";
    num.step = "0.01";
    const r = rangeFor(kind);
    if (r) {
        const m = r.match(/^\[(-?[\d.]+),\s*(-?[\d.]+)\]$/);
        if (m) {
            num.min = m[1];
            num.max = m[2];
        }
    }
    num.value = initial === "" ? "0" : initial;
    num.addEventListener("change", () => {
        plant.setControl(e.id, num.value);
    });
    row.appendChild(num);

    const u = unitFor(kind);
    if (u) {
        const unit = document.createElement("span");
        unit.textContent = u;
        unit.style.opacity = "0.6";
        row.appendChild(unit);
    }
    if (r) {
        const hint = document.createElement("span");
        hint.textContent = r;
        hint.style.opacity = "0.4";
        hint.style.fontSize = "10px";
        row.appendChild(hint);
    }
    return row;
}
