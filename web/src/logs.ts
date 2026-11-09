// logs.ts — Logs panel: live audit trail from GET /logs
// with incremental ?lo= polling, a comment-only composer
// (POST /logs), and a small "N new entries" banner driven
// by /status.latestSeq. Remote-mode only.

import type { LogEntry, RemoteClient } from "./remote";
import { RemoteError } from "./remote";

const MAX_ROWS = 200;
const LOG_SELECTED_CLASS = "log-selected";
const SCROLL_LOCKOUT_MS = 500;
const FOLLOW_KEY = "nsim:logs:follow";
const ALERT_STRIP_ID = "alert-strip";

// getAlertStrip returns the single page-level alert strip in
// <body>, creating it on first use. Single-strip ownership: at
// most one alert is ever shown; a new showAlert replaces it.
// The strip is fixed below the sticky header so it follows the
// window while the operator is looking at the tiles.
function getAlertStrip(): HTMLDivElement {
    let strip = document.getElementById(ALERT_STRIP_ID) as HTMLDivElement | null;
    if (strip) return strip;
    strip = document.createElement("div");
    strip.id = ALERT_STRIP_ID;
    strip.className = "alert-strip";
    strip.hidden = true;
    strip.setAttribute("role", "status");
    document.body.appendChild(strip);
    return strip;
}

function hideAlertStrip(): void {
    const strip = document.getElementById(ALERT_STRIP_ID);
    if (!strip) return;
    strip.hidden = true;
    strip.replaceChildren();
}

export interface LogsPanel {
    // Append newly fetched entries and advance the cursor.
    append(entries: LogEntry[]): void;
    // Entries that look like another client acted: seqs above
    // the caller's last seen that aren't this browser's own.
    noteForeign(count: number, latest: LogEntry | undefined): void;
    setWritable(writable: boolean): void;
    onOwnSeq(seq: number): void;
    // Currently selected entry, if any (clicking the row again deselects).
    selected(): LogEntry | null;
    // Programmatic deselect; also fires the callback with null.
    select(entry: LogEntry | null): void;
    // True if a banner "View" scroll happened within the lockout
    // window; used to suppress the control-tile scroll while the
    // list is scrolling to the newest entry.
    isScrollLockoutActive(): boolean;
    // Whether auto-follow is on (checkbox). When on, a foreign
    // operation entry is auto-selected (jump + red box) and a
    // toast banner with the comment is shown.
    follow(): boolean;
    setFollow(on: boolean): void;
    // Auto-select `entry` (same as clicking its row): shows the
    // red box on the control via onSelect and selects the row.
    autoSelect(entry: LogEntry): void;
    // Show the dismissible auto-banner for a foreign entry and
    // auto-dismiss it; a no-op for comment-kind entries.
    showAutoBanner(entry: LogEntry): void;
}

