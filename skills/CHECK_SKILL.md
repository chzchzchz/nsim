# Skill: Double-Checking Simulation and Model Code

A verification checklist for changes to serialization, numeric models,
state layout, or their tests. Born from real failures: silently dropped
struct fields, decay heat amplified by reactivity, mixed-unit sums
pinned by safety clamps, tests that passed *because of* the bug,
trip latches with no reset path, per-field writes erased by a later
whole-struct assignment, and a flow-graph node acting as an infinite
water source.

Core principle: **in these systems the dangerous failures are silent.**
Serialization drops fields without error; wrong physics produces
plausible-looking numbers; clamps hide divergence; spot-check tests
pass while the rest of the object is corrupt. Verify actively,
empirically, at the strongest available assertion strength.

Applying the checklist is only half the loop; feeding new generic
lessons back into this file is the other half (see section 10).

## 1. Serialization round-trips

`encoding/json` reports *usage* errors loudly but *design* flaws
silently. Two mechanisms drop data without any error:

- **Method promotion through embedding**: an embedded base type with
  `MarshalJSON`/`UnmarshalJSON` transmits those methods to every
  container that embeds it, transitively — the container serializes as
  whatever the leaf's format is, discarding sibling fields.
- **Name collisions among flattened anonymous embeds**: same-named
  fields contributed by two embedded structs (`Pressure`, `Water`, ...)
  cause `encoding/json` to drop *all* of them.

Checks:
- Round-trip and deep-compare the **whole object**, plus representative
  leaves (bools, strings, unit-bearing numerics). Do not trust
  re-marshal byte-equality alone: a lossy-but-idempotent collapse
  reproduces identical bytes on the second pass — compare in Go space
  against the original or closed-form expectations.
- After marshaling, grep the output for keys that must exist. Missing
  key == dropped field.
- Fix mechanisms, not symptoms: named fields or json tags break
  unwanted promotion/collisions; per-type custom marshalers scale badly
  (an `UnmarshalJSON` on a shared base poisons every embedder).
- **Editing a saved state through an embedded type's name is a silent
  no-op.** Anonymous embeds flatten their fields onto the container in
  JSON, so `RPV.ControlRods.Insertion` writes a `"ControlRods"` key
  nothing decodes while the flattened `"Insertion"` key keeps its old
  value - the run then executes a different plant than the file shows.
  Edit flattened paths, and verify by reading back the DECODED plant,
  not the edited file.
- **A pointer field that Step re-points each tick deserializes as a
  detached duplicate.** The JSON carries both the master struct and
  the mirror (e.g. a building aliased as `Aux` during simulation), the
  file looks complete either way, and pre-step reads and writes hit
  whichever copy was decoded last while the other silently wins on the
  first Step - which is why authors end up seeding both copies.
  Reconcile such aliases in `UnmarshalJSON`: re-point the mirror at
  the master and refresh propagated copies so one seed is
  authoritative.

## 2. Units, scales, and magnitudes

- **One scale per sum.** Convert every contributor explicitly and write
  the conversion in the comment; percent-of-one-thing plus
  percent-of-another still type-checks while producing garbage.
- **One name per physical quantity, per conversion factor.** Near-name
  twins (`CondenserCWRise` vs `CondenserDesignCWRiseF`) can carry
  different values indefinitely; the same factor spelled as literals in
  two files has already drifted. Define once, derive when the
  relationship is real, convert every site to the name. A ratio like
  `0.05 / 60.0` is legitimate when the comment states the units;
  convert between scales once, at the storage boundary, with the unit
  in the identifier (`FlowGPM`).
- **Clamps, caps, and floors mask divergence** — a runaway pinned at a
  rated cap looks stable. Sample raw intermediates over time, not just
  clamped outputs. A value parked exactly at a boundary forever is a
  diagnosis, not a steady state: find the driver (often an
  infinite-capacity source node feeding a finite tank) and ask whether
  the coupling is physically real. When TWO regimes both pin at the
  same operational envelope (a shared per-step rate limit), no
  temperature- or level-delta assertion can separate them — the deltas
  converge to the clamp; witness the separation at the level where the
  constraint binds differently (whole-plant demand vs capacity), or
  assert the binding arithmetic directly.
