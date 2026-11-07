# Description skill — operator help text for controls, sensors, alarms, scenarios

This repository documents every operator-facing element through small
markdown files under `assets/descriptions/`. One file per element, keyed by
the element's id. The web UI renders them through `(?)` buttons with
cross-reference navigation. Use this skill whenever asked to add, expand,
or edit description documentation - whether the subject is a control, a
sensor, an **alarm**, or a **procedure scenario**.

## Where descriptions live

Documentation assets follow the same layout as `assets/descriptions/`: a
directory tree whose paths mirror element ids, one markdown file per id.
Each element kind gets its own top-level directory under `assets/`:

| Element kind | Asset root | Example path |
|---|---|---|
| Bridge control/sensor | `assets/descriptions/` | `ReactorBuilding/Pressurizer/HeatersOn.md` |
| Alarm | `assets/alarms/` | `assets/alarms/turbine.trip_vacuum.md` |
| Procedure scenario | `assets/scenarios/` | `assets/scenarios/TurbineTrip.md` |

Alarm ids use the Go convention `<subsystem>.<condition>` (e.g.
`turbine.trip_vacuum`, `rps.sg_low_level`). Dots in file names carry no
hierarchy - the flat file IS the id; only slashes create subdirectories.

Inside each root, subdirectories group by part/category exactly the way
`descriptions/` groups by Part path. Conventions below apply identically
to every root.

Any documented entity can link to any other by using its full id as the
markdown link href, provided ids stay unique across roots.

## Pipeline

1. Edit or add `.md` files. No registration step exists - dropping a file
   adds help; deleting one removes it.
2. `cd web && make descriptions` rebuilds `web/public/descriptions.json`
   (gitignored artifact) via `web/scripts/gen-descriptions.mjs`, which
   walks **every** documentation root, trims contents, sorts keys.
   Adding a new root means adding it to the script's walk list and the
   Makefile `DESC_SRCS` find paths.
3. The UI fetches `/descriptions.json` once per page load. A missing entry
   simply means no `(?)` button; nothing errors.

Generated artifacts are never committed.

## Where to find source material

Full paths, verified on disk before quoting. Page mapping for the
Westinghouse document: `page_nnn.txt` where `nnn = printed_page + 16`.

| Path | Best for |
|---|---|
| `specs/the_westinghouse_pressurized_water_react/` | System design and setpoints. Ch3 RCS/pressurizer, ch4 CVCS/BTRS/RHR, ch5 SI/EFW/CCW operation, ch6 containment/spray, ch7 instrumentation and control, ch11 electrical, ch12 auxiliary cooling, ch15 turbine-generator, ch16 plant operation. |
| `specs/pwr_sim/` | Simulator screen behavior and RPC pop-ups (ch2 reactor control screens, ch3 power maneuvering). Cite these for what the *screen* does. |
| `specs/westinghouse_plant_operations/chapter_19/` | Step-by-step startup/shutdown procedure detail (steam bubble formation, RCP starting permissives, criticality approach). Best source for procedural color. |
| `specs/westinghouse_tech_manual/` | Secondary-side details missing from the main doc (e.g. cooling tower, ch14). |
| `specs/wiki/` | Cached physics articles (`Decay_heat`, `Xenon-135`, `Cooling_tower`, ...). Use to explain phenomena, cite sparingly. |

The existing `core/bridge*.go` registration comments carry ready-made
citations per control; reuse them rather than re-searching.

## Writing guidelines

### Structure (per file)

1. **Paragraph 1 - what it is**: one to three sentences naming the thing
   and what the operator does with it. No preamble, start with the verb or
   noun that matters.
2. **Paragraph 2+ - procedural/operational detail**: setpoints, sequence
   position, interlocks, what its movement means, how failures present.
   This is where the value lives; draw from the operation sections of the
   specs, not the design tables alone.
