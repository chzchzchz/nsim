# Skill: Analyzing a live nsimserve endpoint (read-only review)

Goal: Given a running `cmd/nsimserve` endpoint (the HTTP server in
`cmd/nsimserve/README.md`), produce a read-only after-action review
of what happened during a crisis/drill and how the operator handled
it — without mutating the plant. The four phases below are the whole
workflow; each produces a required artifact in the output directory.
An agent following this skill should be able to run it with no
follow-up clarification.

## 0. Inputs and ground rules

- **Endpoint address** (required). There is no default port; the
  server binary is `go run ./cmd/nsimserve/` with a `-addr` flag
  (default `127.0.0.1:8080`, overridden by the deployment; see
  `cmd/nsimserve/ENDPOINTS.md`). If the user does not give one,
  ask. Pass it as `BASE` below.
- **Output directory** (required). If the user does not name one,
  derive one from the session, not a fixed path: e.g.
  `/tmp/nsim-<branch>-<unique>/` (repo convention: binaries and
  artifacts under `/tmp/nsim-<branch-name>/`, never `/tmp/<binary>`
  directly). Every artifact below lands there. If it doesn't exist,
  `mkdir -p` it. Keep raw downloads (`plant.json`, `state.json`,
  …) in the output dir root and `chart_all.py`'s output in
  `<outdir>/charts/` so the per-step snapshots don't collide with
  the raw downloads (`--keep-json` writes `<seconds>.000.json`
  *into the out dir*, the same filenames a glob over the whole
  outdir would match).
- **Read-only means read-only.** Only use `GET` endpoints:
  `/status`, `/sensors` (bulk) or `/sensors/{key}` or
  `/controls/{key}`, `/logs` (with `?lo=&hi=`), `/alarms`,
  `/commands` (journal/queue view), `/commands/export`,
  `/state`, `/initial-state`, `/livez`, `/readyz`, `/metrics`.
  Never `POST`/`PUT`/`DELETE` — no `PUT /controls/{key}`,
  `POST /commands`, `POST /logs`, or any `/sim/*` lifecycle call.
  Reading the log is fine; writing to it is not. All reads are
  live reads of a *running* plant; a value you read now is not a
  promise about the next second.

### Tooling (which tools to use)

This is an agent/runtime environment; tool names differ by runtime,
so first list the tools you actually have (`crush_info` for Crush)
rather than guessing:

- **HTTP reads**: use the `fetch` tool when the runtime provides
  one; otherwise use `bash` with `curl -s <url>`. If neither
  exists, stop — this skill cannot work without an HTTP GET.
  (`skills/TOOLS.md` in this repo does **not** list `fetch` and its
  examples assume a different runtime — it is not the authority
  for this environment; trust the live toolset, not any checked-in
  tool doc.)
- **File reads**: use the file-reading tool the runtime exposes
  (`view` with `file_path`/`offset`/`limit` in Crush). There is
  no `read` tool. For content search use `grep` or `rg` (prefer
  the search tool for simple patterns, `rg` for large codebases).
  For listings use `ls`/`glob`.
- **One fetch per endpoint; parallelize the independent reads in
  `1.1`** as separate tool calls in the same turn.

**Pitfall:** don't parse raw JSON dumps by eye. Parse structurally
with `jq` (or Python `json` in bash) — `python3 -c "import json,
urllib.request; …"` is the pattern from this skill's authoring
session — and quote only the fields the artifact needs. A raw
`GET /state` dump is ~50 KB and will pollute your context; extract
the subsystems you cite.

## 1. Collect evidence (phase 1: current situation)

Before doing anything else, establish the present:

### 1.1 — Pull everything in parallel

```python
# step "1.1 — collect evidence" (all six fetches in one turn)
fetch BASE/status
fetch BASE/sensors
fetch BASE/controls
fetch BASE/logs
fetch BASE/alarms
fetch BASE/commands
```

Plus `fetch BASE/initial-state` and `fetch BASE/state`. Save
`initial-state`, `state`, `sensors`, `controls`, and `logs` to the
output directory as you fetch them (`*.json` in the artifact table
at the bottom).

### 1.2 — Read `/status`

