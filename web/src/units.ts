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
    mwh:         { unit: "MWh",  range: null,    numeric: true },
    mw:          { unit: "MW",   range: null,    numeric: true },
    ppm:         { unit: "ppm",  range: null,    numeric: true },
    volt:        { unit: "V",    range: null,    numeric: true },
    hz:          { unit: "Hz",   range: null,    numeric: true },
    // Source-range neutron detector count rate (counts per
    // second); huge magnitudes are normal, so the two-decimal
    // sensor formatting must not fall back to raw pass-through.
    cps:         { unit: "cps",  range: null,    numeric: true },
    // Startup rate in decades per minute; signed because a
    // subcritical core reads a negative rate.
    dpm:         { unit: "dpm",  range: null,    numeric: true },
    // Demanded reactor power ramp rate in percent of rated power
    // per second; the model caps acceptance at 0.8 %/s.
    pctpersec:   { unit: "%/s",  range: "[0, 0.8]", numeric: true },
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

// formatSensorValue keeps small readings at two decimals and
// switches to four significant figures once |v| >= 10000, so
// huge magnitudes (e.g. source-range counts) never produce a
// dozen characters of false precision. Large values keep the
// plain fixed form — "55150", not "5.515e4" — so magnitude is
// scannable while the tile stays fit; only beyond six integer
// digits does it fall back to exponent form, written without a
// leading plus ("9.375e11") to save yet another character.
// Non-numeric kinds (bool/enum/string) pass through unchanged.
// The Go side already produces the raw string via
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
    if (Math.abs(f) < 10000) {
        return f.toFixed(2);
    }
    // Four significant figures. Between 10000 and ~1e6 the
    // plain fixed form is shorter than "5.515e4"; past ~1e6 the
    // exponent form wins and drops the leading plus.
    if (Math.abs(f) < 1_000_000) {
        return String(Math.round(f / 10) * 10);
    }
    return f.toPrecision(4).replace("e+", "e");
}