- **Close the ledger after wiring any new transfer path.** Sum per-step
  deltas over every volume touched; the total must be conserved or land
  in an explicitly-labelled sink. When the balanced node clamps at a
  bound, keep the ledger in a connected unclamped node (a surge tank)
  and assert its level returns to start over whole cycles — a pinned
  tank absorbs imbalance silently.
- **A draw path sized against capacity instead of remaining inventory
  resurrects its source every tick.** When the writer recomputes free
  space as `capacity - level`, a drain that reads the capacity field as
  available mass never runs dry: it pins at its rate forever, feeding
  phantom mass (and phantom downstream heating) while every level reads
  plausibly. Bound cumulative draws against the drained system's own
  inventory anchor, persist the accumulator, and add an exhaustion
  regression asserting the flow reaches exactly zero — a per-step
  discharge parked at its cap for the rest of a long run is the
  signature.
- **Verify a flow constant's effective unit empirically.** A constant
  commented "gpm" but integrated as `rate * dt.Seconds()` acts as
  gal/s; gal/hr contributes nothing until divided by 3600. Compute the
  expected displacement over a known window and check one real run.
  Same discipline across subsystem boundaries: if the two sides encode
  "the same" design flow in conventions differing by orders of
  magnitude, pass a dimensionless fraction of rated capacity instead of
  either side's absolute number.
- **A consumer exchanging flow with a producer inherits the producer's
  integration convention, not the spec's printed unit.** Faithfully
  converting the spec figure (÷60 for gpm) while the producer side
  accumulates per-step units makes the consumer hundreds of times too
  slow — and it reads as plausible slow processing, not a bug, because
  every local number is self-consistent. Catch: compute time-to-drain
  across the interface from the producer's accumulation rate; a
  consumer that cannot keep up by at most a stated design factor is
  mis-scaled.
- Sweep for thresholds compared against channels whose reachable range
  excludes them (a <400 psig entry condition on a channel floored at
  1500 psia never fires). Check against reachable min/max, not the
  nominal unit of the constant. Same audit for *actuator authority*:
  condensation-style pressure paths are bounded by their vapor-space
  budget (psi-drop <= bubble x stiffness), so a depressurization demand
  larger than the seeded bubble stalls silently on a plateau instead of
  erroring — integrate the budget across the whole trajectory, not one
  step.
- **Anchor every derived-ratio readout to one known-state sample before
  trusting its trend.** A percentage computed with a wrong divisor
  produces plausible-looking trajectories for many iterations; compare
  one reading against an independent absolute anchor (a rated cap, a
  design value) before drawing any conclusion from the series.
- **Multiplicative per-step updates amplify tiny bias.** A chain scaled
  by `1+rho/100` every millisecond turns a sustained few-pcm defect
  into percent-per-second drift while the driving signal reads as
  near-zero. When a "parked" plant still moves, compare the implied
  drift rate (log power ratio over the window) against the displayed
  signal before trusting either.
- Sign-check feedback loops: does the effect oppose (stable) or
  reinforce (runaway)? Does the sign flip between regimes?
- **Anchor a capacity reference to the producer it must balance.** A
  consumer-side rating (turbine design draw, dump capacity) normalized
  against a producer-side rating derived from different constants
  (core power / loop count vs per-SG spec flow) makes every full-load
  equilibrium sag quietly onto whatever fraction the mismatch allows.
  At nameplate the two scales must close exactly; check by asserting
  the full-load steady state lands on the spec's quoted operating
  point, not near it.

## 3. Regime separation

- Different physical processes need different state and different
  equations; multiplying one regime's curve by another regime's factor
  (decay heat × critical-power defect) is a category error even when
  results look plausible. Prefer formulations where one set of
  equations holds everywhere and regime changes enter only through
  inputs — special-cased flag branches hide whether the physics
  produces the transition, and removing a special case exposes legacy
  calibrations that only made sense under the old structure.
- When the spec assigns two actuators to two variables, the model needs
  matching degrees of freedom: a pressure defined as a pure function of
  inventory makes "hold pressure while draining level" unsatisfiable.
