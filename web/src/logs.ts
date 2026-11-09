// logs.ts — Logs panel: live audit trail from GET /logs
// with incremental ?lo= polling, a comment-only composer
// (POST /logs), and a small "N new entries" banner driven
// by /status.latestSeq. Remote-mode only.

import type { LogEntry, RemoteClient } from "./remote";
import { RemoteError } from "./remote";

const MAX_ROWS = 200;

export interface LogsPanel {
    // Append newly fetched entries and advance the cursor.
    append(entries: LogEntry[]): void;
    // Entries that look like another client acted: seqs above
    // the caller's last seen that aren't this browser's own.
    noteForeign(count: number, latest: LogEntry | undefined): void;
    setWritable(writable: boolean): void;
    clear(): void;
    onOwnSeq(seq: number): void;
}

export function initLogs(
    hostEl: HTMLElement,
    client: RemoteClient,
): LogsPanel {
    hostEl.innerHTML = "";

    const banner = document.createElement("div");
    banner.className = "logs-banner";
    banner.hidden = true;
    hostEl.appendChild(banner);

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
            if (e.source === "script") {
                row.title = "scenario script";
            }
            listEl.appendChild(row);
        }
        while (listEl.children.length > MAX_ROWS) {
            listEl.removeChild(listEl.firstChild!);
        }
        listEl.scrollTop = listEl.scrollHeight;
    }

    function noteForeign(count: number, latest: LogEntry | undefined): void {
        if (count <= 0) return;
        bannerCount += count;
        if (latest !== undefined) bannerLatest = latest;
        renderBanner();
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
        clear(): void {
            lastSeq = -1;
            ownSeq = -1;
            bannerCount = 0;
            bannerLatest = undefined;
            listEl.replaceChildren();
            renderBanner();
        },
        onOwnSeq(seq: number): void {
            ownSeq = Math.max(ownSeq, seq);
        },
    };
}