Record, verbatim: `siteName`, `runId`, `simTimeMs` (the sim clock —
this is the "now" everything is measured against), `timestepMs`,
`stepsPerSecond`, `running`/`paused`, `rev`, and `latestSeq`.
`latestSeq` is the `seq` of the most recent `/logs` entry
(`-1` when the log is empty). A non-2xx response returns
`{"error":"<message>","code":<http status>}` — treat any of those
as "the server is not (yet) answering what you asked", not as data.

### 1.3 — Read sensors, controls, commands, alarms

- `GET /sensors` (bulk, one poll per frame) and `GET /controls`
  (the catalog of every control the server accepts, with live
  values — the "what operations exist?" endpoint). For single keys,
  use `/sensors/{key}` / `/controls/{key}` with `/` encoded as
  `%2F` (or use dot-form `Part.Label`, which the server also
  accepts).
- `GET /commands` — the journal/queue view. Queue `seq` is **not**
  log seq; `logSeq` is the id that references `/logs`. Failed
  writes show `status:"failed"` with `err` here.
- `GET /alarms` — `Plant.Alarms()`; empty `[]` means no active
  alarms.

### 1.4 — Read `/logs` and start tracking

`GET /logs` is the append-only audit trail: every cmdfile operation
(comment attached) plus bare `kind:"comment"` entries, in timestamp
order. Track the highest `seq` seen as `lastSeq`. Half-open range:
`?lo=<seq>&hi=<seq>` matches `seq >= lo and seq < hi`;
`lo=<lastSeq+1>` returns only new entries. Scenario-scripted
entries carry `"source":"script"`.

> **The log is a shared system.** Re-check `/status` and
> `GET /logs?lo=<lastSeq+1>` before every conclusion. If
> `latestSeq > lastSeq`, someone else wrote during your analysis —
> read the new entries, re-read the sensors you depend on, and
> update `lastSeq` before writing anything down. `seq` is one
> server-owned counter per run; it resets after a reset/restart
> unless `-resume` preserves it.

### Output 1 — `situation.md`

Confirm: `situation.md` exists in the output directory and contains
all of the following before moving to phase 2. If any item is
missing, phase 1 is not done.

- Identity: site name, run id, sim clock (`simTimeMs`), paused/running,
  latest seq, objective active or not.
- Headline state: reactor (power %, PZR pressure/level, RCS temps,
  clad temp, DNBR, uncovery, core damage, boron), SG (pressure,
  water vs setpoint), turbine (speed, power, generator breaker,
  grid), containment, and the subsystem the log centers on (CVCS:
  charging/letdown/VCT; feedwater: condensate pump, regulator,
  main valve; etc.).
- Alarms (or "none"), queued commands (or "none"), and any
  irreversible residue already visible (seal damage, emptied tanks).
- One-line assessment of whether the plant is stable or still
  evolving, with the evidence that supports it.

## 2. Diff against the initial state (phase 2)

`GET /initial-state` is a deep-copy snapshot of the plant taken at
run start (boot or last `POST /sim/reset`) and **never steps**
(`initialBytes` in `cmd/nsimserve/server.go` — only rewritten by a
reset/restore). `GET /state` is the same JSON shape, but it is the
**live** plant — a deep-copy snapshot taken at read time, not a
live mirror. Both are byte-compatible with `nsimrun -plant` input
(see `cmd/nsimserve/README.md`). It is exactly this frozen-vs-live
split that makes the diff meaningful. **Save both now**
(`plant.json`, `state.json` in the artifact table) — phase 4
reuses `plant.json` as its `--plant` input.

**Seed for phase 3 timing evidence:** for each phase-1 sensor
reading, note the `simTimeMs` it was taken at. Phase 3 verdicts
(e.g. response latency, "the log's claim that X was true then")
must be timed against `atMs`, not against wall-clock order.

### Output 2 — `initial-vs-current.md`

Confirm: `initial-vs-current.md` exists and contains all of the
following before moving to phase 3.

- A table of every parameter that moved: initial → current (Δ),
  grouped by system (RPV, PZR, SG, RCPs/seals, CVCS, secondary,
  turbine/grid, containment, electrical). Include the
  `BreakDischarged` total and any FOM/damage fractions.