- An on-off or deadband supervisor wrapped around an exponentially
  unstable inner chain cannot hold setpoints whose deadband transit
  time exceeds the inner instability's e-folding - inside the band,
  "do nothing" is still an actuator action with consequences. Give the
  outer law a balance-preserving nulling baseline and probe hold
  behavior inside the deadband with a sampled trajectory, not just
  correction outside it.
- Compose a multi-input actuator demand inside the supply limit, not
  beside it: `max(demandTerm, limitedTerm)` lets whichever term
  saturates bypass the constraint the other was supposed to enforce
  (a Tavg-restoration term holding a dump open straight through a
  header floor). Apply the limiter to the combined demand and test
  each input with the sibling pinned at its extreme.
- A protection channel whose process variable sits legitimately at its
  trip value across a whole regime turns every valid state there into a
  permanent trip (low Tcold throughout cold shutdown). Gate such
  channels on a spec'd permissive derived from state, cite the text,
  test both sides of the threshold, and confirm callers feed coherent
  regime signals.
- An auto-start keyed to a sensed variable that no modeled dynamic can
  drive across its setpoint is unreachable code dressed as coverage (a
  standby-pump latch under a header pressure seeded constant forever).
  Before crediting the latch, produce the trajectory that crosses the
  setpoint through real dynamics; if none exists, the missing dynamics,
  not another test of the latch, are the gap.
- A protection the spec names without a setpoint is externally
  initiated: model the cause as an operator/malfunction-written failure
  latch feeding the shared actuation path, never an invented threshold.
  The trip reset clears the latch but not the failure input (hardware
  damage survives restart), and a restart witness must re-establish
  whatever the trip zeroed (admission valves) or Off->Running never
  fires - a stuck restart precondition there proves the cascade, not a
  bug.
- A block or override gates its own consumer only: sibling outputs
  latched from the same trigger stay armed (the operator SI block
  leaves steamline isolation live on the identical low-steamline
  signal). Asserting a blocked lineup means enumerating every output
  sharing the trigger, not assuming the block silences the family.
- **Gate delivery paths on the physical variable that throttles them,
  not on intent alone.** A pump latched Running with bus power still
  delivers nothing above its shutoff head; wiring flow to intent makes
  every spurious latch (a blocked-but-armed SI channel, a standby
  lineup) inject at full rate into regimes where physics forbids it.
  When a later feature first consumes the delivery, the missing gate
  surfaces as corruption in an unrelated green test — fix the gate in
  production, never by relaxing the must-still-pass witness.
- **Closing a coupling on local readiness alone bypasses the far
  side's interlock.** An auto-close keyed only to "ready" (a breaker at
  rated speed/voltage) slams onto a live peer out of tolerance; key the
  close on the far side being dead OR both sides matching within their
  compatibility band, and regression-test the held-open window (ready
  but unmatched ⇒ still open) — end-state checks see only the eventual
  close, and a ramping quantity self-heals a sag planted too early, so
  plant the violation immediately before the crossing tick.
- **Wiring a dormant field's first consumer re-prices every zero
  value.** A valve or flag that existed as state-only (operator
  control, alarm annotation) acquires physics the moment something
  reads it; construction defaults and hand-built fixtures that never
  set it now run at its zero value, which for a valve means isolated.
  Sweep constructors (MakePlant) and fixture builders for every field
  the new consumer touches, default them to the healthy-lineup state,
  and expect previously-green scenarios to branch where an inert latch
  had been silently accumulating trigger crossings all along. Scope
  the fixture sweep by grepping the type's composite literals repo-wide,
  not by walking the owning part's test file: scenario replay tests
  build their own local lineups (and will fail far from the change site
  with a deficit equal to the gated transfer they expected).

## 4. Test honesty

- **Find the tests that pass because of the bug**, and after fixing,
  re-derive each surviving assertion's causal chain: an old expectation
  can keep passing through an unrelated mechanism, which reads as
  coverage while testing nothing. Symmetrically, when a fix breaks a
  test, separate premise from intent — was the failing setup itself
  produced by the bug's side effect? Rebuild preconditions explicitly
  instead of restoring the artifact.
