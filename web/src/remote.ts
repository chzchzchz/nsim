// remote.ts — HTTP client for nsimserve (cmd/nsimserve).
// Every method maps 1:1 to a route registered in
// handlers.go:buildRoutes; the only server knowledge here
// is the uniform {"error","code"} envelope and percent-encoded
// keys (slashes become %2F for the {key...} wildcard routes).

export interface ObjectiveView {
    active: boolean;
    id?: string;
    title?: string;
    done: boolean;
    metMs?: number;
    detail?: string;
}

export interface StatusView {
    siteName: string;
    runId: number;
    simTimeMs: number;
    timestepMs: number;
    stepsPerSecond: number;
    running: boolean;
    paused: boolean;
    rev: number;
    latestSeq: number;
    objective: ObjectiveView;
}

export interface CatalogEntry {
    key: string;
    part: string;
    label: string;
    kind: string;
    enum?: string[];
    value: string;
    simTimeMs?: number;
}

export interface SensorEntry extends CatalogEntry {
    simTimeMs: number;
}

export interface ControlEntry extends CatalogEntry {}

// GET /alarms wires core.Alarm raw (Go field names, Severity
// as uint8 0|1|2, Timestamp as int64 nanoseconds). The
// web/src/alarms.ts panel wants severity words + time_s.
// remote.ts maps once at this boundary; callers see the
// panel shape.
export interface AlarmEntryRemote {
    id: string;
    description: string;
    severity: "info" | "warning" | "trip";
    active: boolean;
    acknowledged: boolean;
    time_s: number;
}

interface AlarmWire {
    ID: string;
    Description: string;
    Severity: number;
    Active: boolean;
    Acknowledged: boolean;
    Timestamp: number;
}

export function toAlarmPanelShape(wire: AlarmWire): AlarmEntryRemote {
    return {
        id: wire.ID,
        description: wire.Description,
        severity:
            wire.Severity === 1 ? "warning"
            : wire.Severity === 2 ? "trip"
            : "info",
        active: wire.Active,
        acknowledged: wire.Acknowledged,
        time_s: wire.Timestamp / 1e9,
    };
}

export interface QueueEntry {
    seq: number;
    text?: string;
    control: string;
    value: string;
    offsetMs: number;
    dueMs: number;
    status: string;
    err?: string;
}

export interface CommandsView {
    rev: number;
    entries: QueueEntry[];
}

export interface CommandsExport {
    scenario: string;
    commands: Array<{
        offsetMs?: number;
        atMs?: number;
        control: string;
        value: string;
        comment?: string;
    }>;
}

export interface LogEntry {
    seq: number;
    kind: string;
    atMs: number;
    control?: string;
    value?: string;
    comment?: string;
    source?: string;
}

export interface ScenarioInfo {
    id: string;
    number: number;
    category: string;
    title: string;
    startState: string;
    objective: string;
    durationHint?: string;
}

const TIMEOUT_MS = 2000;

export class RemoteError extends Error {
    code: number;
    constructor(message: string, code: number) {
        super(message);
        this.code = code;
    }
}

// normalizeBase validates a user-supplied server URL:
// trims whitespace, drops ONE trailing slash, requires
// http:// or https://. Returns the cleaned base or an error
// message for the dialog to show inline.
export function normalizeBase(raw: string): { base?: string; error?: string } {
    const trimmed = raw.trim();
    // Strip all trailing slashes so base never ends with '/';
    // url() joins with exactly one '/'.
    const base = trimmed.replace(/\/+$/, "");
    if (base === "") return { error: "enter a server URL" };
    let url: URL;
    try {
        url = new URL(base);
    } catch {
        return { error: `"${trimmed}" is not a valid URL` };
    }
    if (url.protocol !== "http:" && url.protocol !== "https:") {
        return { error: `"${trimmed}" must start with http:// or https://` };
    }
    return { base };
}

export class RemoteClient {
    base: string;

    constructor(base: string) {
        this.base = base;
    }

    private url(path: string): string {
        // base is normalized to have no trailing slash; join
        // with exactly one '/'. Path prefix is preserved.
        return `${this.base}${path}`;
    }

