# Plan skill — spec-driven simulation design notes

This repository writes **plan docs** (e.g. `plans/p1.txt`, `plans/p1_2.txt`,
`plans/p3_1.txt`) that describe the *next* implementation slice of the
nuclear-plant simulator before any code is written. Each plan is a small,
self-contained design document tied back to the
`specs/the_westinghouse_pressurized_water_react/` source material.

Use this skill whenever the task says "write a plan to `plans/pN_M.txt`" or
otherwise asks for a design/implementation/architecture plan that must cite
the spec.

## When to use

- User says: `do plans/pN.txt`, `implement plans/pN_M.txt`, or asks for a
  design/implementation/architecture plan tied to the spec.
- User asks to add, refactor, or extend any of: a `Step()` for a plant
  part, a state machine, or a part-to-part coupling.

## Prerequisite reads

| Path | Why |
|------|-----|
| `core/stepper.go` | `Stepper` interface (`Step`, `Update`) |
| `core/units.go` | `TimeStep = time.Millisecond`, unit typedefs |
| `core/parts.go` | `Part`, `Pump`, `Valve`, `Tank`, `DieselGenerator`, constants |
| `core/plant.go`, `core/building.go`, `core/outside.go` | composition topology |
| `core/<part>.go`, `core/<part>_test.go` | current behaviour of each modelled part |

The spec is the Westinghouse PWR document under
`specs/the_westinghouse_pressurized_water_react/`. Page numbers in the
printed document map to file numbers via `printed_page + 16` (see
`specs/the_westinghouse_pressurized_water_act/glossary.md`).

## Citation rules

1. **Always use full paths:**
   `specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:50-51`

2. **Verify every citation before writing the plan.** Read the actual
   `page_NNN.txt` file and confirm the line range contains the quoted text.
   Do not cite from memory.

3. **Page mapping:** `page_nnn.txt` where `nnn = printed_page + 16`.
   Cross-check with `specs/the_westinghouse_pressurized_water_react/toc.md`
   if uncertain.

4. **After writing the plan, run a citation audit.** For every citation,
   re-read the cited lines on disk. Fix any mismatches before submitting.

5. **Constants and equations:**
   - If the spec states a numeric value, cite the exact line.
   - If the value is derived (e.g. a 20-hour cooldown rate of
     (350−140)/20 = 10.5°F/hr), note it as "derived from" the design basis
     and show the arithmetic. Do not invent spec numbers.

## Plan document structure

```markdown
# Plan pN: <one-line summary>
## Scope
<prompt restatement + gap being closed>
## Kept / Cut
| Item | Rationale + citation |
## What "important aspects" means per the spec
<verbatim quotes + citations + dynamic justification>
## State to add / use
<struct fields with spec-justifying comments>
## Step functions
<pseudo-code or real code sketches>
## Wiring
<Plant.Step / MakePlant / BuildGraphFor changes>
## Why this is "important aspects" only
<trimming rationale>
## Spec references
<numbered list, full path:line-range>
## Implementation order
<file → file, dependency-aware>
## Acceptance criteria
<testable per-item list>
```

### "Kept / Cut" table

Place this near the top. Every row must have a spec citation or a clear
dynamics-based rationale.

### Verbose quotes

Use block-quote format for spec passages the plan hinges on, followed by
the citation on the next line. This makes the plan auditable without
opening the spec.

### Step functions

Prefer real Go code sketches over pseudo-code. The plan is the contract
for implementation; concrete signatures and field accesses reduce
ambiguity. Keep sketches minimal — omit error handling and edge-case
branches that do not affect the design decision.

### Wiring

Describe the call-site changes in `Plant.Step`, `MakePlant`, and
`BuildGraphFor`. Use the existing copy-into-nextPlant style as the
reference. No need to repeat boilerplate; only non-obvious edges.

### Constants

Declare all constants in `core/parts.go` near the top of the file or in a
dedicated const block. No inlined magic numbers in step-function code.
If a constant has no spec citation, label it "derived" and briefly
justify the magnitude.

## Anti-patterns

- Restating the user's prompt instead of answering it.
- Long prose summaries of the spec without file:line citations.
- Inventing spec numbers. If the spec does not state a value, say so.
- Adding "control system" or "UI" features to a simulation plan. The
  simulator models physics, not operator screens.
- Folding in unrelated cleanups. Each plan covers one slice.
- Renames for style. Only rename when spec vocabulary clearly differs
  from the code.
- Citing before reading. Always verify the line range on disk first.

## Iteration loop (write → audit → fix)

1. Write the plan with citations you believe are correct.
2. Read every cited `page_NNN.txt` range on disk.
3. Fix mismatches (wrong page, wrong line range, misquoted text).
4. Re-check equations and constants against the spec.
5. Submit only after the audit is clean.

## How to implement a plan

When asked to *implement* a plan:

1. Re-read the plan file. It is the contract.
2. Touch one file at a time.
3. After each meaningful change, run `go build ./...` and the relevant
   `go test ./core/...`.
4. The plan's "Acceptance criteria" become the test list. Write each as a
   `Test…` function that drives `Plant.Step()` end-to-end.
5. If a plan item cannot be implemented as written, surface it before
   silently changing scope.