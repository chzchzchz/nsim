// remoteController.ts — remote-mode controller for main.ts.
//
// Owns all remote connection state (client, connected, writable,
// polling generation, log cursor, status snapshot) so it can be
// constructed before any wiring code runs. Keeping the state here
// removes the TDZ class of bug where synchronously-invoked
// scenario wiring read not-yet-initialized `let remote*`
// declarations from the main() body: main.ts instantiates this
// FIRST, then every consumer reads `remote.*`.
//
// The controller only talks to the server + renders remote tiles
// into the provided hosts. Local rebuild callbacks (rebuildTiles,
// initAlarms, initConsolePanel) are injected as onDisconnected
// hooks so disconnect restores local mode exactly.

import { RemoteClient, RemoteError } from "./remote";
import type {
    AlarmEntryRemote,
    ControlEntry,
    LogEntry,
    SensorEntry,
    StatusView,
} from "./remote";
import type { SensorRow } from "./sensors";
import type { LogsPanel } from "./logs";
import { initLogs } from "./logs";
import { loadPanelLayout } from "./layout";
import { applyMinimized, buildTile } from "./tile";
import { plantFromJSON } from "./plant";
import { mountPlant } from "./svg";
import { displayPart, groupByPrefix, rememberReturn } from "./bindings";
import {
    buildRemoteBindings,
    renderRemotePanel,
    renderRemoteReadout,
    refreshRemoteControls,
    refreshRemoteSensors,
} from "./remoteTiles";
import { renderAlarms } from "./alarms";
import { confirmControlChange } from "./confirmChange";

export type RemoteMode = "local" | "readonly" | "writable";

export interface RemoteControllerHost {
    tilesEl: HTMLElement;
    alarmsEl: HTMLElement;
    consoleEl: HTMLElement;
    logsEl: HTMLElement;
    statusEl: HTMLElement;
    startBtn: HTMLButtonElement;
    stepBtn: HTMLButtonElement;
    resetBtn: HTMLButtonElement;
    spsInput: HTMLInputElement;
    fpsInput: HTMLInputElement;
    spsEl: HTMLElement;
    writeModeBtn: HTMLButtonElement;
    pidEl: HTMLElement;
}

export interface RemoteControllerHooks {
    // Called after disconnect completes so main.ts can restore the
    // local dashboard (tiles/alarms/console) exactly as before.
    onDisconnected?: () => void;
    scenarioNameEl?: HTMLElement;
}



export class RemoteController {
    client: RemoteClient | null = null;
    mode: RemoteMode = "local";
    lastStatus: StatusView | null = null;
    lastRunId = -1;
    lastSeq = -1;
    ownSeq = -1;
    failures = 0;
    sensorRows: SensorRow[] = [];
    logsPanel: LogsPanel | null = null;
    alarmPanel: { refresh(json: string): void } | null = null;
    // Last rendered SVG text so the 1 Hz refresh swaps only when
    // the diagram actually changed (same rule as main.ts local).
    lastSvgHtml = "";
    // Last simTimeMs for which an SVG refresh ran; gates the
    // /state fetch so we do not pull the full plant JSON on ticks
    // where simulated time did not move (paused idle).
    lastSvgSimTimeMs = -1;
    // Bridge id of the control row currently outlined because
    // its log entry is selected; null when nothing is selected.
    // Kept (not just the DOM class) so the outline can be
    // re-applied after tiles are rebuilt on a runId change.
    selectedLogControl: string | null = null;

    private pollTimer: ReturnType<typeof setTimeout> | null = null;
    private pollGen = 0;
    private pollInFlight = false;
    // Effective poll interval in ms. Defaults to 1000 ms (1 Hz)
    // but follows the header's Frames/s box via setFramesPerSecond,
    // hard-capped at 1 fps so a remote poll never spins faster
    // than 1 Hz regardless of what the box says (the box serves
    // the local WASM tick at 1–240 fps).
    private pollMs = 1000;

    constructor(
        private host: RemoteControllerHost,
        private hooks: RemoteControllerHooks,
        pollMs = 1000,
    ) {
        this.pollMs = RemoteController.pollMsFor(pollMs);
    }