- Making a write path rate-limited changes which consumers can still
  win their race, not just the timing: a chase loop sized to outrun a
  faster opposing drift (iodine-pit growth against ppm/s chemistry)
  now fails as a horizon timeout. Recompute each consumer's required
  slew against the slowest opposing process before keeping its old
  window, and prefer driving stored state directly when the test's
  intent is elsewhere.
- A regression test must fail on the old code; if you cannot say what
  value it would have produced pre-fix, it is not a regression test
  yet.
- Strengthen before trusting: whole-structure equality plus closed-form
  expectations computed independently of the code under test beats
  spot-field checks.
- Prefer invariants physics forces over values calibration permits
  ("inventories equal across reactivity", not "rises exactly 0.4°F").
  Anchor transient-shape assertions to source magnitudes and timescales,
  not sign alone — a dropped coupling rescales a transient an order of
  magnitude while leaving direction intact. When model timescales are
  compressed against reference durations, express acceptance as state
  bands along the trajectory, never wall-clock figures quoted from the
  source table.
- Anchor return-to-state and round-trip assertions to the seed value
  captured from the fixture under test, not a shared initial constant:
  upstream slices legitimately redefine construction defaults (initial
  chemistry, rated operating points), and an expectation pinned to the
  old constant fails or, worse, keeps passing against a different
  evolution than the one authored.
- A convergence test whose fixture starts exactly at the asserted
  equilibrium is vacuous; perturb the initial condition so the
  trajectory is exercised. Corollary: an underdamped loop turns a
  fixed-horizon sample into a phase-dependent assertion - the same
  deterministic trajectory passes or fails by sampling luck. Scale the
  perturbation so the residual oscillation fits inside the asserted
  band at the sample time, or bound the envelope over a window instead
  of asserting one endpoint.
- Assert threshold crossings at the crossing — poll until the state
  flips, then check the boundary there. A fixed-horizon read samples
  the system after the transition has come and gone. Corollary: when a
  long window exists only to let a slow ramp reach a threshold (boron
  crawling toward critical at ppm/s), collapse the dead time by seeding
  near the threshold instead of cutting the post-crossing margin, and
  comment the crossing estimate.
- Derive tolerances from discretization error (k = dt/τ ⇒ relative
  error ≈ k/2), not round numbers. Keep horizon/parameter honesty: a
  scenario that only works in a narrow band under current calibration
  says so in its comment.
- **Count skips as untested, not passing.** A skip guard turns a broken
  fixture into a green line precisely because the defect suppressed the
  precondition. Confirm changed tests' skips executed their bodies, or
  use require instead.

## 5. Stored state vs derived values

- History-dependent quantities (inventories, integrator states) are
  irreducible state; instantaneous aggregates are derivable, and storing
  those invites inconsistency. Single-writer discipline for anything
  aggregate-like that stays stored.
- Initializers encode assumptions about the past ("running at rated for
  a long time"). Make them explicit helpers rather than hand-filled
  literals; zero value should mean something coherent, not
  "half-initialized." Audit protective inputs against construction
  defaults: a trip channel reading a field left at zero is permanently
  armed (0 psia steamline holds SI actuated on a fresh plant).
- A seed contradicting the regime its sibling fields encode turns idle
  into transient: an at-power initializer with its support path secured
  lets the seeded offtake drain into low-level trips within minutes,
  and every downstream alarm reads as a fresh defect. Catch: step the
  pristine plant unattended for an hour and count alarms — a coherent
  initializer raises none.
- Promoting a proven test fixture into a production initializer:
  transcribe it field-for-field and name every deviation in a comment.
  A fixture's zeroed fields are load-bearing choices (no nuclear heat
  during pressurization vs neutron-source idle), and a silently
  "improved" value changes which physics the evolution exercises; the
  catch is a line-by-line diff against the fixture, since tests pass
  either way.
- **Contradictory seeds resolve themselves on the first Step** (turbine
  State Off with stop valves open on a pressurized header self-starts),
  so a fixture sampled before stepping describes a plant that no longer
  exists mid-test. Seed coherent intent explicitly, or re-read after
  the first step.
- A completion or acceptance check written over end-state bounds alone
  is satisfied at t=0 whenever the start lineup already sits inside
  the bounds - which is exactly the case when the exercise's trigger
  is a transient whose "recovered" state resembles its pre-transient
  state. Either start the evolution after the transient or persist a
  marker it fired; the catcher is an invariant test that every
  scenario opens with its check unmet.
