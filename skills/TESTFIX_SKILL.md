# Skill: Fixing Failing Tests (nsim)

Companion to `plans/CHECK_SKILL.md` (the general verification
checklist). This captures the working method for triaging and fixing
test failures in this repo, plus the concrete failure shapes met in
practice and the fixes that held up.

## 0. Triage order

1. Run the whole gate before touching anything:
   `go build ./... && go vet ./... && go test ./...`. Individual
   reruns hide cascade failures — fixing one production behavior can
   break a different test whose fixture silently depended on the old
   decoupling.
2. Capture per-test durations (`go test ./core -v`) in the same pass;
   fix failures first, then the slow list.
3. Classify each failure BEFORE editing: production defect / stale
   test premise / fixture incoherence / another session's work in
   progress. The classification decides everything downstream.
4. Check provenance of failing files: `git log --oneline -- <file>`
   and mtimes. Failures in code committed minutes ago belong to an
   active slice — prefer recording over rework, and expect the tree
   to move under you while you work (re-read before every edit,
   reconcile with `git diff HEAD`). Commits whose message restores
   something "dropped by the rebase" flag that nearby formulas were
   reassembled from memory — recompute them against a known-state
   sample instead of trusting the restore.
5. Feature slices land broken more often than finished: a commit
   adding a feature plus its pinning tests may never have had those
   tests compile or run. Run the slice's own pinned tests first —
   they are the clearest statement of its intended contracts
   (dispatch_load_demand_test.go pinned Grid.LoadDemand-as-MW while
   the scenario callers assumed fractions) — then fix outward from
   the contract, not from the callers.

## 1. Root cause: decompose empirically

- When a stored value disagrees with your hand-computed model of it,
  write a throwaway probe test that recomputes every contribution
  using the production functions at the failing state and prints them
  next to the stored field. A mismatch means a hidden writer, a scale
  change, or a term added since you last read the file. Delete the
  probe afterwards.
- For collapses and runaways, sample the trajectory at 1 ms / 10 ms /
  100 ms / 1 s. The first divergent step names the culprit; endpoint
  samples hide it.
- When one logical control is registered in several builder
  functions, map writes are last-write-wins: fixing one registration
  leaves the others silently overriding it (the LoadDemand control
  lived in three builders with different conversion kinds). Grep
  every registration of the key and align them all.
- Re-read production code after any git activity in the repo: a
  parallel commit can change the units or semantics of a field under
  your feet mid-fix (xenon inventory went normalized-to-rated ->
  physical-concentration between two reads of the same function).

## 2. Failure shapes met here

- **Fixture off-equilibrium poisons everything downstream.** Slow
  state variables (iodine/xenon inventories, decay-heat pools) must be
  seeded at their equilibrium FOR THE SCENARIO POWER, not for rated.
  The production seeder (`SeedSteadyStatePower`) does this correctly —
  trust it rather than hand-pinning fields. An amplified coupling
  (xenon worth x inventory ratio ~10x) turns micro disequilibrium into
  the dominant trajectory: a 51-percent-power fixture pinned at
  rated-equilibrium xenon drifted +0.185 percent reactivity and ran
  fission to the rated cap inside a 450 s window.
- **A bigger slower channel masks the channel under test.** Before
  asserting a feedback SIGN through dynamics, estimate every path's
  gain over the assertion window. Xenon responds to a power
  perturbation ~30x more strongly than the Doppler/moderator
  coefficient the test meant to expose. When the interesting sign
  lives only in the weak channel, assert it on the pure function
  (`netTempCoef(600) < 0 < netTempCoef(900)`) — tier-1 physics, no
  dynamics needed.
- **Assertions recomputed from raw demand condemn compliant ramps.**
  Rate-limited regulators walk an internal target at the design pace
  (5 percent/min); mid-glide, plant state matches the walked target,
  not the instant demand. Compare against the controller's published
  program point (`ProgrammedTavg`), not a recomputation from the raw
  setpoint.
