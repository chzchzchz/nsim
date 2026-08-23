#!/usr/bin/env bash
# chartsim.sh - Run nsimrun in timestep increments and plot values over time.
#
# Usage:
#   ./scripts/chartsim.sh <timestep_seconds> <total_seconds> <jq_paths> [input_json] [cmd_file]
#
# Arguments:
#   timestep_seconds - integer seconds between each simulation run (e.g. 1)
#   total_seconds    - integer total simulation duration (e.g. 60)
#   jq_paths         - comma-separated jq paths to extract from each JSON output
#                      (e.g. '.ReactorBuilding.ReactorCoolantPump.Running',
#                            '.ReactorBuilding.Pressurizer.Pressure,.ReactorBuilding.ReactorPressureVessel.ThermalPower')
#   input_json       - optional starting plant JSON (defaults to MakePlant via nsimrun)
#   cmd_file         - optional command file (Part.Label Value per line, see nsimrun)
#
# Output:
#   Creates chart_output/ with:
#     - <NNN>.json       one JSON snapshot per timestep (zero-padded)
#     - data.txt         tab-separated values: time then each jq_path value
#     - chart.gp         gnuplot script
#     - chart.png        rendered chart (run gnuplot chart.gp to generate)
#
# Examples:
#   # Track RCP running state over 60 seconds, sampling every 1 second
#   ./scripts/chartsim.sh 1 60 '.ReactorBuilding.ReactorCoolantPump.Running'
#
#   # Start from a saved plant and apply control commands
#   ./scripts/chartsim.sh 1 60 '.ReactorBuilding.Pressurizer.Pressure' plant.json cmd.txt
#
#   # Track multiple values on the same graph
#   ./scripts/chartsim.sh 5 120 \
#       '.ReactorBuilding.Pressurizer.Pressure,.ReactorBuilding.ReactorPressureVessel.ThermalPower'
#
#   # Use defaults (MakePlant) and no commands
#   ./scripts/chartsim.sh 1 10 '.ReactorBuilding.ReactorCoolantPump.Running'

set -euo pipefail

if [[ $# -lt 3 ]]; then
	echo "usage: $0 <timestep_seconds> <total_seconds> <jq_paths> [input_json] [cmd_file]" >&2
	exit 1
fi

TIMESTEP="$1"
TOTAL="$2"
JQ_PATHS_RAW="$3"
INPUT_JSON="${4:-}"
CMD_FILE="${5:-}"

if ! [[ "$TIMESTEP" =~ ^[0-9]+$ && "$TOTAL" =~ ^[0-9]+$ ]]; then
	echo "error: timestep and total_seconds must be positive integers" >&2
	exit 1
fi

if [[ -n "$INPUT_JSON" && ! -f "$INPUT_JSON" ]]; then
	echo "error: input json '$INPUT_JSON' not found" >&2
	exit 1
fi

if [[ -n "$CMD_FILE" && ! -f "$CMD_FILE" ]]; then
	echo "error: command file '$CMD_FILE' not found" >&2
	exit 1
fi

IFS=',' read -r -a JQ_PATHS <<< "$JQ_PATHS_RAW"
NUM_PATHS=${#JQ_PATHS[@]}

PAD_WIDTH="${#TOTAL}"
OUT_DIR="chart_output"
mkdir -p "$OUT_DIR"

DATA_FILE="$OUT_DIR/data.txt"
> "$DATA_FILE"

PREV_PLANT="$INPUT_JSON"
t=0

# Run nsimrun in increments of TIMESTEP until we reach TOTAL.
# Each run inherits the previous output as its starting plant,
# producing a continuous time series.
while [[ $t -lt $TOTAL ]]; do
	t=$((t + TIMESTEP))
	if [[ $t -gt $TOTAL ]]; then
		t=$TOTAL
	fi

	padded=$(printf "%0${PAD_WIDTH}d" "$t")
	out_json="$OUT_DIR/${padded}.json"

	args=(-seconds "$TIMESTEP")
	if [[ -n "$PREV_PLANT" ]]; then
		args+=(-plant "$PREV_PLANT")
	fi
	if [[ -n "$CMD_FILE" ]]; then
		args+=(-cmd "$CMD_FILE")
	fi

	echo "running nsimrun at t=${t}s -> $out_json"
	if ! go run ./cmd/nsimrun/ "${args[@]}" > "$out_json"; then
		echo "error: nsimrun failed at t=${t}s" >&2
		exit 1
	fi

	# Extract the requested jq_path values for this timestep.
	line="$t"
	for path in "${JQ_PATHS[@]}"; do
		val=$(jq -r "$path" "$out_json" 2>/dev/null || echo "null")
		line="$line $val"
	done
	echo "$line" >> "$DATA_FILE"

	PREV_PLANT="$out_json"
done

# Generate gnuplot script. All paths are plotted on the same graph
# as separate lines sharing the x-axis (time in seconds).
GNUPLOT_SCRIPT="$OUT_DIR/chart.gp"
{
	echo "set terminal pngcairo size 1200,600"
	echo "set output '$OUT_DIR/chart.png'"
	echo "set title 'Simulation over time'"
	echo "set xlabel 'Time (s)'"
	echo "set ylabel 'Value'"
	echo "set grid"
	printf "plot "
for i in "${!JQ_PATHS[@]}"; do
	col=$((i + 2))
	path="${JQ_PATHS[$i]}"
	if [[ $i -gt 0 ]]; then
		printf ", "
	fi
	printf "'%s' using 1:%d with lines title '%s'" "$DATA_FILE" "$col" "$path"
done
echo ""
} > "$GNUPLOT_SCRIPT"

echo "gnuplot script written to $GNUPLOT_SCRIPT"
echo "data written to $DATA_FILE"
echo "run: gnuplot $GNUPLOT_SCRIPT"