3. **Numbers beat adjectives**: prefer concrete values over qualitative
   words. "Trips at 110 percent of rated speed (1980 rpm)" teaches more
   than "trips on overspeed". Every threshold should carry its value and
   units - 220F, 15 psi, 8.0 psia, 50 gallons, 600 psig - sourced from the
   spec or from the simulator's named constants.
4. **Model note** (required when the model diverges): flag simplifications
   ("the bridge exposes aggregate position mostly for visibility") so
   readers do not over-train on the model, and flag elements that are
   declared but not yet wired: "this annunciator is declared but not yet
   connected to a process condition - watch [X](id) directly." A model
   plate that is derived rather than spec-quoted says so in place:
   "simulator-derived typical journal-oil value; the reference names the
   trip but states no number."
5. **Related sections**: `Related controls:` and/or `Related sensors:`
   lines (see below).
6. **Source lines**: citations, one per line, last (see Citations).

Aim for 2-3 substantive paragraphs (~120-200 words). More when the element
genuinely anchors a procedure; less only for trivial indicators.

### Numbers and provenance

- Hunt thresholds where they live: the spec's system-operation sections
  first, then the simulator's named constants (`core/parts.go`,
  `core/rps.go` - e.g. OPC alarm/trip at 103/110 percent of rated speed,
  vacuum trip plate 8.0 psia, diesel coolant 220F, low fuel 50 of 500 gal,
  steamline trip 600 psig, Tcold trip 530F). Constants carry their own
  derivation comments; reuse them instead of re-deriving.
- Attribute honestly: spec-quoted numbers get `Source:` lines;
  simulator-derived plates get an inline "model note" saying so. Never
  dress a code constant up as a spec citation or vice versa.
- If no source states a value, either derive it transparently in a model
  note or drop the number entirely - "high" without a setpoint is a last
  resort, not a default.

### Jargon

Define terms inline on first use, briefly, without breaking stride:

> "...a steam bubble is established in the pressurizer - a cushion of
> saturated steam above the water that gives the primary something
> compressible to push against..."

Expand an acronym once (PORV, Tavg, miniflow, shutoff head, swell/shrink).
Assume a smart engineer who is not a PWR operator.

### Markdown conventions

- Plain prose paragraphs separated by blank lines. Lists only when the
  content is genuinely enumerable (mode names, sequence steps).
- `**bold**` for mode/enum values the reader selects (**Boration**,
  **ColdShutdown**).
- Cross-references: `[Label](<full id>)`. Clicking jumps to that element's
  row and flashes it; if the id has no rendered row the popup swaps to the
  target text instead. Never let a link dead-end - every href must exist
  as a file, in any root.
- Link labels come from the meaningful segment; when the last segment is
  generic (`Open`, `Shut`, `Running`), use the parent segment
  (`[MakeupValve](Outside/CoolingTower/MakeupValve/Open)`).
- Weave 1-3 inline links into the body where they genuinely aid reading,
  plus the Related lines. Do not decorate every noun.
- Never emit a single mixed `See also:` line. References are always
  classified into `Related controls:` / `Related sensors:` lines.
- No HTML, no images, no headings inside a file, no tables. These render
  in a small inline popup.
- ASCII punctuation: commas and periods instead of em dashes; straight
  quotes; `F`/`C` or spelled units are fine.

### Related sections

Classify references against the bridge registrations (grep
`m["…"] = &bridge.{Control,Sensor}` in `core/bridge*.go`). An id registered
as both kinds appears under both headings. Only emit a section when
non-empty:

```
Related controls: [AutoLevelControl](AuxiliaryBuilding/ChemicalVolumeControlSystem/AutoLevelControl)
Related sensors: [ProgrammedLevel](AuxiliaryBuilding/ChemicalVolumeControlSystem/ProgrammedLevel), ...
```

Choose relations by operational coupling (what the operator would check,
actuate, or blame next), not by directory proximity: protection chains
(PORV -> PRT -> rupture disc -> containment), support dependencies
(RHR -> CCW -> SWS), redundancy siblings (Train A <-> Train B), backup
paths (MFP -> EFW). For alarms, relate the initiating sensor, the
automatic action taken, and the controls the operator drives in response.
For scenarios, relate the key controls the operator must drive and the
alarms they expect. Alarm-to-alarm links are fine - ids are unique across
roots, so classification still uses the bridge registration sets while
hrefs may target any documented entity.

