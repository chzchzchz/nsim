// scenarios.ts — scenario selection screen. The catalog comes from
// the wasm side (core.Scenarios); this module renders the picker
// overlay and the acknowledgement dialog that explains the evolution
// before the plant is built with its starting state.

export interface ScenarioInfo {
    id: string;
    number: number;
    category: string;
    title: string;
    startState: string;
    objective: string;
    // duration is a coarse simulated-time estimate; absent for
    // operator-paced evolutions.
    duration?: string;
}

export interface ScenarioGroup {
    category: string;
    items: ScenarioInfo[];
}

export function loadScenarios(): ScenarioInfo[] {
    if (typeof window === "undefined" || !window.nsimScenarios) {
        throw new Error("nsim wasm not loaded; call loadWasm() first");
    }
    return JSON.parse(window.nsimScenarios()) as ScenarioInfo[];
}

// groupByCategory buckets the catalog in first-seen order so the
// picker mirrors the catalog's section layout.
export function groupByCategory(list: ScenarioInfo[]): ScenarioGroup[] {
    const groups: ScenarioGroup[] = [];
    const index = new Map<string, ScenarioGroup>();
    for (const s of list) {
        let g = index.get(s.category);
        if (!g) {
            g = { category: s.category, items: [] };
            index.set(s.category, g);
            groups.push(g);
        }
        g.items.push(s);
    }
    return groups;
}

export interface ScenarioPickerCallbacks {
    // onStart receives the chosen scenario id, or null when the
    // operator picked free play on the default plant.
    onStart(scenarioId: string | null): void;
}

export interface ScenarioPicker {
    close(): void;
}

// ObjectiveStatus mirrors the wasm objectiveJSON snapshot. MetMs
// carries the simulated time completion first latched.
export interface ObjectiveStatus {
    active: boolean;
    id?: string;
    title?: string;
    done: boolean;
    metMs?: number;
    detail: string;
}

// objectiveStatusText renders the completion line for the live
// banner: the unmet bounds while pending, a confirmation with the
// met time once latched.
export function objectiveStatusText(
    done: boolean,
    detail: string,
    metMs?: number,
): string {
    if (!done) {
        return `pending — ${detail}`;
    }
    return metMs !== undefined
        ? `✓ objective met @ ${(metMs / 1000).toFixed(1)} s simulated`
        : "✓ objective met";
}

// renderScenarioPicker mounts a modal overlay listing every catalog
// entry grouped by category. Clicking an entry swaps the panel to an
// acknowledgement dialog stating what the operator is supposed to do;
// confirming starts the scenario.
export function renderScenarioPicker(
    host: HTMLElement,
    scenarios: ScenarioInfo[],
    callbacks: ScenarioPickerCallbacks,
): ScenarioPicker {
    const backdrop = document.createElement("div");
    backdrop.className = "overlay-backdrop";
    const panel = document.createElement("div");
    panel.className = "scenario-picker";
    backdrop.appendChild(panel);
    host.appendChild(backdrop);

    const close = (): void => { backdrop.remove(); };

    function showList(): void {
        panel.innerHTML = "";
        const header = document.createElement("header");
        const h2 = document.createElement("h2");
        h2.textContent = "Select a starting scenario";
        header.appendChild(h2);
        panel.appendChild(header);

        const body = document.createElement("div");
        body.className = "picker-body";

        const freeplay = document.createElement("button");
        freeplay.className = "scenario-freeplay";
        freeplay.textContent = "Free play — default full-power plant";
        freeplay.addEventListener("click", () => {
            close();
            callbacks.onStart(null);
        });
        body.appendChild(freeplay);

        for (const group of groupByCategory(scenarios)) {
            const h3 = document.createElement("h3");
            h3.textContent = group.category;
            body.appendChild(h3);
            const grid = document.createElement("div");
            grid.className = "scenario-grid";
            for (const s of group.items) {
                grid.appendChild(makeCard(s));
            }
            body.appendChild(grid);
        }
        panel.appendChild(body);
    }

    function makeCard(s: ScenarioInfo): HTMLElement {
        const card = document.createElement("button");
        card.className = "scenario-card";
        card.dataset.scenarioId = s.id;
        const num = document.createElement("span");
        num.className = "num";
        num.textContent = String(s.number);
        const title = document.createElement("span");
        title.className = "title";
        title.textContent = s.title;
        card.appendChild(num);
        card.appendChild(document.createTextNode(" "));
        card.appendChild(title);
        card.addEventListener("click", () => showAcknowledgement(s));
        return card;
    }

    // The acknowledgement dialog is the confirmation gate before the
    // scenario state replaces the running plant.
    function showAcknowledgement(s: ScenarioInfo): void {
        panel.innerHTML = "";
        const detail = document.createElement("div");
        detail.className = "scenario-detail";

        const h2 = document.createElement("h2");
        h2.textContent = `${s.number}. ${s.title}`;
        detail.appendChild(h2);

        const start = document.createElement("p");
        const startLabel = document.createElement("span");
        startLabel.className = "label";
        startLabel.textContent = "Starting condition: ";
        start.appendChild(startLabel);
        start.appendChild(document.createTextNode(s.startState));
        detail.appendChild(start);

        const task = document.createElement("p");
        const taskLabel = document.createElement("span");
        taskLabel.className = "label";
        taskLabel.textContent = "Your objective: ";
        task.appendChild(taskLabel);
        task.appendChild(document.createTextNode(s.objective));
        detail.appendChild(task);

        if (s.duration) {
            const est = document.createElement("p");
            const estLabel = document.createElement("span");
            estLabel.className = "label";
            estLabel.textContent = "Expected duration: ";
            est.appendChild(estLabel);
            est.appendChild(document.createTextNode(s.duration));
            detail.appendChild(est);
        }

        panel.appendChild(detail);

        const actions = document.createElement("div");
        actions.className = "scenario-actions";
        const backBtn = document.createElement("button");
        backBtn.textContent = "Back";
        backBtn.addEventListener("click", showList);
        const beginBtn = document.createElement("button");
        beginBtn.className = "primary";
        beginBtn.textContent = "Begin scenario";
        beginBtn.addEventListener("click", () => {
            close();
            callbacks.onStart(s.id);
        });
        actions.appendChild(backBtn);
        actions.appendChild(beginBtn);
        panel.appendChild(actions);
    }

    showList();
    return { close };
}
