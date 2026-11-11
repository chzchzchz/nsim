// faults.ts — shared visibility state for the fault injectors
// (<System>/Faults/*): deliberate malfunction latches, not operator
// actions. Both consoles hide them by default behind a single
// checkbox in the sticky header (#fault-toggle). The toggle is a UX
// guard and a discovery aid, not a security boundary: remote mode
// still requires the server's -allowFaultCommands flag for fault
// writes, and the WASM console applies faults in-page either way.

const STORAGE_KEY = "nsim.showFaultInjectors";

let showFaults = false;
try {
    showFaults = localStorage.getItem(STORAGE_KEY) === "1";
} catch {
    // Storage unavailable (private browsing); hidden stays the default.
}

export function showFaultInjectors(): boolean {
    return showFaults;
}

export function setShowFaultInjectors(show: boolean): void {
    showFaults = show;
    try {
        localStorage.setItem(STORAGE_KEY, show ? "1" : "0");
    } catch {
        // Non-fatal: the choice just does not persist across reloads.
    }
}

export function isFaultEntry(e: { meta: { fault?: boolean } }): boolean {
    return e.meta.fault === true;
}

// initFaultToggle wires the #fault-toggle checkbox in the shared
// header; it is rendered by both consoles because they share the
// shell. onToggle rebuilds whichever console is live.
export function initFaultToggle(onToggle: () => void): void {
    const label = document.getElementById("fault-toggle");
    const cb = document.getElementById("fault-toggle-cb");
    if (!(label instanceof HTMLLabelElement)) return;
    if (!(cb instanceof HTMLInputElement)) return;
    cb.checked = showFaults;
    label.hidden = false;
    cb.addEventListener("change", () => {
        setShowFaultInjectors(cb.checked);
        onToggle();
    });
}
