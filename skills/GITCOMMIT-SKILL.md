# Skill: Committing Changes in This Repository

Use this skill when the user asks to create a git commit (e.g. "commit this").
Do NOT guess or skip the workflow; follow the steps exactly.

## Core principle

**Only commit what matters for simulation and development.**  
Documentation, planning, and specification directories are listed in
`.gitignore` and must never enter the project history.

## What to exclude from commits

| Pattern | Rationale |
|---|---|
| `plans/` | Design notes for future work. Listed in `.gitignore`. |
| `specs/` | Vendored source material. Listed in `.gitignore`. |
| Stale build artifacts | Emitted by toolchains (compilers, bundlers) into source trees. Only commit intentional source changes. |
| Anything not part of the requested change | If the user asked to fix one file, stage only that file. Do not add "while I was here" cleanups, adjacent files, or generated twins. |

Check `.gitignore` in the repository root for the full list of ignored
paths. If a path is ignored, do not force-add it.

## Commit workflow (must follow exactly)

### 1. Inspect the candidate changes

Show the user three things in a single compound command:

```sh
git status --short
git diff
git log --oneline -n 5
```

This reveals:
- Untracked files that might need staging.
- What is actually changing (not what the user thinks changed).
- The imperative-mood style the project expects.

### 2. Stage only relevant files

Add files that are part of the fix or feature. Do NOT add:
- Files matching `.gitignore` (especially `plans/` and `specs/`).
- Scratch or debug artifacts.
- Any file that was not part of the requested change.

**Only commit what you changed.** If the user asked to fix one function
in one file, stage exactly that file. Do not add adjacent tests,
documentation, generated artifacts, or "obvious" follow-ups unless the
user explicitly asked for them.

Use `git add <path>...` for explicit control. Avoid `git add -A` or
`git add .` unless the working tree is already clean except for the
intended change.

### 3. Analyze staged changes

Before drafting the message, list the staged changes and classify them:

- Feature / enhancement / bug fix / refactoring / test / docs
- What problem does this solve for the simulator operator or developer?
- Assess project impact and check for sensitive info (keys, credentials, personal data).

### 4. Write the commit message

Follow these rules strictly:

- **First line:** Under 72 characters. Imperative mood. Clear verb.
  Good: `Fix reactor trip latch reset on power restoration`  
  Good: `Drop duplicate grid load-demand control registrations`  
  Good: `Model natural circulation as thermosiphon hydraulics`
- **Body (optional):** Explain *why* the change exists and what outcome it enables.
  Do not list filenames or implementation details.
- **No internal identifiers:** Do not reference plan IDs, ticket numbers, or
  internal jargon that a new contributor would not understand.
- **Attribution:** The body must end with a blank line, then:
  ```
  💘 Generated with Crush

  Assisted-by: Crush:stepfun-ai/step-3.7-flash
  ```

### 5. Create the commit

Use a heredoc with `git commit -m`:

```sh
git commit -m "$(cat << 'EOF'
First line under 72 chars.

Optional body paragraphs wrapped at 72 chars.

💘 Generated with Crush

Assisted-by: Crush:stepfun-ai/step-3.7-flash
EOF
)"
```

**Do not use `git commit -i` or `git commit --interactive`.**  
**Do not push.**  
**Do not amend unless a pre-commit hook forced a modification.**

### 6. Handle hook failures

- If the pre-commit hook fails, retry exactly once.
- If it fails again, stop and report that the hook is blocking the commit.
- If the hook modifies files, the commit must be amended: run `git add -A` on
  any modified tracked files, then `git commit --amend -m "..."` with the
  same attribution block.

### 7. Verify

```sh
git status
```

Return empty response to the user once verified.

## Checklist before committing

- [ ] No paths under `plans/` or `specs/` are staged (they are gitignored).
- [ ] No stale build artifacts are staged.
- [ ] First line is under 72 characters and uses imperative mood.
- [ ] Message is understandable without reading the diff.
- [ ] Attribution block is present and exact.
- [ ] `git status` is clean after the commit (or only shows expected untracked files).

## Common mistakes to avoid

- Committing because "the user said so" without inspecting the diff.
- Staging everything because it is easier than picking files.
- Using the commit message as a changelog ("Added X, fixed Y, updated Z").
- Forgetting the attribution block.
- Pushing to a remote repository.
- Including plan IDs or internal references that a newcomer cannot look up.

## Repo-specific conventions observed

| Convention | Example |
|---|---|
| First-line verb | `Model`, `Drop`, `Present`, `Fix` |
| Style | No trailing period on the first line if it is a fragment. |
| Body | Explains the physics or engineering rationale, not the diff. |
| Scope | Changes are small and surgical; one commit, one idea. |