    // pollMsFor clamps a poll interval to the 1 Hz hard cap.
    static pollMsFor(ms: number): number {
        return ms < 1000 ? 1000 : ms;
    }

    // setFramesPerSecond applies the header Frames/s value to the
    // remote poll: 1000/fps ms, capped at 1 fps (1000 ms). Values
    // ≥ 1 fps (including the default 60) all collapse to 1 Hz.
    setFramesPerSecond(fps: number): void {
        const ms = fps > 0 ? 1000 / fps : 1000;
        this.pollMs = RemoteController.pollMsFor(ms);
        // Cancel the pending timer so the new interval takes
        // effect on the next scheduled tick. When not connected
        // pollTimer is always null, so this is a no-op there.
        if (this.pollTimer !== null) {
            clearTimeout(this.pollTimer);
            this.pollTimer = null;
            if (this.connected) {
                this.startPoll();
            }
        }
    }

    // pollIntervalMs exposes the effective interval for tests.
    pollIntervalMs(): number {
        return this.pollMs;
    }

    get connected(): boolean {
        return this.mode !== "local" && this.client !== null;
    }

    get writable(): boolean {
        return this.mode === "writable";
    }

    connect(base: string): void {
        this.client = new RemoteClient(base);
        this.mode = "readonly";
        this.lastRunId = -1;
        this.lastSeq = -1;
        this.ownSeq = -1;
        this.failures = 0;
        this.sensorRows = [];
        this.clearLogSelectionOutline();
        this.lastSvgHtml = "";
        this.lastSvgSimTimeMs = -1;
        this.host.spsEl.textContent = "steps/s: —";
        if (this.hooks.scenarioNameEl) this.hooks.scenarioNameEl.textContent = "";
        this.setVisible(true);
        // FPS box stays live in remote mode: it drives the poll
        // interval at 1000/fps ms, capped at 1 Hz. A value of 0.5
        // fps polls every 2 s.
        this.startPoll();
        this.applyWritable();
        this.host.statusEl.textContent = `connected: ${base}`;
    }

    disconnect(): void {
        this.mode = "local";
        this.client = null;
        this.lastStatus = null;
        this.lastRunId = -1;
        this.lastSeq = -1;
        this.ownSeq = -1;
        this.failures = 0;
        this.clearLogSelectionOutline();
        this.lastSvgHtml = "";
        this.lastSvgSimTimeMs = -1;
        this.stopPoll();
        this.logsPanel = null;
        this.setVisible(false);
        this.host.spsEl.textContent = "steps/s: —";
        // Hand control back to the local Sim so its own button
        // state is restored: reconnecting after a writable
        // session must not leave the header disabled, and local
        // Start/Step/Reset must not inherit the remote disabled
        // state. Re-enabling unconditionally is safe because the
        // local lifecycle has no writable gate; Sim.refreshButtons
        // then corrects stepBtn for the running state.
        this.host.startBtn.disabled = false;
        this.host.stepBtn.disabled = false;
        this.host.resetBtn.disabled = false;
        this.host.spsInput.disabled = false;
        this.hooks.onDisconnected?.();
    }

    toggleWritable(): void {
        if (this.mode === "local") return;
        this.mode = this.mode === "writable" ? "readonly" : "writable";
        this.applyWritable();
    }

    setVisible(visible: boolean): void {
        // The server button itself stays visible at all times (it
        // lives in main.ts) so the operator can connect; only the
        // remote-only UI toggles here.
        this.host.writeModeBtn.style.display =
            visible && this.connected ? "" : "none";
        this.host.logsEl.style.display = visible ? "" : "none";
        this.host.writeModeBtn.toggleAttribute(
            "hidden",
            !(visible && this.connected),
        );
        this.host.logsEl.toggleAttribute("hidden", !visible);
    }