- The **operator footprint**: controls whose values now differ from
  boot (`/controls` live values vs `/initial-state`), classified as
  (a) currently held, (b) transient (forced, then returned to auto),
  (c) never touched.
- The **irreversible deltas**: anything that cannot come back
  without a reset (damage fractions, emptied/depleted tanks,
  accumulated holdup like BRS, break-discharged totals). Frame
  these explicitly; they are the cost of the event.
- **Pre-existing faults in the initial state**: the initial plant
  may already contain faults (e.g. RCP seals in overheat alarm at
  boot, SG above its 50% setpoint). Call them out in a dedicated
  paragraph so the operator is not blamed for them.

## 3. Judge every log entry (phase 3: ranked analysis)

Read the full `/logs` (not a sample) and judge each **operation**
entry (`kind:"operation"` with `control`/`value`/`comment`) against
the initial state (phase 2), the plant physics, and the timing
evidence (op `atMs` vs `simTimeMs` progress). A bare
`kind:"comment"` is not judged as an action — it is judged as
communication quality (see "status notes").

### Output 3 — `log-analysis.md`

Confirm: `log-analysis.md` exists and contains all six of these
sections before moving to phase 4. A ranking that doesn't name the
deciding factor is not a ranking.

1. **Entry table** — every entry: `seq`, sim time (`atMs`), kind,
   control/value or comment gist. The reader should see the whole
   conversation, not your selection of it.
2. **Per-operation verdict**, with evidence, using exactly these
   four labels:
   - **Good** — right action, right reason. Cite the physics →
     spec reference where the log gives one; name the alternative
     that was rejected and the evidence that rejected it.
   - **Premature / too hot** — right direction, wrong magnitude or
     timing (e.g. a first-move demand that drains the makeup
     source in under a minute).
   - **Mixed** — right goal, wrong patient (e.g. stacking
     open-loop commands on a live auto controller that silently
     overrides them; the fix is going manual, or changing the
     setpoint — not more commands).
   - **Status note** — the quality of the periodic quantitative
     summaries: do they quote measured numbers at each phase
     boundary? Do they name what to watch? (These are what make
     the log an audit trail rather than a record.)
3. **Ranking** — best → worst, one line per operation, each line
   stating the deciding factor. Ranking is mandatory; it forces
   the judgment the reviewer is asking for.
4. **Gaps / never done** — the structurally important non-actions:
   root-cause diagnosis of the initiating fault (the log may quote
   a limit, e.g. Westinghouse Ops Ch19 ≤120 gpm letdown, without
   ever asking *why* it exceeded it, and re-enabling the faulting
   valve then re-arms it), alarm acknowledgement, trip/SI decision
   rationale ("trip not warranted because…"), chemistry
   verification after inventory restart, response latency (distress
   call → first action — either triage delay or scenario onset;
   say which), and **drift after the "stabilized" declaration**
   (silence is a decision too — name the slow-drifting plant state
   nobody is watching).
5. **Overall verdict** — one paragraph naming the strongest phase
   of the response, the single clearest flaw, and the weakest
   phase.
6. **Qualitative summary** — a non-technical narrative, separate from
   the technical verdicts above, that a non-operator reader can
   follow. Write it from the log's own evidence (phase-1/2 numbers
   and phase-3 verdicts), not as a restatement of the table.
   Must name, in plain language: (a) what went wrong and when,
   (b) the contrasting events — what the operator did that helped
   vs. what made things worse, (c) how the plant ended up (stable
   / damaged but bounded / still evolving), and (d) one sentence on
   whether the record is trustworthy (audit-trail quality). Keep it
   3–5 short paragraphs or ~150–250 words; label it
   `## Qualitative summary` so it is findable without the technical
   sections. If any number is quoted, it must be live-current or
   replay and labeled as such — never a replay number presented
   as a live-plant number. Run the same `python -m py_compile`
   discipline you use for chart scripts: after writing
   `log-analysis.md`, py_compile any helper script you used to
   produce it and note the compile in the artifact's first
   paragraph (e.g. "compiled with `python -m py_compile`").

### Reference points for judging (from the Crisis 17 record)

- Isolate the source first (letdown isolation was the correct
  first move against a 4500 gpm vs 120 gpm-design-max runaway).
