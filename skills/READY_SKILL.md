# Skill: Readiness gate before handing off

Loop this until the stop condition holds, then stop. Run it after
implementing a plan or scenario (plans/PLAN_SKILL.md,
plans/SCEN_SKILL.md).

1. Establish the finish line first. Re-read the plan or scenario
   document being executed (for scenarios: the Definition of done in
   plans/SCEN_SKILL.md plus that scenario's expected-outcomes table).
   Every expectation not yet implemented or not yet verified against
   its band is remaining work - write the list down before touching
   anything.

2. Check the tree before trusting it. Run git status: an in-progress
   rebase or merge resolves first (CHECK_SKILL section 7, including
   running the full gate before --continue); files with mtimes newer
   than your last read get re-read; commits at HEAD you did not create
   get reviewed before building on them. Parallel sessions share this
   workspace.

3. Continue implementing until every item from step 1 is done. No
   partial states, no TODO handoffs, no "you'll also need".

4. Run the real gate and trust it over the editor's diagnostics:
   go build ./... && go vet ./... && go test ./...
   LSP output emitted mid-save or mid-conflict is noise.

5. Run plans/CHECK_SKILL.md against the diff of the task's files. Fix
   what belongs to your change. Unrelated pre-existing failures are
   listed in the handoff as out-of-scope with their owning slice, not
   silently folded in and not silently skipped.

6. Commit only the files the task touched. This workspace keeps
   plans/ and specs/ as untracked symlinks into a sibling repository -
   never stage them broadly. If the patch turned out already upstream
   or there is nothing to commit, report that instead of committing
   empty. Use the repo's commit message format.

7. Report: what shipped, what the gate said, which known deviations
   remain open and why they are out of scope.

Stop only when step 4 is green, step 5 has no unfixed findings within
the task's surface, and the result is committed or explicitly
nothing-to-commit.