    // Outline the control row for the selected log entry (red
    // border via .ctl-row.log-selected) and clear the previous
    // one. Toggled from the Logs panel click: selecting the same
    // entry again passes null and removes the outline. Scrolls
    // the control into view only for explicit user selections,
    // and not while the banner "View" scroll is in flight.
    onLogSelection(entry: LogEntry | null): void {
        this.selectedLogControl = entry?.control ?? null;
        const scroll = !this.logsPanel?.isScrollLockoutActive();
        this.applyLogSelectionOutline(scroll);
    }

    // Clear any selected control row; used on disconnect/connect
    // so a stale outline is removed without relying on tilesEl
    // being rebuilt by the onDisconnected hook.
    private clearLogSelectionOutline(): void {
        this.selectedLogControl = null;
        this.applyLogSelectionOutline(false);
    }

    private applyLogSelectionOutline(scroll: boolean): void {
        const rows = this.host.tilesEl.querySelectorAll<HTMLElement>(".ctl-row");
        for (const row of Array.from(rows)) {
            const isSelected =
                this.selectedLogControl !== null &&
                row.dataset.bridgeId === this.selectedLogControl;
            row.classList.toggle("log-selected", isSelected);
            if (isSelected && scroll) {
                // Minimized tiles hide .tile-body; scrolling to an
                // invisible row does nothing, so bail quietly and
                // let the existing outline be the feedback.
                const tile = row.closest<HTMLElement>(".tile.minimized");
                if (!tile) {
                    // Record the return slot so the backtick key
                    // takes the operator back to the log view.
                    // rememberReturn no-ops when the row is
                    // already on screen, matching flashRow.
                    rememberReturn(row);
                    // block: "center" puts the row in the middle
                    // of the viewport, so the sticky header can't
                    // obscure it.
                    row.scrollIntoView({ behavior: "smooth", block: "center" });
                }
            }
        }
    }

    applyWritable(): void {
        this.host.writeModeBtn.textContent = this.writable
            ? "✏️ Writable"
            : "🔒 Read-only";
        this.host.writeModeBtn.title = this.writable
            ? "Writable — control changes ask for a comment"
            : "Read-only — enable to change controls";
        this.logsPanel?.setWritable(this.writable);
        this.setWriteDisabled(!this.writable);
    }

    setWriteDisabled(disabled: boolean): void {
        const nodes = Array.from(
            this.host.tilesEl.querySelectorAll<HTMLElement>(
                "[data-bridge-id] input, [data-bridge-id] select, [data-bridge-id] button",
            ),
        );
        for (const el of nodes) {
            if (el.closest(".ctl-row")) {
                (el as HTMLInputElement).disabled = disabled;
            }
        }
        this.host.startBtn.disabled = disabled;
        this.host.stepBtn.disabled =
            disabled || !this.lastStatus?.paused;
        this.host.resetBtn.disabled = disabled;
        this.host.spsInput.disabled = disabled;
    }

    // ---- polling ----

    startPoll(): void {
        this.stopPoll();
        const gen = ++this.pollGen;
        const tickPoll = async (): Promise<void> => {
            if (gen !== this.pollGen || !this.connected) return;
            this.pollTimer = setTimeout(
                () => {
                    void tickPoll();
                },
                this.pollMs,
            );
            if (!this.client || !this.connected) return;
            try {
                await this.pollOnce();
                this.failures = 0;
            } catch (err) {
                this.failures++;
                const msg =
                    err instanceof RemoteError ? err.message : String(err);
                this.host.statusEl.textContent = `unreachable — retrying… (${msg})`;
                if (this.failures === 3 && this.client) {
                    const alive = await this.client.livez();
                    if (!alive) {
                        this.host.statusEl.textContent =
                            "server unreachable — retrying…";
                    }
                }
            }
        };
        void tickPoll();
    }

    stopPoll(): void {
        this.pollGen++;
        if (this.pollTimer !== null) {
            clearTimeout(this.pollTimer);
            this.pollTimer = null;
        }
    }

    // drainNow runs one poll immediately (tests + connect fast-path).
    async drainNow(): Promise<void> {
        await this.pollOnce();
    }

