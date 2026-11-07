# Skill: Developing Support for a Plant Operation Scenario

Process for taking an operator-level plant operation (from the Westinghouse
spec documentation) and building executable support for it in nsim: an
implementation plan written to `plans/x/scenario/<N>.txt` (<N> = the number
of the scenario currently being worked, taken from `plans/x/scenarios.txt`),
start-state configuration, a control-command plan, headless execution via
`cmd/nsimrun`, and verification of results against spec-derived expectations.

Inputs:

- Scenario catalog: `plans/x/scenarios.txt` (operations with spec citations)
- Source of truth: `specs/the_westinghouse_pressurized_water_react/`
  (primary) and `specs/pwr_sim/` (training-evolution procedures)
- Runner semantics: `cmd/nsimrun/README.md`

## Evidence rule

Justify every decision with citation evidence drawn from the sources above.
Each start-state value, operator action, control mapping, acceptance band,
threshold, and rate limit written into the plan carries a full-path spec
citation with line numbers; re-open the page file before trusting a line
range, because OCR page boundaries shift content by a few lines. A claim
that cannot be cited is recorded in the plan explicitly as an assumption or
model gap — never stated as if it were spec'd. Facts taken from the code
(control `Kind`s, default plant state, runner flags) are justified by reading
the registration source or godoc at the cited location, not from memory.
The Phase 0 validation verdict is the first application of this rule: the
scenario itself must be citable end to end before anything is built for it.

## Phase 0 — Validate the scenario against the documentation

Before defining or implementing anything, compare the scenario against the
documentation to confirm it is a real, self-consistent plant operation. An
operation that cannot be grounded in the sources is rejected or descoped
here, before any plan, start state, or command effort is spent on it.

1. **Open every page the catalog entry cites.** OCR page boundaries shift
   content by a few lines, so re-read the cited ranges and confirm they
   actually describe this operation — not a neighboring one. A citation that
   does not hold invalidates the catalog entry; fix the entry first.
2. **Decompose the evolution into legs and cite each one.** Initial
   conditions, each major transition, and the end state must each trace to
   primary text (Chapter 16 operations, `specs/pwr_sim/` evolutions,
   Chapter 5 ESFAS actuations). A leg with no citable source is either cut
   from scope or recorded in the plan as an explicit assumption/model gap —
   never silently invented.
3. **Check initial conditions and end states against the mode definitions.**
   Pressures, temperatures, levels, and equipment lineups must agree with
   the Chapter 16 anchors (Table 16-1 and the cold-shutdown / hot-standby /
   power-operation definitions in
   `specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt`
   and `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt`).
   A start state that contradicts them means either the wrong operating mode
   was assumed or the catalog entry describes a different evolution.
4. **Check the automatic actions the docs promise.** Trips, interlocks, and
   auto-starts the spec attaches to the evolution (RPS trip list, ESFAS
   Table 7-1, EFWS auto-start table) must appear in the scenario's expected
   outcome, and nothing in the scenario may require the plant NOT to do
   something the spec says it does automatically. Where the scenario expects
   a different response, reconcile against the source of truth (the
   Westinghouse document wins over `specs/pwr_sim/` training-evolution
   wording) and record the resolution in the plan.
5. **Record the verdict in the plan** (`plans/x/scenario/<N>.txt`) before
   Phase 1 begins: which citations were re-opened, what each leg traces to,
   and any contradiction found plus its resolution. A scenario whose verdict
   lists unresolved contradictions is not implemented.

## Phase 1 — Define the scenario

Write the implementation plan for the chosen operation to
`plans/x/scenario/<N>.txt` before touching any tooling. It records the three
lists below, plus in later phases the per-command citations and the pass/fail
table. The plan may be updated as new information becomes available (stability
findings, control behavior, measured rates) — treat it as a living document,
not a one-time deliverable:

1. **Start state** — temperatures, pressures, levels, equipment lineups,
   operational mode (`core.PlantOperationalMode`: ColdShutdown, HotStandby,
   PowerOperation). Every value must carry a spec citation.
