#!/usr/bin/env node
// Builds public/descriptions.json from assets/descriptions/**.md.
// Each file path mirrors a bridge id: "Part/Label" maps to
// assets/descriptions/Part/Label.md; the file's trimmed contents are
// the markdown description shown by the UI's "(?)" button.
import { readdirSync, readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const seedDir = resolve(here, "..", "..", "assets", "descriptions");
const outFile = resolve(here, "..", "public", "descriptions.json");

function walk(dir, acc) {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
        const p = join(dir, e.name);
        if (e.isDirectory()) walk(p, acc);
        else if (e.name.endsWith(".md")) {
            acc[relative(seedDir, p).slice(0, -3)] =
                readFileSync(p, "utf8").trim();
        }
    }
    return acc;
}

const descriptions = walk(seedDir, {});
const sorted = Object.fromEntries(
    Object.entries(descriptions).sort(([a], [b]) => a.localeCompare(b)),
);
mkdirSync(dirname(outFile), { recursive: true });
writeFileSync(outFile, JSON.stringify(sorted, null, 2) + "\n");
console.log(`descriptions.json: ${Object.keys(sorted).length} entries`);