export function initLogs(
    hostEl: HTMLElement,
    client: RemoteClient,
    onSelect?: (entry: LogEntry | null) => void,
): LogsPanel {
    hostEl.innerHTML = "";
    // A re-created panel (runId change) must not leave the previous
    // panel's alert strip on screen with stale text.
    hideAlertStrip();

    const banner = document.createElement("div");
    banner.className = "logs-banner";
    banner.hidden = true;
    hostEl.appendChild(banner);

    // Auto-follow toggle. The checkbox gates everything: when on,
    // a foreign operation entry auto-selects (jump + red box) and
    // posts a dismissible toast with the comment. Persisted so a
    // refresh keeps the operator's choice; default off.
    const followRow = document.createElement("div");
    followRow.className = "logs-follow-row";
    hostEl.appendChild(followRow);

    const followLabel = document.createElement("label");
    followLabel.className = "logs-follow";
    followRow.appendChild(followLabel);

    const followCb = document.createElement("input");
    followCb.type = "checkbox";
    followCb.id = "logs-follow";
    try {
        followCb.checked = localStorage.getItem(FOLLOW_KEY) === "1";
    } catch {
        followCb.checked = false;
    }
    followCb.title = "Auto-jump to the changed control (with red box) when a new foreign log entry arrives";
    followCb.setAttribute("aria-label", "Auto-follow new log entries");
    followLabel.append(followCb, document.createTextNode(" Follow new entries"));

    // The auto-alert now lives in the page-level strip (see
    // getAlertStrip), not in this panel.
    const listEl = document.createElement("div");
    listEl.className = "logs-list";
    hostEl.appendChild(listEl);

    const inputRow = document.createElement("div");
    inputRow.className = "logs-input-row";
    hostEl.appendChild(inputRow);

    const input = document.createElement("input");
    input.type = "text";
    input.placeholder = "Add comment to log…";
    input.setAttribute("aria-label", "Add comment to log");
    input.spellcheck = false;
    input.autocomplete = "off";
    inputRow.appendChild(input);

    const postBtn = document.createElement("button");
    postBtn.className = "btn";
    postBtn.textContent = "Post";
    postBtn.disabled = true;
    inputRow.appendChild(postBtn);

    const errorLine = document.createElement("div");
    errorLine.className = "logs-error";
    errorLine.setAttribute("role", "alert");
    hostEl.appendChild(errorLine);

    let lastSeq = -1;
    let ownSeq = -1;
    let bannerCount = 0;
    let bannerLatest: LogEntry | undefined;
    let writable = false;
    // Selected log entry, or null. The single source of truth for
    // both the row highlight (class log-selected + aria-pressed)
    // and the control-row outline driven via onSelect. Sequence
    // and entry are not kept separately; seq = entry?.seq.
    let selectedEntry: LogEntry | null = null;
    // Timestamp of the last banner "View" scroll; checked by the
    // RemoteController before scrolling the control tile so the
    // two smooth scrolls don't fight.
    let lastViewScrollAt = 0;

    function applySelection(entry: LogEntry | null): void {
        selectedEntry = entry;
        for (const row of Array.from(listEl.children) as HTMLElement[]) {
            const isSelected =
                entry !== null && row.dataset.seq === String(entry.seq);
            row.classList.toggle(LOG_SELECTED_CLASS, isSelected);
            if (row.getAttribute("role") === "button") {
                row.setAttribute("aria-pressed", String(isSelected));
            }
        }
        // Roving tabindex: only the selected row (or the first
        // operation row when nothing is selected) is tabbable, so
        // 200 rows don't all sit in the tab order.
        syncRovingTabindex();
        onSelect?.(entry);
    }

    // Ensure exactly one operation row has tabindex=0, the rest
    // -1. The stop is the selected row if any; otherwise the
    // first operation row. Non-operation rows keep tabindex=-1
    // semantics by never being given the stop.
    //
    // Focus handling (fix #1): if pruning removed the row that
    // held focus (activeElement is no longer inside the list),
    // the browser drops focus to <body>. We move focus to the
    // new stop so keyboard users don't have to Tab from the top
    // of the document to re-enter the list.
    function syncRovingTabindex(): void {
        let found = false;
        let stop: HTMLElement | null = null;
        for (const row of Array.from(listEl.children) as HTMLElement[]) {
            if (row.getAttribute("role") !== "button") continue;
            const shouldStop =
                selectedEntry !== null
                    ? row.dataset.seq === String(selectedEntry.seq)
                    : !found;
            if (shouldStop) {
                found = true;
                stop = row;
            }
            row.tabIndex = shouldStop ? 0 : -1;
        }
        // If focus was on a row that is no longer in the list
        // (pruned while selected/focused), hand it to the new stop.
        if (
            stop !== null &&
            document.activeElement !== null &&
            document.activeElement !== listEl &&
            !listEl.contains(document.activeElement)
        ) {
            // Only steal focus when the previously focused element
            // was inside this panel (i.e. the browser just dropped
            // it to <body>); don't yank focus away from other
            // parts of the page.
            const detached = document.activeElement === document.body;
            if (detached && listEl.dataset.hadRowFocus === "1") {
                stop.focus({ preventScroll: true });
            }
        }
    }

    // Log entries keyed by seq, pruned in lockstep with the DOM
    // (see append()). The map gives the delegated click listener
    // a single lookup path keyed off dataset.seq, so the element
    // itself only carries the serializable dataset attributes.
    const entriesBySeq = new Map<number, LogEntry>();

    function entryOf(row: HTMLElement): LogEntry | null {
        const seq = Number(row.dataset.seq);
        if (!Number.isFinite(seq) || !row.dataset.control) return null;
        return entriesBySeq.get(seq) ?? null;
    }

    listEl.addEventListener("focusin", () => {
        listEl.dataset.hadRowFocus = "1";
    });
    listEl.addEventListener("focusout", () => {
        listEl.dataset.hadRowFocus = "0";
    });

    listEl.addEventListener("click", (ev) => {
        const row = (ev.target as HTMLElement).closest<HTMLElement>(
            ".log-row[role='button']",
        );
        if (!row || !listEl.contains(row)) return;
        const entry = entryOf(row);
        if (!entry) return;
        applySelection(
            selectedEntry !== null && selectedEntry.seq === entry.seq
                ? null
                : entry,
        );
    });

    // Escape-to-deselect is handled on the host element below
    // (bubbles up from both list and input); listEl handles only
    // the row-level Enter/Space activation.
    listEl.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter" || ev.key === " ") {
            ev.preventDefault();
            (ev.target as HTMLElement).click();
        }
    });

    // Escape anywhere in the panel (input, banner, list) deselects,
    // so the user doesn't have to be focused on the selected row
    // to dismiss it. Single handler: no double-fire.
    hostEl.addEventListener("keydown", (ev) => {
        if (ev.key === "Escape" && selectedEntry !== null) {
            ev.preventDefault();
            applySelection(null);
        }
    });

    function renderBanner(): void {
        if (bannerCount === 0) {
            banner.hidden = true;
            return;
        }
        banner.hidden = false;
        banner.innerHTML = "";
        const textEl = document.createElement("span");
        const latest = bannerLatest;
        if (bannerCount === 1 && latest) {
            const what = latest.kind === "comment"
                ? `comment: ${latest.comment ?? ""}`
                : `${latest.control} = ${latest.value}`;
            textEl.textContent = `1 new entry — latest: ${what}`;
        } else {
            textEl.textContent = `${bannerCount} new entries`;
        }
        const viewBtn = document.createElement("button");
        viewBtn.className = "btn";
        viewBtn.textContent = "View";
        const dismissBtn = document.createElement("button");
        dismissBtn.className = "btn";
        dismissBtn.textContent = "×";
        dismissBtn.setAttribute("aria-label", "Dismiss");
        const clear = (): void => {
            bannerCount = 0;
            bannerLatest = undefined;
            renderBanner();
        };
        viewBtn.addEventListener("click", () => {
            const last = listEl.lastElementChild;
            if (last instanceof HTMLElement) {
                last.scrollIntoView({ behavior: "smooth", block: "nearest" });
            }
            lastViewScrollAt = performance.now();
            clear();
        });
        dismissBtn.addEventListener("click", clear);
        textEl.addEventListener("click", () => { viewBtn.click(); });
        banner.append(textEl, viewBtn, dismissBtn);
    }

    function formatRow(e: LogEntry): string {
        const time = `@ ${(e.atMs / 1000).toFixed(1)}s`;
        if (e.kind === "comment") return `${time}  ${e.comment ?? ""}`;
        const base = `${time}  ${e.control} = ${e.value}`;
        return e.comment ? `${base}  ${e.comment}` : base;
    }

    function append(entries: LogEntry[]): void {
        for (const e of entries) {
            if (e.seq <= lastSeq) continue;
            lastSeq = e.seq;
            const row = document.createElement("div");
            row.className = `log-row ${e.kind}`;
            row.dataset.seq = String(e.seq);
            row.textContent = formatRow(e);
            entriesBySeq.set(e.seq, e);
            if (e.kind === "operation" && e.control) {
                // Stops in the tab order are assigned by
                // syncRovingTabindex() (called after each batch);
                // default to -1, then the first operation row
                // becomes the stop.
                row.tabIndex = -1;
                row.setAttribute("role", "button");
                row.setAttribute("aria-pressed", "false");
                row.dataset.control = e.control;
                // Script rows keep both hints instead of overwriting.
                row.title = e.source === "script"
                    ? `scenario script — click to highlight ${e.control}`
                    : `click to highlight ${e.control}`;
            } else if (e.source === "script") {
                row.title = "scenario script";
            }
            if (selectedEntry !== null && selectedEntry.seq === e.seq) {
                row.classList.add(LOG_SELECTED_CLASS);
                row.setAttribute("aria-pressed", "true");
            }
            listEl.appendChild(row);
        }
        let evictedSelection = false;
        while (listEl.children.length > MAX_ROWS) {
            const first = listEl.firstChild as HTMLElement | null;
            if (first !== null && first.dataset.seq) {
                entriesBySeq.delete(Number(first.dataset.seq));
            }
            if (
                first !== null &&
                selectedEntry !== null &&
                first.dataset.seq === String(selectedEntry.seq)
            ) {
                evictedSelection = true;
            }
            listEl.removeChild(listEl.firstChild!);
        }
        // The selected row aged out of the window; drop the
        // selection so the control outline doesn't outlive the
        // log row the user clicked.
        if (evictedSelection) {
            applySelection(null);
        } else if (entries.length > 0) {
            syncRovingTabindex();
        }
        listEl.scrollTop = listEl.scrollHeight;
    }

    function noteForeign(count: number, latest: LogEntry | undefined): void {
        if (count <= 0) return;
        bannerCount += count;
        if (latest !== undefined) bannerLatest = latest;
        renderBanner();
    }

    let followOn = followCb.checked;

    followCb.addEventListener("change", () => {
        followOn = followCb.checked;
        try {
            localStorage.setItem(FOLLOW_KEY, followOn ? "1" : "0");
        } catch {
            // Private mode / SSR: the checkbox still works for
            // this session.
        }
        if (!followOn) {
            hideAlertStrip();
            // Same rationale as setFollow(false) in the returned
            // panel: unchecking follow clears the live selection.
            if (selectedEntry !== null) {
                applySelection(null);
            }
        }
    });

    function showAutoBanner(entry: LogEntry): void {
        // Operation entries show control + comment; pure
        // comments (no control) still toast the comment text so
        // the operator hears them — there's just no control to
        // jump to.
        if (entry.kind !== "operation" && entry.kind !== "comment") return;
        if (entry.kind === "operation" && !entry.control) return;
        if (entry.kind === "comment" && !entry.comment) return;
        // SingleStrip ownership: getAlertStrip returns the one
        // page-level strip; a new alert replaces the old text.
        const strip = getAlertStrip();
        strip.hidden = false;
        strip.innerHTML = "";
        const msg = document.createElement("span");
        msg.className = "alert-strip-text";
        const what = entry.kind === "comment"
            ? (entry.comment ?? "")
            : entry.comment
                ? `${entry.control} = ${entry.value} — ${entry.comment}`
                : `${entry.control} = ${entry.value}`;
        const at = `#${entry.seq} @ ${(entry.atMs / 1000).toFixed(1)}s`;
        msg.textContent = `Control Update ${at}: ${what}`;
        msg.title = what;
        const dismiss = document.createElement("button");
        dismiss.className = "btn";
        dismiss.textContent = "×";
        dismiss.setAttribute("aria-label", "Dismiss notification");
        dismiss.addEventListener("click", hideAlertStrip);
        strip.append(msg, dismiss);
    }

    input.addEventListener("input", () => {
        postBtn.disabled = !writable || input.value.trim() === "";
    });

    async function post(): Promise<void> {
        const comment = input.value.trim();
        if (!writable || comment === "") return;
        postBtn.disabled = true;
        errorLine.textContent = "";
        try {
            const res = await client.postLogComment(comment);
            ownSeq = Math.max(ownSeq, res.seq);
            input.value = "";
            postBtn.disabled = true;
        } catch (err) {
            const msg = err instanceof RemoteError ? err.message : String(err);
            errorLine.textContent = msg;
            postBtn.disabled = input.value.trim() === "";
        }
    }

    postBtn.addEventListener("click", () => { void post(); });
    input.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter") {
            ev.preventDefault();
            void post();
        }
    });

    return {
        append,
        noteForeign,
        setWritable(next: boolean): void {
            writable = next;
            postBtn.disabled = !next || input.value.trim() === "";
            input.disabled = !next;
        },
        onOwnSeq(seq: number): void {
            ownSeq = Math.max(ownSeq, seq);
        },
        selected(): LogEntry | null {
            return selectedEntry;
        },
        select(entry: LogEntry | null): void {
            applySelection(entry);
        },
        isScrollLockoutActive(): boolean {
            return performance.now() - lastViewScrollAt < SCROLL_LOCKOUT_MS;
        },
        follow(): boolean {
            return followOn;
        },
        setFollow(on: boolean): void {
            followOn = on;
            followCb.checked = on;
            try {
                localStorage.setItem(FOLLOW_KEY, on ? "1" : "0");
            } catch {
                // See change handler.
            }
            if (!on) {
                hideAlertStrip();
                // Unchecking follow clears the live selection too:
                // the toast jumping away with no red box would
                // otherwise leave the control outlined forever.
                if (selectedEntry !== null) {
                    applySelection(null);
                }
            }
        },
        autoSelect(entry: LogEntry): void {
            // Same path as a row click: row highlight + control
            // outline via onSelect, with click-again/Esc to
            // dismiss.
            applySelection(entry);
        },
        showAutoBanner,
    };
}