- A recorded or hand-built start state must serialize latched
  edge-detector inputs coherent with their source fields: a dump
  holding `LastSetpoint=0` beside `Setpoint=1` replays as "no change"
  and silently disarms the >=10 percent load-rejection edge it was
  saved from. After loading a saved state for an evolution whose
  trigger is an edge, confirm the edge still fires through the real
  consumer before trusting the run. The same applies after any
  upstream change to state semantics: a recorded field's meaning can
  shift scale or unit (xenon normalized -> physical concentration),
  so regenerate recorded states instead of replaying them across the
  change.
- **Fixtures inherited across scenarios carry superseded-calibration
  margins even when they decode cleanly.** A heel inventory or seeded
  level sized under one slice's constants can sit inside a phase that
  current constants boil dry, overfill, or trip before the cited
  evolution completes - the file is coherent, the physics moved.
  Before replaying an inherited fixture, recompute the inventories its
  planned phases depend on against today's production constants, and
  re-derive rather than copy when a margin no longer spans the stroke.
- Every latched boolean needs an enumerated path back to false some
  actor can reach; grep writers before believing "until manually
  reset."
- **An end-state bound phrased as a threshold below a zero-seeded
  field is met at t=0.** Fresh plants leave many sub-structs at their
  zero value, so "concentration <= target" reads complete before the
  evolution runs; bind completion to evidence the evolution uniquely
  produces (a scripted setpoint landed with its tracking latch
  released), and let a start-unmet harness catch the vacuous case
  before it ships.
- Write trigger conditions over enumerated states, not complements:
  `state != Running` fires on Off and every future mode, turning relief
  paths into idle-plant drains. Automatic transients get their own
  latched bool, never the operator's mode enum — overwriting Mode
  erases standing commands and misroutes logic.
- An operator command writing state an automatic controller rewrites
  each tick is a no-op with a green read-back. Grep the step function
  for later writers before trusting a manual path; wire a manual latch
  the controller respects.
- Surfacing an automatic transition as a manual control lets the
  operator reach the destination state before the automatic branch ever
  runs, silently skipping incidental latches that lived inside that
  branch (an early recirculation-suction write starves the sump-pH
  latch keyed inside the injection branch it bypasses). Enumerate the
  branch's other writers when exposing the control, and test both
  arrival orders — operator-first and automatic-first.
- The inverse also holds: automating a step the spec assigns to the
  operator deletes a trained evolution and usually fires it at the wrong
  time (an Lo-Lo auto-latch performed the switchover the spec reserved
  for the crew). Require explicit spec text for every automated action;
  when only the cue is spec'd, expose the cue and leave the action to
  the operator.
- After a refactor moves a read between stored field and constant,
  grep for survivors: a field with writers but no production readers is
  dead state wearing a live name. Classify before deleting — design
  parameters deserve readers back, TODO placeholders are plan anchors,
  production-written/test-read fields may be deliberate seams.

## 6. Comments and citations

- Any comment touched by a semantic change gets re-audited: stale field
  names, superseded formulas, wrong numbers, references to removed
  identifiers.
- Every magic constant carries a citation (file:line into specs, or a
  cached reference article) or an explicit "assumption; source is
  silent" marker. Disclose known approximation errors next to the
  constant. In multi-source repos label the source class (primary spec,
  secondary manual, derived) — a secondary number presented as primary
  silently upgrades its authority. Corroborate derived constants
  against independent sources and reconcile disagreements.
