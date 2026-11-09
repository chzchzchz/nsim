// serverPicker.ts — Server dialog: ask for an nsimserve URL,
// probe GET /livez (health) and GET /readyz, then connect or
// disconnect. Reuses the overlay idiom of the scenario
// picker (scenarios.ts) so errors render inline and the
// dialog never half-switches the app into remote mode.

import { RemoteClient, normalizeBase } from "./remote";

export interface ServerPickerCallbacks {
    onConnected(base: string): void;
    onDisconnected(): void;
}

const STORAGE_KEY = "nsim.remote.baseUrl";
const DEFAULT_URL = "http://127.0.0.1:8080";

export function lastBaseUrl(): string | null {
    try {
        return localStorage.getItem(STORAGE_KEY);
    } catch {
        return null;
    }
}

export function rememberBaseUrl(base: string): void {
    try {
        localStorage.setItem(STORAGE_KEY, base);
    } catch {
        // Private browsing / storage disabled: next open
        // falls back to the default.
    }
}

export interface ServerPicker {
    close(): void;
}

export function renderServerPicker(
    host: HTMLElement,
    currentBase: string | null,
    callbacks: ServerPickerCallbacks,
): ServerPicker {
    const backdrop = document.createElement("div");
    backdrop.className = "overlay-backdrop";
    const panel = document.createElement("div");
    panel.className = "scenario-picker server-picker";
    backdrop.appendChild(panel);
    host.appendChild(backdrop);

    const close = (): void => { backdrop.remove(); };

    const header = document.createElement("header");
    const h2 = document.createElement("h2");
    h2.textContent = "Connect to nsimserve";
    header.appendChild(h2);
    panel.appendChild(header);

    const body = document.createElement("div");
    body.className = "picker-body";
    panel.appendChild(body);

    const label = document.createElement("label");
    label.className = "server-url-label";
    label.textContent = "Server URL";
    label.htmlFor = "server-url-input";
    body.appendChild(label);

    const input = document.createElement("input");
    input.id = "server-url-input";
    input.type = "url";
    input.value = currentBase ?? lastBaseUrl() ?? DEFAULT_URL;
    input.spellcheck = false;
    input.style.width = "100%";
    input.style.padding = "6px 8px";
    body.appendChild(input);

    const errorLine = document.createElement("div");
    errorLine.className = "server-error";
    errorLine.setAttribute("role", "alert");
    body.appendChild(errorLine);

    const actions = document.createElement("div");
    actions.className = "scenario-actions";
    panel.appendChild(actions);

    if (currentBase !== null) {
        const disconnectBtn = document.createElement("button");
        disconnectBtn.textContent = "Disconnect";
        disconnectBtn.addEventListener("click", () => {
            close();
            callbacks.onDisconnected();
        });
        actions.appendChild(disconnectBtn);

        const spacer = document.createElement("span");
        spacer.style.flex = "1";
        actions.appendChild(spacer);
    }

    const cancelBtn = document.createElement("button");
    cancelBtn.textContent = "Cancel";
    cancelBtn.addEventListener("click", close);
    actions.appendChild(cancelBtn);

    const connectBtn = document.createElement("button");
    connectBtn.className = "primary";
    connectBtn.textContent = "Connect";
    actions.appendChild(connectBtn);

    let submitting = false;

    async function submit(): Promise<void> {
        if (submitting) return;
        const { base, error } = normalizeBase(input.value);
        if (error !== undefined || base === undefined) {
            errorLine.textContent = error ?? "invalid URL";
            return;
        }
        submitting = true;
        connectBtn.disabled = true;
        input.disabled = true;
        connectBtn.textContent = "Checking…";
        errorLine.textContent = "";
        const client = new RemoteClient(base);
        const live = await client.livez();
        if (!live) {
            errorLine.textContent =
                `no nsimserve at ${base}/livez — is the server running?`;
            submitting = false;
            connectBtn.disabled = false;
            input.disabled = false;
            connectBtn.textContent = "Connect";
            return;
        }
        const ready = await client.readyz();
        if (!ready) {
            errorLine.textContent =
                "server is up but rebuilding the plant (not ready)";
            submitting = false;
            connectBtn.disabled = false;
            input.disabled = false;
            connectBtn.textContent = "Connect";
            return;
        }
        rememberBaseUrl(base);
        close();
        callbacks.onConnected(base);
    }

    connectBtn.addEventListener("click", () => { void submit(); });
    input.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter") {
            ev.preventDefault();
            void submit();
        }
    });

    // Focus the URL field so the operator can edit it
    // immediately; select the host part.
    input.focus();
    input.select();

    return { close };
}
