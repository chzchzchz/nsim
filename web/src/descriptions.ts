// descriptions.ts — operator help text keyed by bridge id
// ("Part/Label"), loaded once from the generated
// /descriptions.json asset (web/scripts/gen-descriptions.mjs builds
// it from assets/descriptions/**.md). A missing entry just means the
// element renders no "(?)" button.

let helpTexts = new Map<string, string>();

export async function loadDescriptions(): Promise<void> {
    try {
        const res = await fetch("/descriptions.json");
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const raw = (await res.json()) as Record<string, string>;
        helpTexts = new Map(Object.entries(raw));
    } catch (err) {
        console.warn("(?) help disabled:", err);
    }
}

export function helpFor(id: string): string | undefined {
    return helpTexts.get(id);
}