- A quoted line range must span the full quotation (its last line
  contains the quote's final word); when OCR breaks a sentence so no
  range spans it, paraphrase with the citation instead of quoting.
- Adjacent sibling pages are the classic wrong-citation trap: verify
  the path, not just the range. Re-read quoted evidence from disk
  before submitting — cited line numbers drift under your own edits in
  the same session, including file:line anchors inside plan documents;
  re-grep them immediately before finishing.
- A cited range that runs past end-of-file invalidates silently: the
  entry looks authoritative and nobody re-opens a page whose tail
  cannot contain the quote. Check `range_end <= file_lines` whenever a
  citation is written or repaired, and treat an out-of-range citation
  as a signal to re-locate the content, not to trim the number.

## 7. Verification workflow

- Trust the toolchain, not the IDE: LSP diagnostics go stale, and emit
  noise mid-save. Confirm with a real
  `go build ./... && go vet ./... && go test ./...`.
- Files change underneath you (parallel sessions share the tree):
  mtime newer than your last read, diff hunks you did not author, or a
  HEAD commit you did not create all mean re-read the region before
  editing, never assume. The same trap exists across sibling checkouts
  that share trees via symlink: an absolute path through the symlink
  target patches the other clone silently. After every edit, confirm
  the changed path shows up in the working repo's own git status.
- Before reshaping or removing state, grep all readers and writers;
  convert consumers first, remove last.
- Reproduce reported misbehavior through the real entrypoint on a saved
  scenario; sample trajectories at several timestamps, not just
  endpoints. When theory and measurement disagree, instrument and
  print — do not stack speculation on speculation.
- Rebuild scratch verification binaries immediately before each run, or
  prefer `go run`: a stale helper binary's failures look exactly like
  model bugs. Regenerate start-state artifacts by replaying the single
  complete edit pipeline against the pristine dump — partial re-application
  yields a plant that runs cleanly while missing whole subsystem
  lineups.
- Once the fix lands, re-run the reproduction using only legitimate
  operator actions; the debug hook used to expose the bug must not stay
  in the verification path. Check sensors against delivered behavior,
  not just behavior.
- Clean up scratch/debug files before finishing; leftover debug tests
  rot into false confidence.
- **When an initializer's seeded value changes, grep tests for the old
  value as a literal expectation.** "Unchanged" assertions pin a seed
  by value, so the failure surfaces far from the change site as a
  mysterious chemistry/level drift instead of a seed mismatch; derive
  such expectations from the initializer's named constant. The same
  sweep covers fixtures that hand-seed lineups the reseeded
  construction defaults no longer imply (pumps secured, breaker open,
  turbine stopped).
- **A single-pass command consumer stalls behind the first
  not-yet-due line.** Runners that walk a command file with one
  cursor silently park on the first entry whose timestamp exceeds
  elapsed time; every later-line command, however small its own
  offset, is delayed behind it and then force-applied at the end of
  the run. Sort command files by timestamp at parse time or verify
  ordering: the failure is silent, and the end-of-run dump shows the
  command as applied.
- **Trim command files to the verification window before probing.**
  Commands scheduled past a short run are force-applied after the
  loop, so observations from a truncated replay of a long file are
  contaminated by future state. Cut the file to offsets inside the
  window and compare like with like.
- **A recorded stabilizing controller does not inherit stability when
  replayed open-loop.** On any unstable equilibrium (xenon-coupled
  power plateaus), sampled or rounded command-file exports diverge
  exponentially even though the generating closed-loop run looked
  clean. Replay-verify through the real consumer and require
  bit-level endpoint agreement before trusting a generated command
  file; export every control update at full precision if fidelity is
  required.
- **A test harness that calls Plant.Step without Plant.Update exercises
  a plant that never advances.** Step writes nextPlant and Update
  copies it back into p, so assertions after bare Step calls verify the
  seed, not the dynamics: a "holds steady" pass on frozen state reads
  as stability evidence while nothing was simulated (a control write
  then looks inert because it never reached a stepped tick). Use the
  package's stepN helper or pair every Step with Update.
- **When parking an unstable equilibrium, close the loop on a
  directly-measured state variable, not a target computed from model
  identities.** Formula-driven demands (insertion from temperature
  program, boron from xenon balance) accumulate drift from every term
  the identity omits; a correction recomputed each window from the
  measured reactivity absorbs them all. Prefer the sensor that already
  exists over deriving the same quantity from constants.
- **Audit fixture generators for clock/step desync:** a loop applying
  controls every Nth tick while stepping the plant once per outer
  iteration simulates a fraction of the claimed horizon. Print
  achieved simulated seconds, never just iteration counts.
- **Unit-check command-file offsets against achieved spacing, not
  intent.** A seconds-intent literal in a millisecond-offset file
  ("2100" meaning 2100 s) fires 2.1 s into the run and silently ends
  the lineup it was meant to hold; the run stays green because a
  cleared flag is a legal state. After any long-hold run, confirm the
  end state still carries the commanded lineup (flag still set, valve
  still open) before crediting it.

## 8. Justification audit for new tests

Every new test traces to one of three sources, with assertion strength
matching the tier:

1. **Physics-forced** (sign/direction, conservation laws): assert hard
   — zero/non-zero, strict inequalities, monotonicity.
2. **Spec-stated** (quoted values, with file:line): assert hard on the
   quoted value.
3. **Invented modeling conventions** (lumped constants, caps, lags):
   assert only existence and direction; pinning an exact invented value
   turns a private choice into a fake fact.

Checks: name the justification in the comment (no justification = delete
or find one first). Expected values recomputed from the *same* constants
as production are wiring checks — mark them as such in the comment.

## 9. Completeness sweeps before declaring done

Mechanical sweeps:
- Enumerate call sites repo-wide whenever a signature or contract
  changes; the compiler finds missing arguments, only the sweep finds
  inconsistent ones.
- Tag references as declaration/write/read/comment before judging
  liveness — hit counts do not classify. After retiring an identifier,
  grep repo-wide for stragglers in comments and docs too. The same sweep
  applies when an index item CLOSES: every ledger row that says
  "blocked by item N" is silently stale from that moment — re-derive or
  re-point each dependent reference as part of the closing slice, not
  someday. The closure checklist is in SCEN_SKILL.md ("Gap ledger and
  closure"): `plans/x/gaps.md`, `plans/x/big_gaps.md`, the
  per-scenario ledger (`plans/x/scenario/<N>.txt`), and
  `plans/x/scenario/remaining_gaps.md` must all agree in the same
  commit.
- Verify edits landed: re-grep for the new symbol after applying.
  Editing tools report success on stale buffers.
- Sweep for siblings of every bug found by inspection: one
  hardcoded-zero input implies others; one drifted conversion literal
  implies a family. Grep the pattern class, not just the instance.
- Renaming a bridge part family is a replay-key migration: grep the
  whole prefix (keys, Part fields, markdown doc links, scenario tests,
  recorded sessions) rather than the leaf label, land every consumer in
  one slice, and say in the handoff that pre-rename exported command
  files need the same string swap.

Step-function write ordering:
- **Before adding a writer to a field, grep every existing writer -
  including assignments split across lines.** An inserted force-off
  above a split-line `X =
	Y` assignment leaves the tail as a second
  writer that runs later and silently reverses the new one on fields
  the old code owned; the hardened test fails on the FIXED tree until
  the duplicate is deleted.
- **A control-map binds to the plant instance passed to it.** Helpers
  that take the plant by value register operator writes against a
  throwaway copy - the Set returns nil, nothing reaches the live
  plant, and the test reads defaults while every local number looks
  self-consistent. Bind once against the stepped variable and assert
  a read-back through the same instance before crediting the write.
- A whole-struct assignment subsumes per-field writes made earlier in
  the same step; re-apply latches after it or move them into the
  stepped type.
- A next-step buffer assembled subtree-by-subtree runs on zero state
  wherever `next.X = next.X.Step(...)` lacks a preceding seed; grep
  each call site for its seed and test a non-zero lineup.
- An edge detector compares against a value latched at the end of the
  previous step, and the latch update sits after every reader of the
  previous value — mid-function refresh flattens edges this very step.

Coupling and isolation contracts:
- Adding a physical coupling changes the contract of every test that
  isolates either endpoint: fixtures premised on "only the valve
  differs" lose their cancellation premise when the valve starts moving
  shared state. Grep tests naming both endpoints before concluding the
  blast radius is local.
- Promoting a placeholder fluid node into a level-bearing one (no-op
  `level`/`setLevel` replaced with real storage) rewrites the behavior
  of every edge touching it at once: transfers that used to vanish into
  the no-op become real credits/debits, static differentials that read
  zero start moving mass both directions, and the simulator's
  low-level/room clamps engage or disengage with `capacity()`. Re-run
  each adjacent edge's flow by hand against the new semantics and give
  the node a coherent floor before trusting any previously-green test.
- Whole-plant end-to-end tests share the simulator with everything:
  neutralize edges you do not mean to exercise, prefer differential
  comparisons between identically-configured plants, orient the
  differential in the assertion (which side must lose more), and
  downgrade exact-value assertions to directional ones where residual
  coupling remains — naming what was neutralized and why. A directional
  comparison between near-equal quantities still passes on float noise:
  give oriented differentials a minimum margin derived from the driving
  magnitude, and when the expected effect is small, instrument the run
  and print the measured separation before fixing the band.
- Zeroing a derived field in fixture setup does not silence its process:
  per-step values are recomputed from drivers inside the next Step.
  Cut the driver (NRVs closed, pump stopped, valve shut), then sample
  after one step to confirm.

Change classification and conflicts:
- Classify every test broken by a semantic change before touching it:
  intentionally updated vs must-still-pass regression. Update only the
  first kind.
- Defects belonging to another slice get recorded as out-of-scope notes,
  never silently folded in.
- A fix whose correctness depends on executing after a statement another
  slice owns is a guaranteed future conflict: resolve by intent (trace
  each side's obligation), compose when both stand, else encode the
  order in ownership. When a plan's own items conflict, the acceptance
  criteria win over sketches, and the deviation is surfaced.
- Rebase/union resolutions verify at tree level: run the full gate on
  the conflicted working tree before continuing; treat precondition
  failures in previously-green tests as unresolved conflicts. During a
  rebase ours/theirs are named from the replay's perspective: --ours
  is the new base, --theirs is your own commit being replayed. Attribute
  any remaining failure to your resolution only after running it on the
  pristine base — a throwaway worktree of the base commit settles blame
  in one run. Keeping
  both sides of a conflict requires composing justifications and
  sweeping for orphaned twins — identical code bodies do not mean
  identical intent.

## Red flags (quick scan)

- "It compiles" offered as evidence for behavior.
- An assertion that survived a major refactor unchanged.
- A test whose failure message had to be rewritten to match new numbers
  rather than new physics.
- A clamp within a factor of 2 of the phenomenon of interest.
- A quantity sitting exactly at capacity or floor for the entire run.
- Round-trip tests that only ever compared two fields.
- Two call sites of the same step function fed from different states.
- An exact-value assertion on a constant whose comment says "derived".
- A citation you did not re-read from disk this session.
- A latched flag whose writers all assign true.
- A cross-system boolean written in one direction only.
- A trigger condition written as `!= <one state>`.
- An edge detected against a value captured inside the same step.
- A changed test whose latest run shows SKIP where it used to show PASS.
- A conflict resolution declared done on compile success alone.
- A name whose only remaining reference is its own definition.
- The same conversion factor spelled two ways in two files.
- An edit batch reported "applied" without a follow-up grep.
- An exact-value end-to-end assertion on a quantity the fluid graph can
  reach.
- A protection channel armed where its trigger condition is the healthy
  state.
- Two controlled variables driven by one derived scalar.
- An interlock whose trigger lies outside its channel's reachable range.
- A formerly failing expectation that now passes because the causal
  path changed.
- A corrective write trimmed at exactly the rate another writer
  refills: the actuator parks at a constant intermediate forever while
  the process variable downstream keeps drifting. Sample the actuator
  position against its commanded target over time, not only the
  process variable.
- An edit batch reported "applied" without a follow-up grep. (already
  listed above; see also §9 verify-edits-landed.)
- A recorded "cannot fix locally" justification accepted without
  re-measuring the actual violation it names: waivers encode hypotheses
  about cost, and measurement can retire them.
- An assertion missing by approximately the size of the effect it
  asserts: the gate or driver behind the mechanism never engaged, so
  the modeled transfer did not run at all — recheck the enabling flag,
  not the rate.

## 10. Feed findings back into this file

- Running the checklist is the input half of a loop; the output half is
  updating it. After working a task through this checklist, ask what
  that run taught which the checklist missed, and write it in before
  declaring done.
- Record only generic lessons: mechanism plus catching check, placed in
  the section they belong to, in the same terse style. If nothing new
  surfaced, say so and move on — no padding with restatements.
