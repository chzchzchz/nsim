#!/usr/bin/env bash
# Generate 1 s plant-state snapshots for every disaster/damage/
# contamination/envelope cmdfile.
#
# For each <name>.txt under meltbench/{disaster,damage,contamination,envelope}/,
# writes meltbench/plantjson/<subdir>/<name>.1s.plant.json.
#
# Outputs are gitignored; regenerate locally as needed. recover/ is not
# snapshotted by design: this script captures the initial plant setup,
# and recovery actions land at 10 s, past the 1 s horizon.
#
# Usage: from the repo root, `bash meltbench/generate.sh`
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT="$SCRIPT_DIR/plantjson"

if [ ! -d "$SOURCE_ROOT/cmd/nsimrun" ]; then
  echo "generate.sh: $SOURCE_ROOT does not look like the repo root (cmd/nsimrun missing)" >&2
  exit 1
fi

cd "$SOURCE_ROOT"

for sub in disaster damage contamination envelope; do
  mkdir -p "$OUT/$sub"
done

count=0
for sub in disaster damage contamination envelope; do
  for src in "$SCRIPT_DIR/$sub"/*.txt; do
    [ -f "$src" ] || continue
    base="$(basename "$src" .txt)"
    out="$OUT/$sub/${base}.1s.plant.json"
    echo "nsimrun -cmd $sub/${base}.txt -seconds 1 -> plantjson/$sub/${base}.1s.plant.json"
    go run ./cmd/nsimrun/ -cmd "$src" -seconds 1 > "$out"
    count=$((count + 1))
  done
done

echo "Wrote $count plant files to $OUT/"
