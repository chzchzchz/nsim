# Skill: driving a plant JSON from one state to another with an nsimrun cmdfile

Goal: given a saved `Plant` state JSON (`-plant`) and a desired end state
(e.g. "turbine producing RealPower"), produce a command file that gets there
when run as:

```bash
go run ./cmd/nsimrun/ -plant <state.json> -seconds <N> -cmd <cmds.cmdfile>
```

## 1. Inspect the source state

Pull only the relevant subsystem out of the state file; the files are large.

```bash
jq '.TurbineBuilding' plans/debug/turbines.json
jq '.ReactorBuilding.Grid, .Outside.CirculatingWaterSystem.PumpRunning' plans/debug/turbines.json
```

Note every field that could gate the transition: valve positions, Running
flags, latched trips, State enums, setpoints/demands, pump availability.

## 2. Read the model code before writing commands

Cmdfiles act through the same physics as the sim, so find out what actually
drives the target quantity:

- The part's `Step` function (core/turbine.go, core/condenser.go, ...) is the
  source of truth for transitions, interlocks, and trip latches.
- Constants live in core/parts.go (ramp rates, trip thresholds, design flows).
  Use them to compute timings instead of guessing:
  e.g. speed ramp 1800 rpm at TurbineSpeedRampRate = 30 rpm/s -> ~60 s roll;
  coast-down below the reset threshold ~3 s; condenser vacuum τ ≈ 2 s.
- Trace the callers up to Plant.Step to see which values are latched/copied
  between buildings each tick (e.g. Synchronized mirrors Generator.BreakerClosed;
  PlantOperation in reactor-lead mode overwrites Turbine.Setpoint every step,
  so an operator Setpoint may not survive).
- Prefer godoc MCP tools (`mcp_godoc_get_doc`, path "./core" + working_dir)
  over reading whole files.

Look specifically for **chicken-and-egg deadlocks**: a transition that requires
a condition that only the transition itself would create. In the turbine case:
Off→Running needs steamFlow > 0, steamFlow needs admission > 0, but the DEHM
governing-valve trim only runs while already Running. If no control exists for
the blocking field, the fix is either an operator action modeled elsewhere or
adding a spec-justified control to the relevant core/bridge_*.go.

Also identify **ordering constraints**: anything that must happen before
something else or a trip latches. Examples hit this session:

- Condenser needs CW pumps running to hold vacuum; admitting steam at
  atmospheric back-pressure trips the turbine on low vacuum immediately.
- Trip reset requires rotor speed ≈ 0, and turning gear holds 5 rpm —
  disengage turning gear first, wait for coast-down, then reset.

Check specs/ for the real startup/recovery sequence when modeling decisions
need justification (see next section).

## 3. Consult specs/

specs/the_westinghouse_pressurized_water_react/ is the primary source of
truth for plant behavior; physics questions may be checked against Wikipedia.

- Navigate with `specs/the_westinghouse_pressurized_water_react/toc.md` and
  `glossary.md` rather than grepping blind.