- Restore cooling/injection *with* inventory (charging pump for
  both seal injection and PZR level; aux seal cooling as a bridge
  while charging ramps).
- A first-move demand that empties the only makeup source (VCT)
  in under a minute is too hot — trim on observed trend data and
  let automation (AutoLevelControl) hold the loop. Watch the
  window after enabling automation: with letdown still isolated
  the controller's only actuator is charging, and it briefly
  overshot (15 → 30 gpm).
- When auto fights you (demand zeroed silently), the fix is manual
  control, not more open-loop commands; hand back to auto once
  recovery is proven, not on hope.
- Status comments at every phase boundary are what make the log
  auditable; a "do not relax X" recommendation protects the next
  operator.
- When a phase's success criterion is met (SG recovery, seals
  frozen), saying so explicitly closes the decision — the log
  should not leave it to be inferred.

## 4. Generate charts with `scripts/chart_all.py` (phase 4)

`scripts/chart_all.py` (shim: `scripts/chart.sh`) replays a cmdfile
from an initial plant with chunked `nsimrun` runs and charts every
numeric sensor over time (SVG fallback when matplotlib is absent;
otherwise PNG; always writes `index.html` with a min/max/final
table). Invariant from `cmd/nsimserve/README.md`: `GET /commands/export`
→ "Round-trip: export and replay it through `nsimrun`".

> **Replay inputs — always use `/initial-state`.** Any replay
> (chart or batch `nsimrun`) of the sampled session starts from
> the **frozen boot snapshot** (`plant.json` from phase 1), never
> from `GET /state`. `/state` is the live plant at the moment
> you read it — starting a replay from it makes the replay answer
> "what if I freeze the plant *now*?", not "what happened during
> this run?". The one exception: replaying from a *chosen
> intermediate* plant (e.g. a snapshot taken during an earlier
> phase-4 run) is legitimate if the user asks for it — say so in
> the findings. `GET /state` on its own is fine as a comparison
> source in phase 2.

### 4.1 — Save the inputs

```bash
# plant.json must be /initial-state verbatim; it is chart_all.py's --plant.
curl -s http://127.0.0.1:8080/initial-state > <outdir>/plant.json
curl -s http://127.0.0.1:8080/commands/export > <outdir>/cmds.json
```

The export is the JSON envelope: `{scenario, commands:[{offsetMs,
control, value, comment}]}`. `offsetMs` is the absolute sim time
it takes effect (rebased in `Journal.CommandsFrom(0)` — see
`cmdfile/journal.go:61-66`). Two consequences an agent must know:

- **The export only ever carries `offsetMs`.** `_parse_cmdfile`
  (`scripts/chart_all.py:93-101`) reads `offsetMs` and discards
  any `atMs` field. The server's `POST /commands` accepts `atMs`,
  but the *export* re-bases it — so the export is never going to
  lose commands. If you are hand-writing a JSON cmdfile, write
  `offsetMs`, not `atMs`.
