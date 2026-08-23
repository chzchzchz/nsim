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
    // refresh polls commandQueueJSON() and re-renders the list
    // when the queue revision has changed.
    refresh(): void;
    // clear empties the list (sim reset starts a fresh queue).
    clear(): void;
}

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

    const h = document.createElement("h2");
    h.textContent = "Command Console";
    hostEl.appendChild(h);

    const listEl = document.createElement("div");
    listEl.id = "cmd-list";
    hostEl.appendChild(listEl);

    const inputRow = document.createElement("div");
    inputRow.className = "cmd-input-row";
    hostEl.appendChild(inputRow);

    const ta = document.createElement("textarea");
    ta.id = "cmd-input";
    ta.rows = 3;
    ta.placeholder =
        "TimeOffsetMs Part.Label Value  (one command per line)\n"
        + "e.g. 1000 ReactorBuilding.ReactorCoolantPump.Running 0";
    ta.spellcheck = false;
    inputRow.appendChild(ta);

    const submitBtn = document.createElement("button");
    submitBtn.id = "btn-cmd-submit";
    submitBtn.textContent = "Submit";
    inputRow.appendChild(submitBtn);

    const errorLine = document.createElement("div");
    errorLine.id = "cmd-error";
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
        for (const e of snap.entries) {
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