2. **Operator actions** — each action mapped to the evolution step it
   implements, with citation. The plan must be very specific about which
   controls are updated to complete the procedure: for every step record the
   exact `core.GetControlMap` control path(s) and the value(s) written
   (matching the `Kind` grammar of each registration). Ordering of the steps
   is required; exact simulated-time offsets are chosen later in Phase 3/4.
   An action whose control does not exist yet is recorded explicitly as a gap
   and routed through "Missing controls" — do not leave the mapping vague or
   defer it to Phase 3.
3. **Expected outcome** — observable end state and rate limits that must hold
   during transit, each with citation. Give every expectation a numeric
   acceptance band, e.g. "cooldown holds 50 +/-10 F/hr" rather than "cools
   down"; Phase 5 comparisons are mechanical only if the threshold is written
   here first.

Example (cold startup): initial conditions <140 F RCS, 400 psig, water-solid
pressurizer are defined in
`specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:38-51`
with the Table 16-1 footnotes at
`specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:89-95`;
hot-standby/no-load conditions (557 F, 2250 psia) at
`specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-61`.

## Phase 2 — Create the plant start-state configuration

Only do this if the default plant (`core.MakePlant()`) does not match the
scenario start state.

1. **Dump the full default state** as the editing base:

   ```bash
   mkdir -p plans/x/scenario/<N>
   go run ./cmd/nsimrun -seconds 0 > plans/x/scenario/<N>/start.json
   ```

2. **Edit fields inside the full dump**. The `-plant` merge in
   `cmd/nsimrun/main.go:47-63` replaces top-level JSON keys wholesale
   (`ReactorBuilding`, `AuxiliaryBuilding`, `TurbineBuilding`, `Outside`,
   `Electrical`, `PlantOperation`). Do NOT hand-write a minimal JSON file with
   one nested field and expect deep merging: replacing a top-level key drops
   all sibling state in that key.
3. **Justify every deviation from defaults** with a spec citation placed in
   the implementation plan `plans/x/scenario/<N>.txt` (not in JSON). Examples
   of citable start states:
   - Hot startup initial conditions (557 F, 2235 psig, 25 percent
     pressurizer level):
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:48-68`
   - Cold shutdown boration requirement:
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:50-55`
   - Mode definitions and gating between them:
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_222.txt:79-86`,
     hot standby definition at
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:109-116`.
4. **Validate stability**: run the start state briefly with no commands and
   confirm parameters hold steady wherever the scenario expects steady state:

   ```bash
   go run ./cmd/nsimrun -plant plans/x/scenario/<N>/start.json -seconds 60 | jq '.'
   ```

   If the state drifts, either the configuration contradicts the internal
   equilibrium or the model is missing support (see "Missing controls" below).

## Phase 3 — Develop the control-command plan

Confirm the control mapping recorded for each Phase-1 operator action against
`core.GetControlMap`. Enumerate
available controls from the registrations in `core/bridge*.go`; representative
entries:

| Operator action (spec evolution) | Control path |
|---|---|
| Start RCPs sequentially (`chapter_16/page_224.txt:35-37`) | `ReactorBuilding.ReactorCoolantPumps.A..D.Running` (legacy aggregate `ReactorBuilding.ReactorCoolantPump.Running` fans out to all four) |
| Energize pressurizer heaters / form bubble (`page_224.txt:11-23`) | `ReactorBuilding.Pressurizer.HeatersOn` |
| Pressurizer spray for cooldown (`chapter_16/page_227.txt:91-98`) | `ReactorBuilding.Pressurizer.SprayOpen` |
| Dilute/borate to criticality (`page_224.txt:124-138`) | `ReactorBuilding.ReactorPressureVessel.ControlRods.BoronSetpoint` |
| Withdraw/insert rod banks (`page_224.txt:155-163`) | `ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion` |
| Manual reactor trip response (`chapter_16/page_233.txt:62-64`) | `ReactorBuilding.ReactorPressureVessel.Trip`, `.ReactorProtectionSystem.Reset` |
| Block/restore SI lineup (`chapter_16/page_227.txt:108-113`) | `ReactorBuilding.SafetyInjectionSystem.HHSIPumpRunning`, PORV/BlockValve controls |
| Place RHRS in service <350 F (`chapter_16/page_227.txt:124-131`) | `AuxiliaryBuilding.ResidualHeatRemovalSystem.TrainA/B.*` (pumps, suction, bypass valves) |
| Charging/letdown for level control (`page_224.txt:15-33`) | `AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning`, `.LetdownValveOpen` |
| Boration to cold shutdown margin (`page_227.txt:50-55`) | `AuxiliaryBuilding.ChemicalVolumeControlSystem.BoronMode`, `.BTRSPump` |
| AFW to main feedwater swap (`chapter_16/page_226.txt:45-50`) | `AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA/B.MotorPumpRunning`, `TurbineBuilding.MainFeedwaterControlValve.Open` |
| Condensate / main-feedwater pump startup (`chapter_16/page_224.txt:175-177`) | `TurbineBuilding.CondensatePump.Running`, `TurbineBuilding.MainFeedwaterPump.Running` |
| Pull condenser vacuum (`chapter_16/page_224.txt:171-177`) | `TurbineBuilding.Condenser.VacuumPump` (sensor: `TurbineBuilding.Condenser.Pressure`) |
| Turbine unload/trip/recovery (`pwr_sim/chapter_3/page_041.txt:5-46`) | `TurbineBuilding.Turbine.StopValve`, `.Setpoint`, `.DumpValve`, `.LoadRunback` |
| Parallel generator on the grid (`chapter_16/page_226.txt:52-53`) | `TurbineBuilding.Generator.BreakerClosed`, `TurbineBuilding.Turbine.SpeedDemand` (roll), `TurbineBuilding.Turbine.LubeOilPump` (pre-startup checklist) |
| SG pressure relief / atmospheric dump (`chapter_16/page_233.txt:17-38`) | `ReactorBuilding.SteamGenerator.AtmosphericDumpValve.Open`, `.SafetyValve.Open`, `ReactorBuilding.PORV.Open` |
| LOOP / EDG sequencing (`chapter_16/page_233.txt:103-109`) | `Electrical.ESFBusA/B.BreakerClosed`, `AuxiliaryBuilding.DieselGenerator.StartSignal` |
| SI lined up with initiation signal manually blocked, accumulators isolated during startup lineup (`chapter_16/page_224.txt:11-13`) | likely gap — no dedicated SI-block control exists yet; route through "Missing controls" |

Before writing values, read each target control's registration in the
`bridge_*.go` file: the `Kind` field fixes the value grammar via
`bridge.ConvertControlValue` (`bool`, `normalized`, `ppm`, `enum`, ...), and
e.g. `ControlRods.Insertion` takes a normalized demand while
`ControlRods.BoronSetpoint` takes ppm directly.

Write this command plan as a command file consumed by `nsimrun` (format
documented in `cmd/nsimrun/README.md:15`):

```
0      ReactorBuilding.ReactorCoolantPump.Running 1
60000  ReactorBuilding.Pressurizer.HeatersOn 1
```

or export the JSON envelope from the web console
(`{"commands":[{"offsetMs":...,"control":...,"value":...}]}`).

Rules for the command plan:

- Offsets are milliseconds of simulated time from run start; `core.TimeStep`
  is 1 ms (`core` package docs). Multiple commands with the same offset apply
  in file order (`cmd/nsimrun/main.go:79-86`).
- Command application failures are logged, not fatal
  (`cmd/nsimrun/main.go:82-85`) — always check stderr for
  `command error at ... ms` lines after a run.
- Commands scheduled past `-seconds` are force-applied after the loop with no
  intervening stepping (`cmd/nsimrun/main.go:90-95`); never rely on this —
  keep all offsets inside the simulated window.
- Annotate each command line/block with the spec citation for the evolution
  step it performs, appended to the implementation plan
  `plans/x/scenario/<N>.txt` (not inline in the command file, which must stay
  machine-parseable).
- Command paths accept dot or slash form interchangeably
  (`cmdfile.ResolveKey`, `cmdfile/cmdfile.go:87-98`); unknown names fail with
  `unknown control` at apply time — smoke-test the whole file against a 0-second
  run first (`go run ./cmd/nsimrun -seconds 0 -cmd cmds.txt`).
- Controls are one-shot writes, not ramps. Rod withdrawal over twenty minutes
  means either one demand write (if the control models internal demand
  dynamics, like `Insertion`) or many time-stamped small writes (if it does
  not). Check which from the registration's `Set` body before planning.