### Citations

- End every file with `Source:` lines, one per line, most authoritative
  first. Format:
  `Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:159-164`
- Verify line ranges on disk before writing them. Reuse registrations'
  existing citations when they cover the claim.
- Pure UI semantics (acknowledge-all button behavior) may omit sources;
  anything physical or numeric needs at least one.
- Never invent numbers. If the spec does not state a value, either derive
  it transparently or drop it.

## Editing workflow

1. Locate the file(s); read the current body fully.
2. Preserve `Related …` and `Source:` lines verbatim when rewriting bodies
   (scripted rewrites should split on the first `Related ` line and
   re-append the trailer unchanged).
3. After edits: `cd web && make descriptions`.
4. Validate:
   - Coverage both ways vs registrations when bridge ids changed:
     diff the registered ids against each root's `.md` tree. Always
     pass `--no-filename` when extracting across multiple files -
     without it rg prefixes every match with `core/bridge_aux.go:`
     and the diff reports every id as missing:

     ```bash
     rg -o --no-filename 'm\["([^"]+)"\] = &bridge\.(Control|Sensor)' \
        -r '$1' core/bridge*.go | sort -u > /tmp/reg.txt
     find assets/descriptions -name '*.md' | \
        sed 's|^assets/descriptions/||; s|\.md$||' | sort > /tmp/doc.txt
    comm -23 /tmp/reg.txt /tmp/doc.txt   # registrations missing docs
      comm -13 /tmp/reg.txt /tmp/doc.txt   # docs without registrations
      ```

      Note: `rg` only matches string-literal IDs. When a bridge file
      constructs IDs inside a loop (`m[afw.id+"/Running"]`), those
      dynamically-built entries will not appear in `/tmp/reg.txt`; verify
      the loop body manually rather than trusting `comm` to flag them.

      For alarm roots, diff against the alarm id source
     (e.g. `core/alarm.go`) instead.
   - Broken-link scan: every `](target)` resolves to a real `.md` in any
     root (ids are globally unique across roots).
   - `npx tsc --noEmit` if TS was touched.
5. Descriptions are documentation, but they ship in the release bundle -
   run `make build` (or at least the vite build) if layout/CSS changed.

## Anti-patterns

- Restating the label ("HeatersOn: turns heaters on"). Say what it does to
  the plant and when an operator reaches for it.
- Textbook chapters. Two tight paragraphs beat five loose ones; the specs
  remain the deep reference.
- Unexplained jargon in paragraph one.
- Qualitative thresholds when a number exists: "trips on overspeed" with
  103/110 percent sitting in the constants is a missed teaching moment.
- Presenting a simulator-derived plate as a spec value, or leaving readers
  to assume a declared-but-unwired annunciator will actually sound.
- Machinery where prose would do: prefer a well-written paragraph plus
  cross-reference links over building new tooling for presentation.
- Citations copied from memory or from another file without checking the
  range still says what you claim.
- Orphan files after renames, dead-link hrefs, duplicate labels like
  `[Open](a)` and `[Open](b)` in one line, or one unclassified `See also:`
  line instead of the two Related sections.
- Committing `web/public/descriptions.json` or anything under `plans/` or
  `specs/`.

## Quick checklist

- [ ] Body explains function, then procedure, then consequences.
- [ ] Concrete values (with units) wherever thresholds exist; derivation
      labeled when simulator-sourced.
- [ ] Jargon expanded on first use.
- [ ] 2+ paragraphs unless trivially indicative.
- [ ] Inline links where reading benefits; labels meaningful.
- [ ] Related controls/sensors classified and non-empty sections only.
- [ ] Source lines verified on disk; model notes present for divergence
      and unwired elements.
- [ ] `make descriptions` run; link scan clean.
