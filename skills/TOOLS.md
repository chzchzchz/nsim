# Skill: Tool Usage — File Reading, Searching, and Editing

Applies to: step-3.7-flash and similar models that may guess non-existent tool names.

## Use `view`, not `read`

There is **no `read` tool**. To inspect file contents, use **`view`**.

`view` parameters:

- `file_path` (required): absolute path to the file.
- `offset` (optional): 0-based line number to start from.
- `limit` (optional): maximum lines to return. Default is 200.

## Use `grep`, not bash `grep`

There is **no reliable bash `grep` path**. To search file contents, use one of:

- **Builtin `grep` tool**: preferred for simple regex or literal searches. Respects `.gitignore`, returns paths sorted by mtime with line numbers, and supports `literal_text` and `include` filters.
- **`rg` (ripgrep) via bash**: faster for large codebases and supports richer flags. Use it when builtin `grep` feels slow or you need ripgrep-specific options. The project prefers ripgrep for heavy searches.

Do **not** use `grep` via bash — either the builtin tool or `rg` is correct.

## Use `glob` for filename searches, `ls` for directory listings

- **`glob`**: find files by name/pattern (glob syntax).
- **`ls`**: list a directory as a tree.
- **`grep`**: search file *contents* by regex or literal text.
- **`view`**: read file *contents*.

## When to read the whole file

- **Small files** (roughly under 200 lines): reading the whole file with `view` is fine and often simpler than guessing offsets.
- **Large files**: use `offset` + `limit` to load only the needed section.
- If you omit `limit`, `view` returns up to 200 lines. That is already a sensible partial read for most source files.

## Use `edit` for surgical changes

To modify file contents, use the **`edit`** tool (or `multiedit` for several changes in one file).

Required parameters:

- `file_path` (required): absolute path to the file.
- `old_string` (required): the exact text to replace.
- `new_string` (required): the replacement text.
- `replace_all` (optional): true to replace every occurrence.

Exact-match rules:

- Include **3–5 lines of context** before and after the target so the match is unique.
- Preserve **all whitespace, indentation, blank lines, and trailing commas** exactly as they appear in the file.
- Do **not** trim or re-wrap code; copy it byte-for-byte from `view` output.
- If `old_string` is not found, re-read the file with `view` and copy more context — never retry with guessed or abbreviated text.

Common mistakes:

- Omitting `file_path` in the JSON parameters.
- Matching a block that ends with a different comma/blank-line pattern than the file actually contains.
- Using `read` instead of `view` to inspect the file before editing.
- Guessing whitespace (tabs vs spaces, indent count) or line endings.

LSP shortcuts:

- For whole-function/method/type replacements, prefer `lsp_replace_symbol` — it resolves symbol boundaries automatically and avoids whitespace-matching failures.
- For renames, prefer `lsp_rename`.

## Tool-call format rules

This is the exact format these tool calls must use:

- Use `<function=toolname>` with an equals sign, e.g.
  `<function=view>`, not `<function:view>`.
- Do **not** place tool calls inside fenced code blocks or markdown
  backticks; send them as raw XML blocks at message scope.
- Required parameters must be present on every call: `view` needs
  `file_path`; `bash` needs `command` and `description`.
- `multiedit` needs `file_path` plus an `edits` array of objects
  with `old_string` and `new_string`.
- If the runtime reports `missing required parameter`, the call was
  either malformed or wrapped in markdown — re-issue it as a raw
  XML block with all required args.

## Troubleshooting failed tool calls

If a tool call fails, check these common causes before retrying:

- **Wrapped in markdown**: The call must be a raw XML block, not
  inside triple-backtick code fences. Code-fenced calls are ignored
  by the runtime.
- **Wrong function syntax**: Use `<function=toolname>` with an
  equals sign, not `<function:toolname>`.
- **Missing required parameter**: Every call needs the parameters
  listed in the tool's description. `view` requires `file_path`;
  `bash` requires `command` and `description`; `edit`/`multiedit`
  require `file_path` plus `old_string`/`new_string`.
- **`old_string` not found in `edit`/`multiedit`**: The text must
  match the file byte-for-byte, including tabs/spaces and trailing
  commas. Re-read the target region with `view` and copy the exact
  lines from the rendered output. Never guess.
- **`view` returned nothing**: The file may not exist, or the path
  may be wrong. Use `ls` or `glob` to confirm the path first.
- **`grep` returned nothing useful**: Try `rg` via `bash` for
  faster/regex-rich searches, or narrow the `path`/`include`
  filters.
- **Edit succeeded but file is wrong**: Re-read the file with
  `view` to confirm the change landed. If it did not, the call
  likely never fired (markdown wrap) or matched the wrong block
  (whitespace mismatch).

## Why this matters

Step-family models in particular have a tendency to call a generic `read` tool by name or to shell out `grep` via bash. When that happens, the call fails and wastes a turn. Using the correct builtin tool with the correct parameter names avoids that failure mode.