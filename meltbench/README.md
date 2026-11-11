# Corium production via nsimrun cmdfile — sequence catalogue

The simulator has **no corium state**. "Production of corium" is therefore
operationalized as: **driving the lumped clad past the zircaloy melting
regime until the `VesselFailed` latch fires** (`core/reactor.go`, set when
uncovered clad reaches `ZrMeltF`). Per Kuan/Haskin
(`specs/wiki/Nuclear_meltdown.txt`), molten zircaloy plus dissolved UO₂
flowing down and freezing forms the debris bed (stage 5, ~1,700 K);
relocation to the lower plenum (stage 6) is corium itself. `VesselFailed`
latches at 2,150 K (3,410 °F) while uncovered, the model's stage-6
boundary.

Directory layout:

- `disaster/` — unrecovered fault sequences (no operator recovery within
  the run window). Corium-positive (`VesselFailed=true`) or high-damage
  (oxidation capped) unless marked a negative control.
- `recover/` — recovery twins: each starts with the same priming as its
  disaster twin, then an operator action at 10 s (BlockValve isolate,
  letdown close, trip + SI restore, spool depressurize). True recoveries
  ledger-pin; no-closing probes keep draining.
- `damage/` — non-corium pump damage sequences (RCP seal, EFWS deadhead)
- `contamination/` — environmental pathway sequences (Paths A/B/C, charcoal delay)
- `envelope/` — design-basis envelope scenarios: one actuation or failure
  mode per file (CV22 fail-open, dropped rod, RCP seal, mid-loop drain,
  rod ejection), with success/failure twins sharing a numeric prefix.
- `plantjson/` — generated 1 s state snapshots for the `disaster/`,
  `damage/`, `contamination/` and `envelope/` cmdfiles (`generate.sh`).
  Gitignored; not committed.

## Cmdfile header convention

Every `disaster/` and `recover/` cmdfile must explain, in `#` comment
lines before the first command:

1. **Purpose** — which fault/drain/interlock it tests and how it
   attacks the RCS inventory/heat-removal balance.
2. **Avoided outcome** — for `disaster/`, the corium definition above
   (`CladTempF >= 3,410 F` while uncovered → `VesselFailed`); for
   `recover/`, which disaster-twin fault each timed action terminates
   or mitigates.
3. **Verdict / mechanism** — e.g. relief drains to PRT, ledger pins,
   anchor-driven vs ledger-driven, or negative-control status.
4. **Lineup caveats** — commands that merely pin `MakePlant` defaults
   (call them out as such).

Keep each header to ~3–6 lines; the one-line catalogue entries below
stay summaries, the cmdfile owns the explanation. See
`disaster/corium_05_makeup_shut_porv.txt` for the template.

## Run command

```
go run ./cmd/nsimrun/ -cmd <file> -seconds <N> > out.json
```

`nsimrun` steps at `core.DefaultTimeStep` (1 ms, `core/units.go:47`) and
only writes the final state, so one invocation yields one tick. Generate
one file per wanted tick by re-running with a larger `-seconds`:

```bash
# t=0: opening commands applied, zero steps taken
go run ./cmd/nsimrun/ -cmd <corium file> -seconds 0 > state_t0000.json

# first few ticks (10 ms, 100 ms, 1 s, ...); same -cmd each time
for t in 0.01 0.1 1 5 10; do
  go run ./cmd/nsimrun/ -cmd <corium file> -seconds $t \
    > state_$t.json
done
```

- Use `-seconds 0` for the pure lineup after the `offset=0` commands
  (break open, SI blocked, ...) have been applied but before any physics.
- Prefer `< 3600` s per snapshot: at 1 tick/ms, 1 simulated hour costs
  ~1 min wall-clock (the full corium runs are hours long).
- Regenerate the 1 s snapshots with `bash meltbench/generate.sh`, which
  writes `meltbench/plantjson/{disaster,damage,contamination,envelope}/<name>.1s.plant.json`.
  The script snapshots initial plant setup only; `recover/` is skipped
  by design (its twins are disaster twins plus an operator action
  at 10 s, past the snapshot horizon). Outputs are
  gitignored; regenerate locally as needed.