- **An unused parameter in a control function is a missing feedback
  term.** `applyAutoControl` accepted measured Tavg and never used it,
  so Tavg could never be driven onto the program line — exactly the
  behavior its own header quote promised ("[...] and the measured
  reactor coolant temperature",
  specs/the_westinghouse_pressurized_water_react/chapter_7/page_130.txt:68-76).
  Grep the body of every control callback for its arguments.
- **Scenario end-states need real margin.** "Insert rods to reduce
  power level" for subcritical hot standby means shutdown margin
  (deep insertion), not trim worth: two percent of rod worth cannot
  hold subcritical once cooldown swings Doppler positive. Check the
  commanded worth against the END-state physics, including the
  positive swing coming from the direction the plant is moving.
- **A control's conversion kind is its unit contract, and layers can
  each be self-consistent while contradicting at their junction.**
  The grid dispatch control registered Kind "normalized" (fraction of
  rated) while the wiring converted the same field as MW; scenario
  callers entering fractions got walks that froze at one tenth of a
  percent of intent, while the feature's own pinned test entering MW
  passed in isolation. Signature of the mismatch: a walk or
  integrator takes one increment and freezes near zero.
  Trick: pick the canonical unit where the field lives, register the
  bridge kind that documents it, convert at exactly one site — and
  engage operator permissives explicitly in fixtures, because a gated
  feature looks exactly like a rate bug when the gate is closed.