    async pollOnce(): Promise<void> {
        if (!this.client || this.pollInFlight) return;
        this.pollInFlight = true;
        const client = this.client;
        try {
            const status = await client.status();
            if (this.client !== client) return;
            this.lastStatus = status;

            const sec = (status.simTimeMs / 1000).toFixed(2);
            const state = status.paused ? "paused" : "running";
            this.host.statusEl.textContent =
                `${state} @ ${sec} s simulated — ${status.siteName}`;
            this.host.startBtn.textContent = status.paused
                ? "▶ Start"
                : "⏸ Pause";
            this.host.startBtn.disabled = !this.writable;
            this.host.stepBtn.disabled = !status.paused || !this.writable;
            this.host.resetBtn.disabled = !this.writable;
            this.host.spsInput.disabled = !this.writable;

            if (status.runId !== this.lastRunId) {
                this.lastRunId = status.runId;
                this.lastSeq = -1;
                this.ownSeq = -1;
                this.host.logsEl.innerHTML = "";
                this.selectedLogControl = null;
                this.logsPanel = initLogs(this.host.logsEl, client, (entry) =>
                    this.onLogSelection(entry),
                );
                this.logsPanel.setWritable(this.writable);
                await this.buildTiles();
                // A concurrent disconnect/connect while building
                // tiles may have swapped this.client; do not fetch
                // logs for a stale generation.
                if (this.client !== client || !this.connected) return;
                const all = await client.logs(0);
                if (this.client !== client) return;
                this.logsPanel.append(all);
                const last = all.length
                    ? all[all.length - 1].seq
                    : status.latestSeq;
                this.lastSeq = Math.max(status.latestSeq, last);
            }

            // Tolerate transient 5xx per snapshot; 401/403 mean the
            // session is no longer authorized and must surface
            // rather than render as "no data".
            const tolerant = async <T>(p: Promise<T>): Promise<T | null> => {
                try {
                    return await p;
                } catch (err) {
                    const code =
                        err instanceof RemoteError ? err.code : undefined;
                    if (code === 401 || code === 403) throw err;
                    return null;
                }
            };
            const [controls, sensors, alarms] = await Promise.all([
                tolerant(client.controls()),
                tolerant(client.sensors()),
                tolerant(client.alarms()),
            ]);
            if (this.client !== client) return;
            if (controls !== null) {
                const values: Record<string, string> = {};
                for (const c of controls) values[c.key] = c.value;
                refreshRemoteControls(this.host.tilesEl, values);
            }
            if (sensors !== null) {
                const values: Record<string, string> = {};
                for (const s of sensors) values[s.key] = s.value;
                refreshRemoteSensors(this.sensorRows, values);
            }
            if (alarms !== null && this.alarmPanel) {
                this.alarmPanel.refresh(JSON.stringify(alarms));
            }
            await this.pollLogs(client, status);
            // Remote P&ID: reconstruct the plant in the local wasm
            // and render the SVG client-side, the same renderer
            // local mode uses. Polling GET /state instead of /svg
            // avoids adding a server endpoint (see main.ts). Gated
            // on simulated time so paused/idle ticks don't pull the
            // full plant JSON and re-render an identical SVG.
            if (status.simTimeMs !== this.lastSvgSimTimeMs) {
                await this.refreshSvg(client);
                this.lastSvgSimTimeMs = status.simTimeMs;
            }
        } finally {
            this.pollInFlight = false;
        }
    }

    // refreshSvg fetches GET /state, reconstructs the plant in the
    // local wasm, renders the P&ID, and swaps only on change (the
    // 1 Hz diagram-poll rule in main.ts). Tolerates failure:
    // a transient /state error leaves the previous diagram in place.
    // A missing wasm export (e.g. a test stub or an older blob)
    // is also tolerated: remote mode still works without a P&ID.
    // A render failure is surfaced on the status line so a
    // persistent bug does not silently leave a stale diagram.
    async refreshSvg(client: RemoteClient): Promise<void> {
        let state: unknown;
        try {
            state = await client.state();
        } catch {
            return;
        }
        if (this.client !== client) return;
        let svg: string;
        try {
            if (typeof window === "undefined" || !window.nsimPlantFromJSON) {
                return;
            }
            const plant = plantFromJSON(JSON.stringify(state));
            const raw = plant.getSvg();
            svg = raw as string;
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            console.error("remote svg render failed:", err);
            this.host.statusEl.textContent = `P&ID render failed: ${msg}`;
            return;
        }
        if (typeof svg !== "string") return;
        if (svg === this.lastSvgHtml) return;
        mountPlant(this.host.pidEl, svg);
        this.lastSvgHtml = svg;
    }