- Later snapshots replay from scratch with the same `-cmd`, they do not
  need `-plant` chaining. Chain with `-plant <previous state>` only to
  continue without replaying, or to strip/re-strip state no cmdfile can
  set every tick (e.g. `corium_03`'s emptied accumulator).
- Validate a snapshot by re-ingesting it:
  `go run ./cmd/nsimrun/ -plant state_t0000.json -seconds 0 > /dev/null`.

## Files in `disaster/`

- `disaster/corium_01_hotleg_si_block.txt` — corium_01: large-break LOCA with safety injection blocked
- `disaster/corium_02_trip_then_break.txt` — corium_02: manual trip, then large-break LOCA with SI blocked
- `disaster/corium_03_fukushima_stripped_sbo.txt` — corium_03: SBO-style stripped injection (Fukushima class)
- `disaster/corium_04_porv_stuck_open.txt` — corium_04: PORV stuck open + safety valves + SI blocked
- `disaster/corium_05_makeup_shut_porv.txt` — corium_05: charging isolated + PORV stuck open + SI blocked; makeup shut removes the RCS refill leg while relief drains steam (avoided outcome: corium via `VesselFailed` latch; see cmdfile header)
- `disaster/corium_06_pressurizer_night_drain.txt` — corium_06: pressurizer night drain (heaters off, spray open, PORV stuck open)
- `disaster/corium_07_cold_shutdown_lockout.txt` — corium_07: cold-shutdown cavity lockout (negative control)
- `disaster/corium_08_hotleg_block_and_drain.txt` — corium_08: hot-leg block valve + pressurizer proportional drain
- `disaster/corium_16_both.txt` — corium_16: hot-leg break + SGTR both at t=0, SI blocked (combined fault)
- `disaster/corium_17_letdown_without_charging.txt` — corium_17: CVCS letdown without charging + SI blocked
- `disaster/corium_18_sgtr_si_block.txt` — corium_18: SG tube rupture alone + SI blocked (no depressurization)
- `disaster/corium_19_small_break_si_block.txt` — corium_19: small break (0.1-open) with SI blocked, untripped
- `disaster/corium_20_letdown_tripped_control.txt` — corium_20: tripped letdown without charging + SI blocked (negative control)
- `disaster/corium_24_porv_hotleg01_si_block.txt` — corium_24: stuck PORV + small hot-leg break (0.1-open), SI blocked
- `disaster/corium_25_letdown_porv_si_block.txt` — corium_25: letdown without charging + stuck PORV, SI blocked
- `disaster/corium_26_porv_sgtr_si_block.txt` — corium_26: stuck PORV + SG tube rupture, SI blocked
- `disaster/corium_27_auxspray_porv_si_block.txt` — corium_27: aux spray + stuck PORV, SI blocked

## Files in `recover/`

- `recover/corium_01_hotleg_si_block.txt` — corium_01: hot-leg break (recover: SI restore at 10 s)
- `recover/corium_04_porv_stuck_open.txt` — corium_04: PORV stuck open (recover: BlockValve isolate at 10 s)
- `recover/corium_16_both.txt` — corium_16: hot-leg break + SGTR combined (recover: SI restore at 10 s)
- `recover/corium_17_letdown_without_charging.txt` — corium_17: letdown without charging (recover: letdown close + charging + trip at 10 s)
- `recover/corium_18_sgtr_si_block.txt` — corium_18: SG tube rupture (recover: SI restore at 10 s, depressurize at 30 s)
- `recover/corium_19_small_break_si_block.txt` — corium_19: small break 0.1 (recover: trip + SI restore at 10 s)
- `recover/corium_24_porv_hotleg01_si_block.txt` — corium_24: PORV stuck + small break 0.1 (recover: BlockValve + trip + SI at 10 s)
- `recover/corium_25_letdown_porv_si_block.txt` — corium_25: letdown + PORV (recover: letdown close + BlockValve + trip at 10 s)
- `recover/corium_26_porv_sgtr_si_block.txt` — corium_26: PORV + SGTR (recover: BlockValve + SI at 10 s, depressurize at 30 s)
- `recover/corium_27_auxspray_porv_si_block.txt` — corium_27: aux spray + PORV (recover: spray close + BlockValve + trip at 10 s)

## Files in `damage/`

- `damage/damage_01_rcp_seal_overheat.txt` — damage_01: RCP seal overheat (charging lost while RCPs running)
- `damage/damage_02_efws_turbine_deadhead.txt` — damage_02: EFWS turbine-pump deadhead (recirculation isolated)

## Files in `contamination/`

- `contamination/contamination_01_sgtr_secondary_release.txt` — Path A (primary→secondary): SGTR + MSLB contaminates steamline, charcoal-delayed venting
- `contamination/contamination_02_hotleg_containment_release.txt` — Path B (primary→containment): hot-leg break entrains activity, CSS scrub to sump
- `contamination/contamination_03_containment_failure_release.txt` — Path B→C approach: relief overfill, PrimaryActivity enormous, containment holds
- `contamination/contamination_04_charcoal_delay.txt` — waste-gas holdup vs vented Bq (60-day delay observable)

Three-barrier framing: `specs/the_westinghouse_pressurized_water_react/chapter_6/page_107.txt:13-24` (zircaloy / RCS pressure boundary / containment vessel).
Path C (containment → environment, charcoal bypass) requires the `ContainmentFailed` latch; none of the
cmdfile windows reach it — it is Go-test-only (`TestPlant_HydrogenDeflagrationDrivesContainmentFailure`).

## Files in `envelope/`

Design-basis envelope scenarios: one actuation or failure mode per file,
with success/failure twins sharing a numeric prefix. Each header cites its
spec page and the `nsimrun` window used to validate it:

- `envelope/envelope_01_cv22_fail_open.txt` — CV22 pressurizer relief valve fails open (upset; `specs/pwr_sim/chapter_4/page_049.txt` §4.13); venting depressurizes, low-pressure trip cuts power
- `envelope/envelope_01_cv22_condenser_routing.txt` — CV22 routed to the letdown condenser (success twin); venting depressurizes without filling the PRT
- `envelope/envelope_02_dropped_rod_runback.txt` — dropped control-rod bank (upset/success; `specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt`); bank jams at 0.85 insertion, direct trip + runback, no clad damage
- `envelope/envelope_03_rcp_seal_failure.txt` — RCP seal injection failure (failure case; charging lost while RCPs run); seal faces overheat to Damage=1 in ~1,000 s (`SealHeatRatePerSec`)
- `envelope/envelope_03_rcp_seal_success.txt` — failure twin with `AuxSealCoolingStandby`; `SealHeatDamageFrac` frozen at 0
- `envelope/envelope_04_mid_loop.txt` — mid-loop refueling drain; lowers RCS inventory with pumps running, overpressurization disabled
- `envelope/envelope_04_rod_ejection.txt` — rod ejection (positive insertion; `specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt`); RPS rate trip catches it

## Disaster/recover pairs (same filename in both dirs)

| Pair | Disaster entry | Recover action @10 s | Verdict |
|---|---|---|---|
| `01` hot-leg | Large-break LOCA, SI blocked | SI restore + Reset (no-closing probe) | Creep; uncovery keeps growing (120 gal/s > ECCS) |
| `04` PORV | PORV stuck + safety, SI blocked | BlockValve isolate | True recovery: PRT pins 6,370 gal, P -> 2,250 psia |
| `16` combined | Hot-leg + SGTR both, SI blocked | SI restore (no-closing probe) | Tracks hot-leg-only run; not stable |
| `17` letdown | Letdown w/o charging, SI blocked | Letdown close + charging + trip | Pins 750 gal |
| `18` SGTR | Tube rupture alone, SI blocked | SI restore @10 s, depressurize @30 s | Slows draw; still open |
| `19` small break | 0.1-open, SI blocked, untripped | Trip + SI restore | 12 gal/s < ECCS once fission off |
| `24` dual-drain | PORV + 0.1 break, SI blocked | BlockValve + trip + SI restore | BNC pins 20 gal |
| `25` letdown+PORV | Letdown + relief, SI blocked | Close letdown, BlockValve, trip | Pins 770 gal |
| `26` PORV+SGTR | Dual non-creditable, SI blocked | BlockValve + SI + depress@30 s | Slows draw; still open |
| `27` spray+PORV | Spray cooldown + relief | Close spray, BlockValve, trip | Pins 20 gal |

Disaster-only (no recover twin by design): `02` (order-matters trip),
`03` (Fukushima SBO stripped), `05` (makeup+PORV), `06` (night drain),
`07` (cold-shutdown negative control), `08` (block+drain),
`20` (tripped letdown, negative control).

## Damage (`damage/`, non-corium pump damage)

| File | Mechanism | t -> damage = 1 | Rate |
|---|---|---|---|
| `damage_01_rcp_seal_overheat.txt` | RCP seal-face overheat, charging lost, all 4 RCPs running | 1,000 s | `SealHeatRatePerSec` |
| `damage_02_efws_turbine_deadhead.txt` | EFWS turbine-pump deadhead, recirculation shut | 2,000 s | `DeadHeadHeatRatePerSec` |

Go-test-only (no cmdfile): turbine blade erosion (700 h), SG water hammer
(20 events), clad fatigue (500 cycles), liner fatigue (cycles),
chloride SCC risk flag, H2 deflagration -> containment failure
(needs per-tick sink stripping; cmdfile probe 0%).
