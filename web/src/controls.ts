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
    row.className = "ctl-row";

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
            writeControl(plant, e.id, cb.checked ? "1" : "0", cb);
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
                writeControl(plant, e.id, sel.value, sel);
            });
            row.appendChild(sel);
        } else {
            const inp = document.createElement("input");
            inp.type = "text";
            inp.size = 8;
            inp.value = initial;
            inp.addEventListener("change", () => {
                writeControl(plant, e.id, inp.value, inp);
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
            writeControl(plant, e.id, inp.value, inp);
        });
        row.appendChild(inp);
        return row;
    }

    // Numeric kinds.
    const num = document.createElement("input");
    num.type = "number";
    num.step = "0.01";
    // Bound the rendered width: a 6-character box fits ppm,
    // gpm, °F, etc. without growing to the browser default
    // (~20 chars) when the live value happens to be a long
    // integer. maxLength is a soft cap against accidental
    // paste of oversized values.
    num.size = 6;
    num.maxLength = 8;
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
        writeControl(plant, e.id, num.value, num);
    });
    row.appendChild(num);

    const u = unitFor(kind);
    if (u) {
        const unit = document.createElement("span");
        unit.textContent = u;
        unit.className = "ctl-hint";
        row.appendChild(unit);
    }
    if (r) {
        const hint = document.createElement("span");
        hint.textContent = r;
        hint.className = "ctl-hint faint";
        row.appendChild(hint);
    }
    return row;
}

// writeControl applies a widget change and surfaces rejections:
// the plant keeps its previous state, so the widget flashes and
// logs instead of silently pretending the set succeeded.
function writeControl(
    plant: PlantHandle,
    id: string,
    value: string,
    widget: HTMLElement,
): void {
    const err = plant.setControl(id, value);
    if (!(err instanceof Error)) {
        return;
    }
    console.error(`${id}: ${err.message}`);
    widget.classList.add("ctl-error");
    setTimeout(() => widget.classList.remove("ctl-error"), 1500);
}