    async pollLogs(client: RemoteClient, status: StatusView): Promise<void> {
        if (!this.logsPanel) return;
        if (status.latestSeq <= this.lastSeq) return;
        const entries = await client.logs(this.lastSeq + 1);
        if (this.client !== client) return;
        if (entries.length === 0) {
            this.lastSeq = status.latestSeq;
            return;
        }
        const threshold = Math.max(this.lastSeq, this.ownSeq);
        const foreign = entries.filter((e) => e.seq > threshold);
        this.logsPanel.append(entries);
        if (foreign.length > 0) {
            this.logsPanel.noteForeign(
                foreign.length,
                foreign[foreign.length - 1],
            );
            // Auto-follow: select the latest foreign entry so its
            // control jumps into view with the red box (same path
            // as a manual row click), and show the toast. A burst
            // batch collapses to the single latest entry — one
            // scroll, one rememberReturn, one toast — instead of
            // firing N of each.
            if (this.logsPanel.follow()) {
                const latest = foreign[foreign.length - 1];
                if (latest !== undefined) {
                    if (latest.control) {
                        this.logsPanel.autoSelect(latest);
                    }
                    this.logsPanel.showAutoBanner(latest);
                }
            }
        }
        const newest = entries[entries.length - 1].seq;
        this.lastSeq = Math.max(status.latestSeq, newest);
        this.ownSeq = Math.max(this.ownSeq, newest);
    }

