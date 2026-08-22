// controls.ts — build the control panel. Each bridge.Control
// entry becomes an <input>/<select> bound to plant.setControl.

import type { PlantHandle } from "./plant";

export function renderPanel(host: HTMLElement, plant: PlantHandle): void {
    host.innerHTML = "";
    const controls = plant.controlKeys();
    if (controls.length === 0) {
        host.textContent = "(no controls)";
        return;
    }

    // We need the Kind for each control; read it back via
    // setControl with the current value to obtain the existing
    // kind from the bridge. Since we don't expose kinds
    // directly through wasm, we infer the widget type from the
    // id string: anything with "_Running" or boolean-sounding
    // labels is bool; everything else is normalized.
    for (const id of controls) {
        const label = id.slice(id.lastIndexOf("/") + 1);
        const field = document.createElement("fieldset");
        const lg = document.createElement("legend");
        lg.textContent = label;
        field.appendChild(lg);

        const isBool = label === "Running" || label === "HeatersOn";
        if (isBool) {
            const cb = document.createElement("input");
            cb.type = "checkbox";
            cb.addEventListener("change", () => {
                plant.setControl(id, cb.checked ? "1" : "0");
            });
            field.appendChild(cb);
        } else {
            const num = document.createElement("input");
            num.type = "number";
            num.step = "0.01";
            num.min = "0";
            num.max = "1";
            num.value = "0";
            num.addEventListener("change", () => {
                plant.setControl(id, num.value);
            });
            field.appendChild(num);
        }
        host.appendChild(field);
    }
}
