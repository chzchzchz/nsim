// units.test.ts — display metadata and numeric formatting policy.

import { describe, expect, it } from "vitest";
import {
    formatSensorValue,
    knownKind,
    rangeFor,
    unitFor,
} from "./units";

describe("formatSensorValue", () => {
    it("caps numeric kinds at two decimals", () => {
        expect(formatSensorValue("psig", "2250")).toBe("2250.00");
        expect(formatSensorValue("percent", "12.34567")).toBe("12.35");
        expect(formatSensorValue("signedpercent", "-4.36")).toBe("-4.36");
        expect(formatSensorValue("fahrenheit", "-1.2")).toBe("-1.20");
    });

    it("passes non-numeric kinds through unchanged", () => {
        expect(formatSensorValue("bool", "1")).toBe("1");
        expect(formatSensorValue("enum", "Running")).toBe("Running");
        expect(formatSensorValue("string", "hello")).toBe("hello");
    });

    it("lets NaN/Infinity through so raw engine state stays visible", () => {
        expect(formatSensorValue("watt", "NaN")).toBe("NaN");
        expect(formatSensorValue("hz", "Infinity")).toBe("Infinity");
    });
});

describe("unit metadata", () => {
    it("maps kinds to display units", () => {
        expect(unitFor("psig")).toBe("psig");
        expect(unitFor("fahrenheit")).toBe("°F");
        expect(unitFor("normalized")).toBe("");
    });

    it("exposes control ranges only where bounded", () => {
        expect(rangeFor("normalized")).toBe("[0, 1]");
        expect(rangeFor("percent")).toBe("[0, 100]");
        expect(rangeFor("pctpersec")).toBe("[0, 0.8]");
        expect(rangeFor("signedpercent")).toBeNull();
        expect(rangeFor("rpm")).toBeNull();
    });

    it("knows the taxonomy and rejects gaps", () => {
        expect(knownKind("gpm")).toBe(true);
        expect(knownKind("pctpersec")).toBe(true);
        expect(knownKind("alarm")).toBe(true);
        expect(knownKind("furlongs")).toBe(false);
    });
});