- **Tight bands plus slow drift equals guaranteed red.** A +/-0.05 F
  band over a 450 s horizon is falsified by second-order xenon drift
  alone. Either shorten the horizon to before the drift matters,
  convert to directional assertions ("outlet relaxes downward toward
  the program"), or pin the drifting variable at equilibrium — pick
  whichever preserves the test's actual intent, and say which was
  chosen in the comment.

## 3. How this simulator's structure produces those failures

Commentary tying the shapes above to concrete modeling choices, so
triage starts from the architecture instead of rediscovering it:

- **Everything steps every millisecond.** Test runtime is simulated
  seconds x 1000, so spec evolutions measured in hours cannot be
  stepped naively. Per-tick multiplicative updates
  (`FissionPower *= 1 + rho/100`) also turn any sustained percent-scale
  reactivity into an exponential power drift of about rho x 10 per
  second: a "tiny" 0.005 percent bias compounds to minus a quarter of
  rated power inside a 20 s assertion window.
- **Three separated timescales share one state vector.** Fission power
  responds per step, primary temperatures over ~90 s, xenon over hours.
  Mid-band operating points therefore carry slow instability channels
  (a xenon-driven positive power coefficient at part load) that are
  invisible in short windows and dominant in long ones: the same
  scenario passes at 45 s and runs away at 450 s. Pick the window from
  the channel under test, and seed slow inventories at their
  scenario-power equilibrium (`SeedSteadyStatePower`) — or they pick it
  for you.
- **Worth-site scale conversions amplify quietly.** The xenon worth
  reads physical inventory through the iodine-to-xenon ratio (~10x), so
  small power moves swing reactivity tenfold more than the raw
  inventory delta suggests. A hand-solved criticality that omits the
  converted term is wrong by tenths of a percent at reduced power.
- **Steps copy value structs, and controllers recompute derived
  fields.** A fixture write to a per-step quantity (MSR drain flow,
  condensing rate) is overwritten by the next Step no matter how idle
  the plant looks — silence the driver (valve, pump, NRV latch), never
  the field. Value-copy assembly also creates deliberate one-step lags
  across subsystems (CondensingRate is produced by plant wiring after
  the condenser consumes it): data-flow order is part of the model.
- **Constants are equilibrium anchors, not boundaries.** SG header
  pressure relaxes toward the shell-pressure anchor scaled by
  draw-versus-boil ratio, so dump/ADV couplings move the anchor rather
  than clip a transient — artificial-load scenarios sag headers exactly
  when their hold/modulation logic ignores the process variable.
  Absolute-pressure expectations must be argued from the anchors
  (shell 920 psia, design 1000 psia), not from a run that once passed.
- **Open-loop temperature lines differ from control program lines.**
  The RPV thermal model settles Tavg on its own power/heat-balance
  curve whose full-load point anchors the rod program; away from that
  anchor the lines separate (~8 F at half load). A regulator whose
  outer loop errors on power fraction alone parks Tavg off-program —
  the unused measured-Tavg parameter was the tell.
- **Bridge controls write demands, not states.**
  `ControlRods/Insertion` writes DemandInsertion and the drive slews;
  dump valves travel, breakers close, diesels crank. An observation
  sampled immediately after a Set sees stale state unless the window
  budgets the travel time.
- **MakePlant is a live full-power lineup, not a neutral canvas.** Stop
  valves stand open on a pressurized header, so the turbine self-starts
  on the first Step wherever an enum still claims Off; seed explicit
  intent for anything other than "at power", and do not trust mode
  fields read before the first step.
- **Coupling contracts live in plant.go wiring.** Subsystem Steps take
  caller-computed fractions (steam offtake, turbine load), so gaining
  one wiring edge changes the meaning of every unit fixture that
  isolates either endpoint. Grep fixtures mentioning both endpoints
  whenever the wiring gains an input.

## 4. Physics details that were hard to reason about

Specific plant-modeling details that caused confusion while fixing
tests, with the working trick for each:

- **The Doppler reference is Tavg-anchored but fed outlet
  temperature.** `CoreDopplerReferenceTempF` (571 F) is the design
  average temperature, yet the term receives OUTLET (621 F at rated),
  so rated operation carries a deliberate +50 F x coef offset that the
  rod/boron calibration absorbs.
  Trick: when hand-solving criticality, reproduce that offset using
  outlet; what matters is the deviation from rated outlet, never the
  deviation from 571 itself.
- **Rated operation rides the rated cap under positive reactivity on
  fresh fuel.** MakePlant's "100 percent FP" is held by the
  `FissionPower <= rated` clamp with rho around +0.05 percent, not by
  critical balance.
  Trick: do not expect rho near zero at full power in fresh-chemistry
  fixtures; check whether an assertion secretly depends on cap-pinning
  before calling the state steady.
- **Xenon lives in two scales.** Stored `Xenon135` is a physical
  concentration whose rated equilibrium sits near lambdaI/(lambdaX +
  burnup) ~= 0.1; the worth site converts through `xenonNormalized`
  (~10x).
  Trick: never hand-pin raw xenon values — seed via
  `SeedSteadyStatePower` and read via `xenonNormalized`; a literal 1.0
  meant rated equilibrium under the OLD scale and ten-times equilibrium
  under the current one.
- **Percent-scale reactivity compounds per millisecond.** The quasi-
  static update applies `(1 + rho/100)` every tick, so sustained rho
  folds or grows power with e-folding time 100/|rho| seconds: minus
  0.005 percent collapses power at five percent per second.
  Trick: convert any expected rho to its e-fold time before choosing an
  assertion window; if tau is much longer than the window only drift
  shows, much shorter and the run ends pinned at floor or cap.
- **Net temperature coefficient flips sign across usable chemistry.**
  c(1000 ppm) = +1 pcm/F (fresh fuel) versus c(500 ppm) = -0.5 (end of
  life): fresh-fuel part-load points are bistable (ride the cap or fold
  to the floor) because temperature feedback reinforces instead of
  opposing.
  Trick: when a part-load scenario is bistable, print
  `netTempCoef(boron)` first; maneuvering scenarios need diluted shim,
  and the dilution itself is worth +0.25 percent reactivity that the
  rod solve must absorb.
- **Two different "power" numbers exist.** `FissionPower` excludes
  decay heat (~6.5 percent of steady output plus RCP shaft loss);
  programs and SG heat input track `TotalThermalPower`.
  Trick: name which one an assertion means — at half load the
  fission-only reading legitimately sits ~3.3 percent below the load
  level.
- **Tavg barely moves even when nothing is steady.** Inlet blending
  buffers Tavg ((inlet+outlet)/2 with inlet tied to a reference blend),
  so probes watched Tavg sit at 589.5 while power collapsed.
  Trick: read OUTLET (and the outlet target) for thermal response;
  Tavg is the buffered display, not the sensitive indicator.
- **Riding a clamp for a whole window can be legitimate.** During the
  rejection transient the header sat at the 1000 psia design clamp for
  the full ride-through — the clamp doing its job while production
  exceeded draw.
  Trick: when a value pins at a bound over the assertion window, say so
  explicitly in the comment ("pinned at design pressure while dumps
  carry the transient") instead of leaving a reader to wonder whether a
  runaway was absorbed.

## 5. Choosing the fix (preference order)

1. **Fix the fixture premise**: seed equilibria, coherent equipment
   lineups, perturbation small enough to stay linear.
2. **Fix production** when a spec-cited behavior is genuinely absent
   (the measured-Tavg feedback term above), updating the owning
   session's unit tests to the corrected semantics in the same change.
3. **Convert exact fixed-horizon bands to directional assertions**
   when a residual slow channel remains — naming the residual channel
   in the comment so the next reader knows why the band is soft.
4. **Record as known deviation** (comment with citation, right where
   the stronger assertion would sit) when the missing behavior belongs
   to another slice's roadmap. Never widen a band past its physical
   meaning and never delete coverage silently.

A rewritten test must still have failed on the old code — state in the
comment what value the old code would have produced. Speedups follow
CHECK_SKILL section 4: seed near thresholds instead of ramping to
them, poll to crossings with caps, trim regression windows to what the
original bug report justifies.

## 6. Improving test times

The cost model first: runtime = simulated seconds x steps per second
x per-step cost. Simulated seconds are negotiable by seeding and
polling (above); steps per second became negotiable too once TimeStep
turned into an overridable variable (`overrideTimeStep`,
core/timestep_test.go), with a fidelity bill that must be argued per
channel (below). Per-step cost is fixed: `Plant.Update` is a mandatory
double-buffer flip (six struct copies) after every Step, so
harness-level batching buys nothing. Strategies, in preference order:

- **Rank before cutting.** Capture per-test durations
  (`go test ./core -v`, sort on the parenthesized time) and spend
  effort on the head of the list; the median test costs fractions of a
  second and trimming it wastes review attention.
- **Seed near thresholds instead of ramping to them.** A boration leg
  that crawled 600 -> ~1140 ppm at 0.8 ppm/s needed a 1000 s window
  purely to cross its own boundary; starting at 1100 ppm cut the same
  assertion from 137 s to 10 s while keeping the live CVCS path and a
  directional outcome.
- **Trim regression windows to what the original report justifies.**
  Bug reproductions whose writeup says the corruption appeared "within
  seconds" do not need five-minute horizons; 30 s catches the identical
  regression.
- **Poll to crossings with caps instead of stepping past them.**
  Replaces fixed windows with early exits where dynamics cooperate and
  keeps an honest upper bound where they do not (diesel coolant decay:
  8.6 s -> 0.4 s; diesel shutdown: 3.0 s -> 0.4 s).
- **Run walk-completion math on regulator scenarios.** A cascade whose
  program slews at the spec pace needs demand-step/rate seconds to walk
  plus one thermal tau to converge; anything beyond that in the fixture
  is idle stepping. Settle windows can be cut to exactly that sum.
- **Parallelize independent plant tests.** Every test builds its own
  `Plant`, so heavy tests are order-independent and `t.Parallel()`
  overlaps them across cores; the package otherwise runs strictly
  sequentially. Verify no shared package-level mutables first, then add
  it to the measured heavy hitters. Combined with the trims above this
  roughly halved wall time; the longest single test becomes the floor.
- **Justify what survives.** Tests still above ~5 s carry a one-line
  reason (physical time constant like CCWS's one-hour inlet tau, or
  sole end-to-end integration cover) so future passes re-derive the
  decision instead of re-litigating it.
- **Widen the tick only where every exercised channel is width-exact,
  and prove it per channel.** The reactor's quasi-static update
  compounds `1 + rho/100` once per STEP with no dt factor, so
  closed-loop response speed scales with tick count, not simulated
  time: widening the tick does not merely coarsen trajectories, it
  changes where they land (the rod auto-control descent parked at
  0.82 of rated under a uniform 10 ms tick against a < 0.65 band;
  100 ms barely left rated). Channel classes: linear-in-dt ramps,
  timer accumulations, and fuel burns are exact at any width;
  algebraic assignments (sws.go sets OutletTemp directly from the
  duty list each step) likewise; threshold crossings shift by at most
  one tick; lag terms `gap*dt/tau` carry O(dt/tau) error per step,
  stated numerically (at dt = 10 ms, tau = 10 s the discrete decay's
  effective tau shifts about 0.05 percent). The per-step fission
  compounding is the channel that resists: program-walk windows stay
  at DefaultTimeStep.
- **Split a test into phases when only some windows maneuver.**
  overrideTimeStep calls chain (each restores the previous width on
  cleanup), so passive windows (chemistry trim, plateau holds,
  wide-margin directional assertions) ride a coarse tick while walk
  windows stay at production width. Probe widths empirically: failure
  values diagnose sensitivity (power stuck at 0.99 means no response,
  0.82 partial), and the measured evidence belongs in the test's
  policy comment.
- **Re-express anything defined in steps in simulated time before
  widening.** Compile-time step counts (`const n = int(d / TimeStep)`)
  stopped compiling when TimeStep became a variable; sampling
  cadences like checkEvery = 200 silently change meaning (200 steps
  is 200 ms at the default tick, 2 s at 10 ms), so write
  `int(200 * time.Millisecond / TimeStep)`; checkpoints sampled at
  magic indices (`case 4999`) must become time-derived or they sample
  different moments at different widths.
- **Classify detection logic as snapshot-based or rate-based before
  claiming width safety.** A detector comparing one operator write
  against a fixed fraction (`prevSetpoint - Setpoint >=
  LoadRejectDetectFraction`) is width-independent; anything that
  accumulates a rate across steps is not. Read the production
  comparison before asserting either way.
- **Removing t.Parallel is the price of mutating global state, and it
  trades wall time for CPU time.** Converted tests leave the parallel
  pool, so wall-clock gains run smaller than CPU gains; report both.

Never: loosen tolerance bands to absorb a shortened window, replace a
crossing assertion with a skip, shrink a horizon below the
walk/crossing estimate recorded in its comment, or ship a widened-tick
justification whose numbers were not recomputed from source.

## 7. Gates and cleanup

- Final gate is always the real toolchain: `go build ./... && go vet
  ./... && go test ./...` — LSP diagnostics in this repo go stale at
  scale (dozens of phantom errors while the build is green) and must
  never be treated as evidence either way.
- Probe and scratch test files are deleted, not commented out; left
  behind they rot into false confidence.
- Any test still above ~5 s carries a one-line justification comment
  (physical time constant, single end-to-end integration cover), or it
  gets trimmed like the rest.

## 8. Justification comments: cite or compute

A justification comment earns its place only if a reviewer can check
it without rerunning anything:

- **Cite a spec or name the constant; never neither.** Physics and
  sizing claims anchor to specs paths with line ranges (the flywheel
  coastdown: specs/the_westinghouse_pressurized_water_react/
  chapter_1/page_020.txt:83-84). Model-behavior claims anchor to the
  named production symbol so the reader can grep its value
  (LoadRejectDetectFraction = 0.10, dieselAmbientRate, sws.go's direct
  OutletTemp assignment). "Negligible" without the arithmetic is not
  a justification.
- **Show the arithmetic behind every number asserted.** Effective
  values come from recomputation, not recollection: the RCP coastdown
  comment first claimed a 0.005 percent tau shift at the 10 ms tick;
  recomputing lambda = -ln(1 - dt/tau)/dt gave 0.05 percent. A 10x
  error survived one review pass and was caught only by redoing the
  math from the constants.
- **Re-verify every cited value against source before committing.**
  Grep the constant's definition, confirm the spec file and line
  range exist and say what the comment claims, and re-read the touched
  function after any intervening commit; the tree moves between
  sessions and quotes rot.
- **Record measured evidence alongside analysis.** When a widened tick
  fails, write the failing value and width into the policy comment;
  when it passes, list the channels that were checked, not merely that
  it passed.

## 9. Handoff discipline

Failures classified as another slice's work in progress get written
down (test comment or notes file) with three things: the failing
assertion, the diagnosed mechanism, and the citation for the intended
behavior. They are never silently skipped in the summary given to the
user — count skips and known-failures as open items, not passes.