- Fastest route: the model code cites its justifications. Grep the relevant
  core/*.go comments for `specs/` references — e.g. turbine.go points at
  chapter_15/page_201.txt (DEHM operator controls) and page_204.txt (manual
  valve controllers), which is exactly what justified adding the missing
  governing-valve control this session.
- Read the cited page files around the referenced lines: neighboring
  paragraphs usually describe the full operating sequence (startup order,
  interlocks, trip behavior) that the cmdfile must reproduce.
- When you add code or commands based on a spec passage, cite the full path
  with line numbers (never truncate with "..."), e.g.
  `specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:24-33`.
- For nuclear-physics background not covered by the Westinghouse doc, fetch
  `https://en.wikipedia.org/wiki/<ARTICLE_NAME>` and cache it at
  `specs/wiki/<ARTICLE_NAME>.txt` before relying on it.

## 4. Enumerate the controls you may command

Cmdfile lines resolve against `Plant.GetControlMap()` — registered controls in
core/bridge_*.go only. Sensor-only state cannot be commanded.

```bash
grep -rn 'm\["' core/bridge_*.go | grep 'Control{'
```

Value formats match each control's Kind: bool `1`/`0`/`true`/`false`,
normalized fractions `0..1`, enum/momentary controls take their enum word
(e.g. `Reset`). Dot and slash paths are equivalent
(`TurbineBuilding.Turbine.SpeedDemand` == `TurbineBuilding/Turbine/SpeedDemand`).

If a required control is missing, add it to the right bridge file following the
existing pattern (Get/Set, Kind validation via bridge.ConvertControlValue /
bridge.StrToBool), with a comment citing the spec lines that justify operator
control. Rebuild and re-run tests.

## 5. Write a draft cmdfile

Grammar: one `TimeOffsetMs Part.Label Value` per line, `#` comments allowed,
lines auto-sorted by offset, commands with offsets past the simulated duration
still apply at the end.

Sequence commands in physically-safe order with generous margins:

```
0   <support systems: cooling, vacuum, pumps, clear runback latches>
10s <remove holdbacks: disengage turning gear, open block valves>
20s <reset trips, then start the driver: crack governing valve, raise demand>
120s<close the breaker / reach the target once sync or steady conditions hold>
```

Every command line must be preceded by `#` comment lines explaining the
*reason* for the command — the interlock it avoids, the threshold it waits
out, the spec sequence step it follows — not a restatement of what the line
does. A reader should be able to replay or adapt the file without re-deriving
the physics. Start the file with a header giving the source state, the run
invocation, and an overview of the timeline:

```
# Restart the main turbine-generator from plans/debug/turbines.json
# (on turning gear, governing valves shut) up to a synchronized generator.
#
# Run: nsimrun -plant plans/debug/turbines.json -seconds 300 -cmd plans/debug/turbine.cmdfile

# Circulating water is the condenser heat sink; without it the shell sits
# at atmospheric and any steam admitted trips the turbine on low vacuum.
0 Outside/CirculatingWaterSystem/PumpRunning 1

# Turning gear holds the rotor at 5 rpm, above the trip-reset threshold;
# disengage so it coasts down (~3 s) before the reset at t=20 s.
10000 TurbineBuilding/Turbine/TurningGear 0
```

## 6. Run, inspect, iterate

Build once per iteration (binaries go to /tmp/nsim-<branch-name>/ per AGENTS.md):

```bash
go build -o /tmp/nsim-main/nsimrun ./cmd/nsimrun
/tmp/nsim-main/nsimrun -plant plans/debug/turbines.json \
    -seconds 300 -cmd plans/debug/turbine.cmdfile \
    > out.json 2> out.err
cat out.err                       # command errors + end-of-run alarm summary
jq '<target fields>' out.json     # did we reach the goal?
```

Diagnose failures systematically:

- `command error ... unknown control` -> typo, wrong building prefix, or the
  field has no control (see step 3).
- An `[TRIP]` alarm names the protection that fired; work backwards to which
  precondition was skipped (vacuum, bearing oil, overspeed from a bad demand).
- State didn't change at all -> re-read the Step function for a gate you missed
  (latched flag, caller-copied value, deadlock as in step 2).
- Compare against the input state to tell new alarms from pre-existing ones
  (`jq` the same paths on both files).

Repeat until the target fields read correctly at the end of the run, then do a
final clean pass: keep timings conservative, keep comments accurate, delete
experimentation debris.

## Gotchas learned here

- `-plant` merging replaces whole top-level building subtrees; fields absent
  from the file become zero values, not defaults. Don't assume MakePlant
  defaults survived (e.g. GoverningValve.Open defaulted to 1 in MakePlant but
  was 0 in the export, and that difference was the whole bug).
- Latched trips persist across steps until explicitly reset under the right
  conditions; clearing the cause is not the same as clearing the latch.
- Operator setpoints can be overridden by automatic layers (reactor-lead
  PlantOperation drives Turbine.Setpoint). Verify who owns the value you set.
- The diesel.tripped-style alarms in the stderr summary may be pre-existing
  conditions of the source state; check before treating them as your bug.