- Respect the rate-limit layer: `core.PlantOperation` enforces 5%/min loading
  and 50 F/hr heatup/cooldown with its own spec citations embedded
  (`core.PlantOperation` godoc; limits specified in
  `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141`
  and `specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:153-154`).
  Do not schedule commands that fight these limits unless the scenario is
  explicitly testing a violation.

## Phase 4 — Play commands into nsimrun

```bash
go build ./... && \
go run ./cmd/nsimrun \
  -plant plans/x/scenario/<N>/start.json \
  -seconds <duration> \
  -cmd  plans/x/scenario/<N>/commands.txt \
  > plans/x/scenario/<N>/final.json
```

Guidance:

- Simulation time equals physical time (1 ms steps). Evolutions measured in
  hours (13 h cold startup per Table 16-1,
  `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:39-88`;
  ~20 h cooldown,
  `specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:149-151`)
  are executed as chained segments:
  1. Split `commands.txt` into `seg01.txt`, `seg02.txt`, ... at phase
     boundaries (heatup, bubble formation, criticality, ...).
  2. Rebase each segment's offsets to zero at its own start.
  3. Run sequentially, feeding stdout back in:
     `go run ./cmd/nsimrun -plant seg01-out.json -seconds T2 -cmd seg02.txt > seg02-out.json`
  4. Keep the last segment's output as the final result for Phase 5.
- Validate rate-limited behavior on short windows first (minutes, not hours)
  rather than running the full multi-hour evolution repeatedly.
- Capture stderr separately; command errors appear there.

## Phase 5 — Compare results with expected outcome

1. Extract observables from the output JSON with `jq`. Field paths mirror the
   `core.Plant` schema (same names as the sensor/control parts).
2. Check each Phase-1 expectation against its numeric band. Typical checks and
   their citations (verify every cited line range by opening the page file —
   OCR page boundaries shift content by a few lines):
   - Heatup/cooldown stayed within 50 F/hr:
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:140-141`,
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:153-154`
   - Pressurizer drained into the 20-80 percent band while forming the bubble:
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-23`
   - Criticality approach did not exceed 1.0 DPM steady-state without rod
     motion:
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:155-163`
   - AFW-to-main-feedwater transition near ~3.5 percent steam flow:
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:45-50`
   - Hot-standby RCP cap (max two of four secured):
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:24-29`
   - RHRS entry conditions (below 350 F and ~400 psig):
     `specs/the_westinghouse_pressurized_water_react/chapter_16/page_227.txt:128-131`
     (model threshold constant: `core.RHRSTempThreshold`)
   - Trip setpoints respected during transients (e.g., low pressurizer
     pressure trip at 1800 psia, `core.RPSLowPressurizerPressureTrip`).
3. Append a pass/fail table to the implementation plan
   (`plans/x/scenario/<N>.txt`). Each failure is one of:
   - **Command-plan error** — wrong command, wrong timing, wrong start state.
     Fix the
     plan or start state and rerun Phase 4.
   - **Model gap** — the simulator cannot express the action or does not
     reproduce the spec'd behavior. Go to "Missing controls".
4. Never mark a scenario supported until every expectation passes with the
   citation column filled in.