    private async request(
        path: string,
        init: RequestInit,
    ): Promise<Response> {
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), TIMEOUT_MS);
        try {
            const res = await fetch(this.url(path), {
                ...init,
                signal: controller.signal,
            });
            return res;
        } catch (err) {
            if (err instanceof DOMException && err.name === "AbortError") {
                throw new RemoteError("request timed out after 2 s", 0);
            }
            const msg = err instanceof Error ? err.message : String(err);
            throw new RemoteError(`cannot reach ${this.url(path)}: ${msg}`, 0);
        } finally {
            clearTimeout(timer);
        }
    }

    private async json<T>(path: string, init: RequestInit = {}): Promise<T> {
        const res = await this.request(path, {
            headers: { "Content-Type": "application/json" },
            ...init,
        });
        const text = await res.text();
        let body: unknown = null;
        try {
            body = JSON.parse(text);
        } catch {
            body = null;
        }
        if (!res.ok) {
            if (body !== null && typeof body === "object" && "error" in body) {
                const msg = String((body as { error: unknown }).error);
                throw new RemoteError(msg, res.status);
            }
            throw new RemoteError(
                `${res.status} ${res.statusText} from ${path}`,
                res.status,
            );
        }
        return body as T;
    }

    // Health probe: healthy = 200 with {"ok":true}. Used by
    // the Server dialog; re-probed after 3 failed polls.
    async livez(): Promise<boolean> {
        try {
            const body = await this.json<{ ok: boolean }>("/livez", {
                method: "GET",
            });
            return body.ok === true;
        } catch {
            return false;
        }
    }

    // Readiness: true while stepping, false during /sim/reset.
    async readyz(): Promise<boolean> {
        try {
            const body = await this.json<{ ready: boolean }>("/readyz", {
                method: "GET",
            });
            return body.ready === true;
        } catch {
            return false;
        }
    }

    status(): Promise<StatusView> {
        return this.json<StatusView>("/status");
    }

    sensors(): Promise<SensorEntry[]> {
        return this.json<SensorEntry[]>("/sensors");
    }

    controls(): Promise<ControlEntry[]> {
        return this.json<ControlEntry[]>("/controls");
    }

    async alarms(): Promise<AlarmEntryRemote[]> {
        const wire = await this.json<AlarmWire[]>("/alarms");
        return wire.map(toAlarmPanelShape);
    }

    commands(): Promise<CommandsView> {
        return this.json<CommandsView>("/commands");
    }

    commandsExport(): Promise<CommandsExport> {
        return this.json<CommandsExport>("/commands/export");
    }

    logs(lo: number): Promise<LogEntry[]> {
        return this.json<LogEntry[]>(`/logs?lo=${encodeURIComponent(String(lo))}`);
    }

    scenarios(): Promise<ScenarioInfo[]> {
        return this.json<ScenarioInfo[]>("/sim/scenarios");
    }

    state(): Promise<unknown> {
        return this.json<unknown>("/state");
    }

    putControl(key: string, value: string, comment?: string): Promise<ControlEntry & { seq: number }> {
        return this.json<ControlEntry & { seq: number }>(
            `/controls/${encodeURIComponent(key)}`,
            {
                method: "PUT",
                body: JSON.stringify(
                    comment !== undefined
                        ? { value, comment }
                        : { value },
                ),
            },
        );
    }

    submitCommandsText(text: string): Promise<unknown> {
        return this.json("/commands", {
            method: "POST",
            headers: { "Content-Type": "text/plain" },
            body: text,
        });
    }

    postLogComment(comment: string): Promise<{ seq: number; comment: string; atMs: number }> {
        return this.json("/logs", {
            method: "POST",
            body: JSON.stringify({ comment }),
        });
    }

    pause(): Promise<{ paused: boolean }> {
        return this.json("/sim/pause", { method: "POST", body: "{}" });
    }

    resume(): Promise<{ paused: boolean }> {
        return this.json("/sim/resume", { method: "POST", body: "{}" });
    }

    step(): Promise<{ steps: number }> {
        return this.json("/sim/step", { method: "POST", body: "{}" });
    }

    reset(body: { scenario?: string; plant?: string } = {}): Promise<{ runId: number }> {
        return this.json("/sim/reset", {
            method: "POST",
            body: JSON.stringify(body),
        });
    }

    speed(stepsPerSecond: number): Promise<{ stepsPerSecond: number }> {
        return this.json("/sim/speed", {
            method: "POST",
            body: JSON.stringify({ stepsPerSecond }),
        });
    }
}