- **`scenario` may be empty.** When the server was started with
  `-scenario` (or `-plant`), the export's `scenario` field is
  still empty (`journal.CommandsFrom` doesn't stamp it). In that
  case `plant.json` already encodes the scenario lineup, so you
  MUST pass `--plant plant.json` and must NOT pass `--scenario`
  (they are mutually exclusive — see below):

```bash
python3 scripts/chart_all.py \
  --cmd <outdir>/cmds.json --plant <outdir>/plant.json --seconds 330 --step 5 --out <outdir>/charts --keep-json
```

**Mutual-exclusion rule (why):** `plantinit.BuildInitialPlant`
(`internal/plantinit/plantinit.go:54-58`) rejects `-scenario` +
`-plant` together; `nsimrun` likewise rejects an explicit
`-scenario` that conflicts with a non-empty export scenario
(`cmd/nsimrun/main.go:53-60`). `chart_all.py` enforces the same
pairwise check (`main()`: *"error: --scenario and --plant are
mutually exclusive"*). So the rule of thumb: **export scenario
non-empty → pass `--scenario` only; otherwise pass `--plant`
only.** If you get the mutual-exclusion error, that is the bug —
do not retry with both flags.

### 4.2 — Run chart_all.py

Flags (`scripts/chart_all.py:_parse_args`, `scripts/chart.sh` is
just a shim): `--cmd`, `--plant`, `--scenario`, `--seconds`
(total sim seconds; default is last command's offset + 1 step —
sufficient for a scripted-sequence review), `--step` (seconds
between snapshots; 5 s is fine for a multi-minute run), `--out`,
`--sensors`, `--bin`, `--keep-json`, `--no-index`, `--list`.

```bash
python3 scripts/chart_all.py \
  --cmd <outdir>/cmds.json --plant <outdir>/plant.json \
  --seconds <lastAtMs/1000 + margin> \
  --step 5 \
  --out <outdir>/charts \
  --keep-json
```

- `--keep-json` keeps per-step `.json` snapshots in the out dir,
  named `%06.3f.json` % sim time — e.g. `0000.000.json`,
  `0005.000.json`, …, `0330.000.json`. These are inside `charts/`
  (because `--out` is `charts/`). **They sort lexicographically
  as numbers** (`%06.3f` zero-pads the seconds field; 4-digit
  seconds still sort correctly up to 99999 s). Quote numbers
  from these, not from the `index.html` min/max table.
- `--seconds` in `nsimrun` is decimal seconds (`5`, `1.5`) — never
  `"1s"`. `chart_all.py` handles the command-window split for
  you; you only supply the end time.
- `--sensors` accepts a comma-separated list of **prefix
  patterns only**: `*` is only a trailing wildcard
  (`name.startswith(pat[:-1])`); leading `.` is optional and
  stripped; patterns without `*` match the name exactly or as a
  prefix ending in `.`. An internal `*` is treated as a literal
  character and will match nothing. Default (all sensors) is
  preferred for a crisis review.
- The run builds `nsimrun` once via `go build ./cmd/nsimrun/` and
  prints `charted N sensors from M snapshots in <out> (K png, L
  svg)` to **stderr** — that line is your confirmation.
- Allow minutes, not seconds: the run is real-time-per-step churn
  through `nsimrun` chunks and may take 30–90 s (longer if
  `--bin` isn't prebuilt and the machine is cold). Run it as a
  background command and poll it; do not conclude it hung from a
  single empty output. Progress is visible in the per-step
  `.json` files appearing in the out dir.

### Output 4 — `chart-findings.md`

Confirm: `chart-findings.md` exists next to (or inside)
`charts/` and contains all of the following.

- What was generated: the chart script's stderr line (sensor count,
  snapshot count, PNG/SVG split) and the `charts/index.html`
  location.
- **The ~5 event trajectories** (read exact numbers from the
  per-step JSONs — never the index table alone). To read a
  series: open the per-step JSONs in time order and read the leaf
  values. **Leaf shapes**: scalar fields are plain numbers
  (`TurbineBuilding.FeedwaterRegulator.FeedwaterFlow`,
  `….SealHeatDamageFrac`); unit-bearing quantities are objects
  `{"val": <number>, "unit": "<unit>"}` — so their value is
  `….Water.val`, `….VCTLevel.val`, etc. (Full set from the
  authoring session; all were verified against the real
  snapshots:)

  ```
  ReactorBuilding.Pressurizer.Water.val                  # PZR water, gal
  ReactorBuilding.SteamGenerator.Water.val               # SG water, gal
  AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.val
  ReactorBuilding.ReactorCoolantPumps.LoopA.Damage       # seal damage (plain number)
  TurbineBuilding.FeedwaterRegulator.FeedwaterFlow       # plain number, gal/s
  ReactorBuilding.ReactorPressureVessel.CladTempF       # plain number, degF
  ReactorBuilding.ReactorPressureVessel.UncoveryFrac     # plain number
  ```

  The trajectories for a crisis look like (quote exact numbers):
  the initiating variable, its extremum and timing, the operator's
  response, and the settle — e.g. PZR water 825→0→900 gal; seal
  damage →10.6% frozen; SG water 79.9k→52.6k→68.2k→62.1k; VCT →0
  at t≈145 s; charging 0→15→30→~0 gpm.
- **Confirmation/refutation**: for each verdict from phase 3, say
  whether the charts proved it (e.g. the 304 gpm feedwater flow
  during manual feed, or the AutoLevelControl 15→30 gpm overshoot —
  both visible only in trajectories, not in the log).
- Any **controller behavior** the charts reveal that the log
  doesn't mention (setpoint overshoot on controller enable, auto
  correctly cutting feed above setpoint, drift after hand-back to
  auto).

### Replay-fidelity caveat (always state it)

`chart_all.py` re-simulates from `/initial-state` with only the
exported commands applied. If the initiating fault (e.g. the
4500 gpm letdown runaway) is an exogenous scenario event rather
than an initial-state field, the replay drains through the
baseline value (75 gpm) instead and the trajectories match the log
qualitatively, not bit-for-bit (seal damage 10.60% replay vs
10.39% live is typical). Replay numbers are re-simulation
numbers — never present them as live-plant numbers; label which
is which.

The replay also exists as a plain batch alternative, per
`cmd/nsimserve/README.md`:

```bash
curl localhost:8080/commands/export > session.json
go run ./cmd/nsimrun/ -cmd session.json -seconds 60
```

## Artifact checklist

Everything below lands in the user-given output directory:

| File | Source |
|---|---|
| `plant.json` (outdir root) | `GET /initial-state` (verbatim — this is what `chart_all.py --plant` consumes) |
| `state.json` (outdir root) | `GET /state` (optional but useful) |
| `logs.json` (outdir root) | `GET /logs` |
| `cmds.json` (outdir root) | `GET /commands/export` |
| `sensors.json`, `controls.json` (outdir root) | bulk live reads |
| `situation.md` (outdir root) | phase 1 (confirm: identity, headline state, alarms/queue, stability call) |
| `initial-vs-current.md` (outdir root) | phase 2 (confirm: delta table, operator footprint, irreversible deltas, pre-existing faults) |
| `log-analysis.md` (outdir root) | phase 3 (confirm: entry table, per-op verdicts, ranking, gaps, overall verdict, qualitative summary) |
| `charts/` (`index.html` + SVGs/PNGs + `NNNN.NNN.json` per-step snapshots) | phase 4 via `scripts/chart_all.py` |
| `chart-findings.md` (next to or inside `charts/`) | phase 4 commentary (confirm: stderr line, ~5 trajectories with numbers, per-verdict confirmation, controller behavior, replay caveat) |

## Pitfalls this skill exists to prevent

- **Treating the log as static.** `latestSeq` vs `lastSeq` tells
  you whether the shared system changed under you. Always
  re-poll before concluding.
- **Confusing queue seq with log seq.** `/commands` responses give
  `seq` (queue) and `logSeq` (the `/logs` id). Cite `logSeq`
  when referring to the log.
- **Crediting the replay as the live trajectory.** It is a
  re-simulation from the initial state; the exogenous initiating
  event is not in it (phase 4 caveat). Never present replay
  numbers as live-plant numbers; label which is which.
- **Skipping phase 2.** Without the initial-state diff you cannot
  tell what the plant *was* — e.g. that the seals were already in
  alarm at boot, or that SG started at 79.9% (not the 50%
  setpoint).
- **Judging actions without their alternatives.** A good verdict
  names the rejected option and the evidence that rejected it.
- **Omitting the qualitative summary.** The per-operation verdicts
  are for the operator; the qualitative summary (phase 3, §6) is the
  only part a non-operator reader will trust. Do not ship a
  `log-analysis.md` without it, and do not copy numbers into it
  without labeling live-current vs. replay.
- **Declaring stabilized without a drift check.** After the last
  log entry, re-read `/sensors`; tanks emptying or levels
  drifting toward a setpoint with no operator watching are
  findings.
- **Passing both `--plant` and `--scenario` to chart_all.py /
  nsimrun.** They are mutually exclusive; pick per the
  export-scenario rule in §4.1.
- **Reading an `index.html` min/max table as trajectory
  evidence.** Only per-step JSONs show timing.
- **Replaying from `/state`.** `/state` is the live plant at read
  time; any replay (chart or batch) starts from `/initial-state`
  (frozen boot snapshot), because a replay answers "what happened
  during this run?", not "what if I freeze the plant now?".