5. Encode the durable physics assertions as fast unit tests in
   `core/scenario<N>_test.go` (the per-scenario file used by the existing
   replay tests): drive `core.PlantOperation.Step` (or the specific
   part's Step) directly with boundary inputs and assert the spec'd rate or
   setpoint behavior using testify `require`/`assert` (per `AGENTS.md`, tests
   under 1 s). Full-run command files stay as scenario artifacts, not tests.

## Scenario catalog registration

After the pass/fail table goes green, surface the evolution as a coded
scenario so the web picker and headless tooling open its start state
directly (`core.Scenarios()` / `core.ScenarioByID`, consumed by
`web/wasm/main.go`):

1. Append an entry to `scenarioCatalog` in `core/scenarios.go`: an `ID`
   slug, `Number` = <N> from `plans/x/scenarios.txt`, the matching
   `Category`, a short `Title`, and operator-facing `StartState` /
   `Objective` strings.
2. `Apply` layers the time-zero state override onto a proven lineup
   fixture (`fullPowerLineup`, `coldShutdownLineup`, ...). Pick the fixture
   matching the Phase-2 start.json instead of inventing initial conditions;
   limit the override to the latched fault or actuation signal.
3. Operator actions stay in the command plan, never inside `Apply`.

## Missing controls: implementing support

When a Phase-3 operator action has no matching `GetControlMap` entry:

1. **Confirm the gap**: grep the `m["..."] = &bridge.Control{` registrations
   across `core/bridge.go`, `core/bridge_reactor.go`, `core/bridge_aux.go`,
   `core/bridge_turbine.go`, `core/bridge_outside.go`.
2. **Justify from the spec**: find the procedure text requiring the action
   (Chapter 16 operations, Chapter 4 auxiliary systems, Chapter 5 engineered
   safeguards) and record the full-path citation. An un-cited control must not
   be implemented ("only implement what is important for simulation").
3. **Implement minimally**, choosing the smaller change first:
   - *State exists, surface it*: add a registration in the appropriate
     `bridge_*.go` following neighboring entries: choose the `Part` path,
     a `Kind` understood by `bridge.ConvertControlValue`, and a
     `SetControlFunc` that validates range/timing. The web UI picks up new
     entries on reset (`web/README.md:125-130`); add a widget case in
     `web/src/controls.ts` only if introducing a new Kind.
   - *State missing*: add minimal state/dynamics to the relevant part file
     (e.g., `core/pressurizer.go`, `core/rhrs.go`). Before editing Go code,
     query the `godoc` MCP server for exact types/signatures; follow
     `AGENTS.md` (generalization over specialization, no unnecessary
     formatting changes, testify `require`/`assert`, tests under 1 s).
4. **Test behavior, not wiring**: write a test that sets the new control via
   the control map, steps the plant, and asserts the spec'd effect (e.g.,
   heater actuation raises pressurizer temperature toward saturation per
   `specs/the_westinghouse_pressurized_water_react/chapter_16/page_224.txt:15-19`).
5. **Return to Phase 3** with the new control path added to the mapping table.

## Definition of done for a scenario

- Implementation plan with the validation verdict, the three lists, command
  annotations, and
  pass/fail table lives at `plans/x/scenario/<N>.txt`; `start.json`,
  `commands.txt`, and run outputs live under `plans/x/scenario/<N>/`
- Every cited line range re-verified by opening the page file
- Full run reproduces every expected outcome within its numeric band
- All deviations fixed or explicitly listed as known gaps
- `core/scenarios.go` carries the `Scenario` entry for `<N>` applying the
  start-state fixture
- Durable assertions captured as `core/scenario<N>_test.go` fast unit tests
- `go build ./...` and `go test ./...` pass

## Appendix: worked example skeleton (turbine trip recovery)

Scenario source: `specs/pwr_sim/chapter_3/page_041.txt:5-46`. Timings below
are illustrative — derive real ones from the model's response.

Start state: default plant already represents power operation, so start.json
is just the defaults dump (`nsimrun -seconds 0`). Expected outcomes with
bands: reactor power reduced toward ~60 percent by rod insertion without
reactor trip; SG pressure held near setpoint by dump/bypass steam; recovery
to full power possible after reset (citations: stepback rationale
`specs/pwr_sim/chapter_3/page_041.txt:27-35`, bypass-valve role
`specs/pwr_sim/chapter_3/page_041.txt:38-45`).

```
# seg01.txt — trip and stabilize
0      TurbineBuilding.Turbine.StopValve 0
0      TurbineBuilding.Turbine.DumpValve 1
2000   ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion 0.6
30000  ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion 0.0

# seg02.txt — clear the trip and recover
0      TurbineBuilding.Turbine.DumpValve 0
1000   TurbineBuilding.Turbine.Reset Reset
2000   TurbineBuilding.Turbine.StopValve 1
5000   TurbineBuilding.Turbine.Setpoint <ramp target>
```

Comparison: extract power fraction, SG pressure, and Tavg trajectories from
segment outputs (`jq 'keys'` on an output file to discover field paths; the
full schema reference is `core/plant_json_test.go`), then check them against
the bands recorded in Phase 1.
