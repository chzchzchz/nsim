#!/usr/bin/env node
// Builds public/descriptions.json from every documentation asset
// root. Each root mirrors element ids: assets/descriptions holds
// bridge control/sensor ids ("Part/Label"), assets/alarms holds
// alarm ids ("subsystem.condition"). Each file's trimmed markdown is
// keyed by its id in the output.
import { readdirSync, readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoDir = resolve(here, "..", "..");
const seedRoots = ["descriptions", "alarms"].map((d) =>
    resolve(repoDir, "assets", d),
);
const outFile = resolve(here, "..", "public", "descriptions.json");

function walk(seedDir, dir, acc) {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
        const p = join(dir, e.name);
        if (e.isDirectory()) walk(seedDir, p, acc);
        else if (e.name.endsWith(".md")) {
            acc[relative(seedDir, p).slice(0, -3)] =
                readFileSync(p, "utf8").trim();
        }
    }
    return acc;
}

const descriptions = {};
for (const root of seedRoots) {
    walk(root, root, descriptions);
}
const sorted = Object.fromEntries(
    Object.entries(descriptions).sort(([a], [b]) => a.localeCompare(b)),
);
mkdirSync(dirname(outFile), { recursive: true });
writeFileSync(outFile, JSON.stringify(sorted, null, 2) + "\n");
console.log(`descriptions.json: ${Object.keys(sorted).length} entries`);
