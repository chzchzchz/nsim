// confirmChange.ts — writable-mode confirm dialog.
// Every individual control change opens this before a PUT
// is sent: it shows control old -> new, requires a comment,
// and Cancel sends nothing. One dialog at a time.

export interface ConfirmResult {
    submitted: boolean;
    comment?: string;
}

let open = false;

export function confirmControlChange(
    key: string,
    oldValue: string,
    newValue: string,
): Promise<ConfirmResult> {
    if (open) {
        // One dialog at a time; a second change while one is
        // open is ignored rather than stacked.
        return Promise.resolve({ submitted: false });
    }
    open = true;
    return new Promise<ConfirmResult>((resolve) => {
        const backdrop = document.createElement("div");
        backdrop.className = "overlay-backdrop";
        const panel = document.createElement("div");
        panel.className = "scenario-picker server-picker";
        backdrop.appendChild(panel);
        document.body.appendChild(backdrop);

        const finish = (result: ConfirmResult): void => {
            open = false;
            backdrop.remove();
            resolve(result);
        };

        const header = document.createElement("header");
        const h2 = document.createElement("h2");
        h2.textContent = "Confirm control change";
        header.appendChild(h2);
        panel.appendChild(header);

        const body = document.createElement("div");
        body.className = "picker-body";
        panel.appendChild(body);

        const change = document.createElement("div");
        change.className = "server-change";
        change.textContent = `${key}: ${oldValue || "∅"} → ${newValue}`;
        body.appendChild(change);

        const label = document.createElement("label");
        label.className = "server-url-label";
        label.htmlFor = "confirm-comment-input";
        label.textContent = "Comment (required)";
        body.appendChild(label);

        const input = document.createElement("input");
        input.id = "confirm-comment-input";
        input.type = "text";
        input.spellcheck = false;
        input.placeholder = "why this change?";
        input.style.width = "100%";
        input.style.padding = "6px 8px";
        body.appendChild(input);

        const actions = document.createElement("div");
        actions.className = "scenario-actions";
        panel.appendChild(actions);

        const cancelBtn = document.createElement("button");
        cancelBtn.textContent = "Cancel";
        cancelBtn.addEventListener("click", () => finish({ submitted: false }));
        actions.appendChild(cancelBtn);

        const submitBtn = document.createElement("button");
        submitBtn.className = "primary";
        submitBtn.textContent = "Submit";
        submitBtn.disabled = true;
        actions.appendChild(submitBtn);

        input.addEventListener("input", () => {
            submitBtn.disabled = input.value.trim() === "";
        });
        submitBtn.addEventListener("click", () => {
            if (input.value.trim() === "") return;
            finish({ submitted: true, comment: input.value.trim() });
        });
        input.addEventListener("keydown", (ev) => {
            if (ev.key === "Enter") {
                ev.preventDefault();
                if (!submitBtn.disabled) submitBtn.click();
            }
        });

        input.focus();
    });
}
