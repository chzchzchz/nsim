// alarms.dom.test.ts — alarm panel refresh behavior under happy-dom:
// stable rows survive unchanged, changed rows rebuild, cleared rows
// disappear, and the eyes button acknowledges through the bridge.

// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { renderAlarms, type AlarmEntry } from "./alarms";

const entry = (over: Partial<AlarmEntry> = {}): AlarmEntry => ({
    id: "Plant/HighPressure",
    description: "pressure high",
    severity: "warning",
    active: true,
    acknowledged: false,
    time_s: 10,
    ...over,
});

function mountPanel(initial: AlarmEntry[]) {
    const host = document.createElement("div");
    document.body.appendChild(host);
    const sets: [string, string][] = [];
    const panel = renderAlarms(
        host,
        {
            sensorJSON: () => "",
            setControl: (id, value) => {
                sets.push([id, value]);
                return null;
            },
        },
        initial,
    );
    return { host, panel, sets };
}

describe("renderAlarms", () => {
    it("keeps identical snapshots and rebuilds only changed rows", () => {
        const { host, panel } = mountPanel([entry()]);
        const badgeBefore = host.querySelector(".sev-badge");
        expect(badgeBefore).not.toBeNull();

        panel.refresh(JSON.stringify([entry()]));
        expect(host.querySelector(".sev-badge")).toBe(badgeBefore);

        panel.refresh(JSON.stringify([entry({ time_s: 12.5 })]));
        expect(host.querySelector(".sev-badge")).not.toBe(badgeBefore);
        expect(host.querySelector("tbody tr")!.textContent).toContain(
            "12.5 s",
        );
    });

    it("drops rows whose alarms cleared", () => {
        const { host, panel } = mountPanel([
            entry(),
            entry({ id: "Plant/LowLevel", severity: "trip" }),
        ]);
        expect(
            host.querySelectorAll("tbody tr:not(.alarm-detail)"),
        ).toHaveLength(2);

        panel.refresh(JSON.stringify([entry()]));
        expect(
            host.querySelectorAll("tbody tr:not(.alarm-detail)"),
        ).toHaveLength(1);
        expect(host.textContent).toContain("Plant/HighPressure");
        expect(host.textContent).not.toContain("Plant/LowLevel");
    });

    it("acknowledges through Alarms/Reset on the eyes button", () => {
        const { host, sets } = mountPanel([entry()]);
        (
            host.querySelector<HTMLButtonElement>("button.alarm-eyes")!
        ).click();
        expect(sets).toEqual([["Alarms/Reset", "Plant/HighPressure"]]);
    });

    it("disables the eyes button once acknowledged", () => {
        const { host, panel } = mountPanel([entry()]);
        const eyes = host.querySelector<HTMLButtonElement>(
            "button.alarm-eyes",
        )!;
        panel.refresh(JSON.stringify([entry({ acknowledged: true })]));
        expect(eyes.disabled).toBe(true);
        expect(
            host.querySelector("tbody tr")!.classList.contains("alarm-acked"),
        ).toBe(true);
    });
});