    async buildTiles(): Promise<void> {
        if (!this.client) return;
        const client = this.client;
        const [controls, sensors] = await Promise.all([
            client.controls().catch(() => [] as ControlEntry[]),
            client.sensors().catch(() => [] as SensorEntry[]),
        ]);
        if (!this.client) return;
        const bindings = buildRemoteBindings(controls, sensors);
        const groups = groupByPrefix(bindings);
        const layout = loadPanelLayout(groups.map((g) => g.prefix));
        const ordered = layout
            ? layout
                .order.map((name) => groups.find((g) => g.prefix === name)!)
                .filter(Boolean)
            : groups;
        this.host.tilesEl.innerHTML = "";
        this.sensorRows = [];
        const controlValues: Record<string, string> = {};
        for (const c of controls) controlValues[c.key] = c.value;
        for (const group of ordered as ReturnType<typeof groupByPrefix>) {
            buildTile(
                this.host.tilesEl,
                group.prefix,
                (ctlHost) => renderRemotePanel(
                    ctlHost,
                    group.bindings,
                    displayPart(group.prefix),
                    controlValues,
                    () => this.writable,
                    async (key, value, comment) => {
                        const res = await client.putControl(key, value, comment);
                        this.ownSeq = Math.max(this.ownSeq, res.seq);
                        this.logsPanel?.onOwnSeq(res.seq);
                        return { seq: res.seq };
                    },
                ),
                (senHost) => renderRemoteReadout(
                    senHost,
                    group.bindings,
                    displayPart(group.prefix),
                ),
                this.sensorRows,
            );
        }
        if (layout) {
            const minimized = new Set(layout.minimized);
            for (const el of Array.from(
                this.host.tilesEl.children,
            ) as HTMLElement[]) {
                if (minimized.has(el.dataset.panel ?? "")) {
                    applyMinimized(el, true);
                }
            }
        }
        const sensorValues: Record<string, string> = {};
        for (const s of sensors) sensorValues[s.key] = s.value;
        refreshRemoteSensors(this.sensorRows, sensorValues);
        refreshRemoteControls(this.host.tilesEl, controlValues);
        // Tiles were just rebuilt, so re-apply any live log
        // selection outline to the fresh rows (without scrolling:
        // this is not a user selection). If the selected control
        // is no longer in the catalog (new run), reconcile by
        // clearing the log selection too, so the log row doesn't
        // claim a control that doesn't exist.
        const hadSelection = this.selectedLogControl !== null;
        this.applyLogSelectionOutline(false);
        if (
            hadSelection &&
            this.selectedLogControl !== null &&
            !this.host.tilesEl.querySelector<HTMLElement>(
                `.ctl-row.log-selected[data-bridge-id="${CSS.escape(this.selectedLogControl)}"]`,
            )
        ) {
            this.logsPanel?.select(null);
        }

        this.host.alarmsEl.innerHTML = "";
        const alarms = await client
            .alarms()
            .catch(() => [] as AlarmEntryRemote[]);
        const shim = {
            sensorJSON: (_id: string) => JSON.stringify(alarms),
            setControl: (id: string, value: string): Error | null => {
                if (!this.writable) return new Error("read-only");
                void (async () => {
                    const dialog = await confirmControlChange(
                        id,
                        "(current)",
                        value,
                    );
                    if (!dialog.submitted || dialog.comment === undefined) return;
                    const res = await client.putControl(
                        id,
                        value,
                        dialog.comment,
                    );
                    this.ownSeq = Math.max(this.ownSeq, res.seq);
                    this.logsPanel?.onOwnSeq(res.seq);
                })().catch((err) => console.error(err));
                return null;
            },
        };
        this.alarmPanel = renderAlarms(this.host.alarmsEl, shim as never, alarms);

        this.host.consoleEl.innerHTML = "";
        const consoleSubmit = document.createElement("button");
        consoleSubmit.className = "btn";
        consoleSubmit.textContent = "Submit";
        consoleSubmit.id = "btn-cmd-submit";
        const consoleText = document.createElement("textarea");
        consoleText.id = "cmd-input";
        consoleText.rows = 3;
        consoleText.spellcheck = false;
        consoleText.placeholder = "TimeOffsetMs Part.Label Value  (one per line)";
        const consoleList = document.createElement("div");
        consoleList.id = "cmd-list";
        const consoleError = document.createElement("div");
        consoleError.id = "cmd-error";
        this.host.consoleEl.append(
            consoleText,
            consoleSubmit,
            consoleList,
            consoleError,
        );
        consoleSubmit.disabled = !this.writable;
        consoleSubmit.addEventListener("click", () => {
            const text = consoleText.value.trim();
            if (!text) return;
            void client
                .submitCommandsText(text)
                .then(() => {
                    consoleText.value = "";
                    consoleError.textContent = "";
                })
                .catch((err) => {
                    consoleError.textContent =
                        err instanceof RemoteError ? err.message : String(err);
                });
        });
    }

    // ---- lifecycle pass-throughs (called from main.ts handlers) ----

    async togglePauseResume(): Promise<void> {
        if (!this.client || !this.connected) return;
        try {
            if (this.lastStatus?.paused) await this.client.resume();
            else await this.client.pause();
        } catch (err) {
            console.error("pause/resume failed:", err);
        }
    }

    async step(): Promise<void> {
        if (!this.client) return;
        if (this.lastStatus && !this.lastStatus.paused) return;
        try {
            await this.client.step();
        } catch (err) {
            console.error(err);
        }
    }

    async reset(): Promise<void> {
        if (!this.client) return;
        try {
            await this.client.reset({});
            this.lastRunId = -1;
        } catch (err) {
            console.error(err);
        }
    }

    async setSpeed(n: number): Promise<void> {
        if (!this.client) return;
        try {
            await this.client.speed(n);
        } catch (err) {
            console.error(err);
        }
    }

    async applyScenario(id: string | null): Promise<boolean> {
        if (!this.client) return false;
        if (!this.writable) return false;
        if (id === null) return false;
        try {
            await this.client.reset({ scenario: id });
            this.lastRunId = -1;
            return true;
        } catch (err) {
            console.error("reset failed:", err);
            return false;
        }
    }
}
