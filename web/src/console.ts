// console.ts — command console: submit nsimrun-style command
// lines ("TimeOffsetMs Part.Label Value") against the running
// simulation and track their status. Offsets are relative to
// the simulation clock at submit time; the wasm-side queue
// fires them on the exact simulated millisecond.

import type { PlantHandle } from "./plant";

export interface QueueEntry {
    seq: number;
    text: string;
    dueMs: number;
    status: "pending" | "done" | "failed";
    err?: string;
}

interface QueueSnapshot {
    rev: number;
    entries: QueueEntry[];
}

export interface ConsolePanel {
    // refresh polls the queue revision and re-renders the list
    // only when it changed.
    refresh(): void;
    // clear empties the list (sim reset starts a fresh queue).
    clear(): void;
}

// Long sessions accumulate entries; rendering is capped so a rev
// change never rebuilds an unbounded DOM.
const MAX_ROWS = 200;

const STATUS_MARK: Record<QueueEntry["status"], string> = {
    pending: "\u2026",
    done: "\u2713",
    failed: "\u2717",
};

export function initConsole(
    hostEl: HTMLElement,
    plant: PlantHandle,
): ConsolePanel {
    hostEl.innerHTML = "";

    const header = document.createElement("div");
    header.className = "cmd-header";
    hostEl.appendChild(header);

    const h = document.createElement("h2");
    h.textContent = "Command Console";
    header.appendChild(h);

    const errorLine = document.createElement("div");
    errorLine.id = "cmd-error";

    // Downloads the journaled commands as a JSON file for
    // nsimrun replay; distinct from the top-bar clipboard button,
    // which copies the plant state JSON.
    const exportBtn = document.createElement("button");
    exportBtn.id = "btn-cmd-export";
    exportBtn.textContent = "\u2b07 Export log (.json)";
    exportBtn.setAttribute(
        "aria-label",
        "Export command log as JSON file",
    );
    exportBtn.title =
        "Download all commands submitted this run as a JSON file "
        + "(replay with: go run ./cmd/nsimrun -cmd nsim-commands.json). "
        + "Not the clipboard copy \u2014 that one copies plant state.";
    exportBtn.addEventListener("click", () => {
        const raw = plant.commandLogJSON();
        if (typeof raw !== "string") {
            const msg = raw instanceof Error ? raw.message : String(raw);
            errorLine.textContent = `log export failed: ${msg}`;
            return;
        }
        const blob = new Blob([raw], { type: "application/json" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "nsim-commands.json";
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
        errorLine.textContent = "";
    });

    const btnGroup = document.createElement("div");
    btnGroup.className = "cmd-export-btns";
    btnGroup.appendChild(exportBtn);

    const exportPlantBtn = document.createElement("button");
    exportPlantBtn.id = "btn-cmd-export-plant";
    exportPlantBtn.textContent = "\u2b07 Export Plant (.json)";
    exportPlantBtn.setAttribute(
        "aria-label",
        "Export current plant state as JSON file",
    );
    exportPlantBtn.title =
        "Download the current plant state as a JSON file "
        + "(distinct from the command log export).";
    exportPlantBtn.addEventListener("click", () => {
        const raw = plant.toJSON();
        const blob = new Blob([raw], { type: "application/json" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = "nsim-plant.json";
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
        errorLine.textContent = "";
    });
    btnGroup.appendChild(exportPlantBtn);
    header.appendChild(btnGroup);

    const listEl = document.createElement("div");
    listEl.id = "cmd-list";
    hostEl.appendChild(listEl);

    const inputRow = document.createElement("div");
    inputRow.className = "cmd-input-row";
    hostEl.appendChild(inputRow);

    const ta = document.createElement("textarea");
    ta.id = "cmd-input";
    ta.rows = 3;
    ta.setAttribute("aria-label", "Command input");
    ta.placeholder =
        "TimeOffsetMs Part.Label Value  (one command per line)\n"
        + "e.g. 1000 ReactorBuilding.ReactorCoolantPump.Running 0";
    ta.spellcheck = false;
    inputRow.appendChild(ta);

    const submitBtn = document.createElement("button");
    submitBtn.id = "btn-cmd-submit";
    submitBtn.textContent = "Submit";
    inputRow.appendChild(submitBtn);

    hostEl.appendChild(errorLine);

    let lastRev = -1;

    function submit(): void {
        const text = ta.value.trim();
        if (!text) {
            return;
        }
        const err = plant.submitCommands(text);
        if (err instanceof Error) {
            errorLine.textContent = err.message;
            return;
        }
        errorLine.textContent = "";
        ta.value = "";
        refresh();
    }

    submitBtn.addEventListener("click", submit);
    ta.addEventListener("keydown", (ev) => {
        if ((ev.ctrlKey || ev.metaKey) && ev.key === "Enter") {
            ev.preventDefault();
            submit();
        }
    });

    function buildRow(e: QueueEntry): HTMLElement {
        const row = document.createElement("div");
        row.className = `cmd-row ${e.status}`;
        const mark = document.createElement("span");
        mark.className = "cmd-mark";
        mark.textContent = STATUS_MARK[e.status];
        row.appendChild(mark);
        const time = document.createElement("span");
        time.className = "cmd-due";
        time.textContent = `@ ${(e.dueMs / 1000).toFixed(2)}s`;
        row.appendChild(time);
        const text = document.createElement("span");
        text.textContent = e.text;
        row.appendChild(text);
        if (e.err) {
            row.title = e.err;
        }
        return row;
    }

    function refresh(): void {
        // Cheap revision probe first: idle frames must not pay for
        // marshaling and parsing the full snapshot.
        if (plant.commandQueueRev() === lastRev) {
            return;
        }
        const raw = plant.commandQueueJSON();
        if (typeof raw !== "string") {
            return;
        }
        let snap: QueueSnapshot;
        try {
            snap = JSON.parse(raw) as QueueSnapshot;
        } catch {
            console.error("bad command queue JSON:", raw);
            return;
        }
        if (snap.rev === lastRev) {
            return;
        }
        lastRev = snap.rev;
        listEl.replaceChildren();
        const overflow = Math.max(0, snap.entries.length - MAX_ROWS);
        if (overflow > 0) {
            const more = document.createElement("div");
            more.className = "cmd-row cmd-more";
            more.textContent = `+ ${overflow} older ${overflow === 1 ? "entry" : "entries"} hidden`;
            listEl.appendChild(more);
        }
        for (const e of snap.entries.slice(-MAX_ROWS)) {
            listEl.appendChild(buildRow(e));
        }
        listEl.scrollTop = listEl.scrollHeight;
    }

    function clear(): void {
        lastRev = -1;
        listEl.replaceChildren();
    }

    return { refresh, clear };
}
