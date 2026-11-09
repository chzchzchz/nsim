#!/usr/bin/env bash
# chart.sh - Chart every numeric plant sensor after applying a command file.
#
# Usage:
#   ./scripts/chart.sh --cmd <cmdfile> [--plant <plant.json>] [chart flags]
#
#   ./scripts/chart.sh --seconds 60 --cmd cmd.txt --plant plant.json
#   ./scripts/chart.sh --cmd cmd.txt --plant plant.json --sensors '.ReactorBuilding.*'
#   ./scripts/chart.sh --list ...         # list discovered sensors only
#
# The heavy lifting lives in scripts/chart_all.py: this is a thin shim
# that builds the simulator once and forwards to it. An embedded
# Python driver is used instead of bash because sampling thirteen
# hundred series, windowing command files, and avoiding matplotlib
# hangs as bash loops.
#
# Options (--seconds/--step/... are chart_all.py flags):
#   --cmd <file>       command file w/ per-line `TimeOffset Part.Label Value`
#                      (or a web-console *.json export); time stamps are
#                      absolute ms from simulation start
#   --plant <file>     initial plant JSON (default: sim design defaults)
#   --scenario <id>    starting scenario id (mutually exclusive with --plant)
#   --seconds <total>  total simulated seconds (default: last command
#                      offset + one step; required when --cmd is empty)
#   --step <seconds>   seconds between snapshots (default: 1)
#   --out <dir>        output directory (default: chart_output/)
#   --sensors <glob>   only chart sensors matching these patterns
#                      (comma-separated; '*' matches any tail; leading
#                      '.' as in jq paths is accepted, e.g.
#                      '.ReactorBuilding.*' or 'ReactorBuilding.*')
#   --list             discover sensor names and exit before simulating
#   --keep-json        keep per-step snapshots in the out dir
#   --no-index         skip writing index.html into the out dir
#   --bin <path>       prebuilt nsimrun binary (default: go build)
#
# Output:
#   chart_output/<Sensor.Path>.png   one chart per varying numeric sensor
#   chart_output/<Sensor.Path>.svg   same, when matplotlib is absent
#   chart_output/index.html          filterable page linking every chart
#   chart_output/<NNN>.json          per-step snapshots (with --keep-json)
#
# Examples:
#   # graph every sensor while applying cmd.txt to plant.json
#   ./scripts/chart.sh --cmd cmd.txt --plant plant.json
#
#   # graph every sensor for 60s with a command file
#   ./scripts/chart.sh --cmd cmd.txt --plant plant.json --seconds 60
#
#   # only pressurizer and RPV sensors
#   ./scripts/chart.sh --cmd cmd.txt --plant plant.json \
#       --sensors '.ReactorBuilding.Pressurizer.*,.ReactorBuilding.ReactorPressureVessel.*'
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [[ $# -eq 0 ]]; then
	echo "usage: $0 --cmd <cmdfile> [--plant <plant.json>] [chart flags]" >&2
	exit 1
fi

# Forward everything to python; no own flag parsing beyond --bin so
# chart_all.py remains the single source of truth for flags.
exec python3 "$SCRIPT_DIR/chart_all.py" "$@"
