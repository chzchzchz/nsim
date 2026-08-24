// units.ts — display metadata for the bridge Kind taxonomy.
//
// The Kind strings ("bool", "psig", "fahrenheit", …) and their
// parsing/validation rules live in bridge/convert.go (Go side).
// The TypeScript side only needs to render values and labels, so
// this file mirrors just the displayable subset.
//
// Keep the entries in sync with the Kind switch in
// bridge/convert.go.

export interface UnitInfo {
    // Short label appended to a value (e.g. "2250 psig"). Empty
    // when the kind is dimensionless (e.g. "normalized").
    unit: string;
    // Range string for control inputs (e.g. "[0, 1]"). Null when
    // the kind is unbounded or has no numeric range.
    range: string | null;
    // Whether sensor values of this kind render as floating-point
    // numbers. False for bool/enum/string kinds.
    numeric: boolean;
}

export const UNITS: Record<string, UnitInfo> = {
    bool:        { unit: "",     range: null,    numeric: false },
    normalized:  { unit: "",     range: "[0, 1]",   numeric: true },
    percent:     { unit: "%",    range: "[0, 100]", numeric: true },
    signedpercent: { unit: "%",  range: null,    numeric: true },
    rpm:         { unit: "rpm",  range: null,    numeric: true },
    psig:        { unit: "psig", range: null,    numeric: true },
    psia:        { unit: "psia", range: null,    numeric: true },
    fahrenheit:  { unit: "°F",   range: null,    numeric: true },
    gallon:      { unit: "gal",  range: null,    numeric: true },
    gpm:         { unit: "gpm",  range: null,    numeric: true },
    watt:        { unit: "W",    range: null,    numeric: true },
    ppm:         { unit: "ppm",  range: null,    numeric: true },
    volt:        { unit: "V",    range: null,    numeric: true },
    hz:          { unit: "Hz",   range: null,    numeric: true },
    string:      { unit: "",     range: null,    numeric: false },
    enum:        { unit: "",     range: null,    numeric: false },
    alarm:       { unit: "",     range: null,    numeric: false },
};

export function unitFor(kind: string): string {
    return UNITS[kind]?.unit ?? "";
}

export function rangeFor(kind: string): string | null {
    return UNITS[kind]?.range ?? null;
}

// knownKind reports whether the display table covers a bridge
// Kind; bindings.ts warns on gaps so drift from the Go taxonomy
// in bridge/convert.go surfaces instead of rendering silently
// wrong.
export function knownKind(kind: string): boolean {
    return kind in UNITS;
}

// formatSensorValue caps a numeric sensor value at two decimal
// places. Non-numeric kinds (bool/enum/string) pass through
// unchanged. The Go side already produces the raw string via
// fmt.Sprintf("%v", f); we re-parse and reformat here so we can
// apply a consistent precision policy without changing the bridge
// contract. NaN/Infinity pass through unchanged so the user sees
// the underlying engine state rather than a misleading "NaN.00".
export function formatSensorValue(kind: string, raw: string): string {
    if (!UNITS[kind]?.numeric) {
        return raw;
    }
    const f = Number(raw);
    if (!Number.isFinite(f)) {
        return raw;
    }
    return f.toFixed(2);
}
