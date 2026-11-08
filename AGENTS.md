# Design

* Only implement what is important for simulation
* Prefer generalization over specialization
* Trace implementation details back to specifications in specs/ (referencing full file paths / line numbers)
* DO NOT commit files in specs/* or plans/*

# Specifications

These are sources of truth and where to look for answers when asking questions. Full path (shown below) references to specification files (with line numbers) should be cited when commenting to soundly justify plant/physics modeling decisions. Do not truncate the path with ellipses.

##  Westinghouse PWR document

* Table of Contents: specs/the_westinghouse_pressurized_water_react/toc.md
* Glossary: specs/the_westinghouse_pressurized_water_react/glossary.md
* This is the source of truth.

## Nuclear Physics

* Physics questions may be resolved by checking wikpedia.
* Alwayus cache wikipedia articles in specs/wiki/<ARTICLE_NAME>.txt
* Access wikpedia articles via (non-agentic) ordinary web fetch tool of url https://en.wikipedia.org/wiki/<ARTICLE_NAME>
* Article "Category:Nuclear_technology" is a good place to start.
* Cache downloaded PDFs in specs/pdf/<DOCUMENT_NAME>.pdf; prefer Wikipedia citation link URLs when they resolve to a PDF (e.g., NUREG reports, primary literature), otherwise fetch via DuckDuckGo HTML search (https://html.duckduckgo.com/html/) and download the result URL that is a direct PDF.

# Tools

* Set "description" in tool json if tool is failing for no reason.
* Prefer godoc mcp tool commands to analyze source code.
* godoc mcp must query local packages by relative path with working_dir set to this repo root, e.g. path "./core", working_dir "/home/chz/go/src/github.com/chzchzchz/nsim-alt6". Never pass the import path (github.com/chzchzchz/nsim/core) as path: the module is not published, so the tool tries a remote git fetch and fails with "could not read Username for 'https://github.com'". Local resolution still reports the correct import path because go.mod declares module github.com/chzchzchz/nsim.
* Prefer gopls mcp's lsp tool commands for mechanical code changes.
* cmd/nsimrun can simulate a Plant using plant json (see cmd/nsimrun/README.md)
* **Use the builtin `grep` tool** to search file contents. Do not use bash `grep`. For large codebases, `rg` (ripgrep) via bash is also acceptable. Use `glob` to find files by name/pattern and `ls` to list directories. Use `view` to read file contents.
* **Use `view` to read files.** There is no `read` tool. Key parameters: `file_path` (required), `offset` (line number to start from, 0-based), `limit` (max lines to return). Default `limit` is 200 lines; omit `limit` to read up to 200 lines. Use `offset` and `limit` to avoid loading large files, but reading the whole file is acceptable for small files where the full context is needed. See `skills/TOOLS.md` for model-specific notes (applies to step-3.7-flash and similar).

# Writing Tests

* Avoid writing slow tests (e.g., >1 second runtime)
* Use testify/require and testify/assert packages for tests instead of t.Fatal or t.Error

# Running and Testing

* Do not write binaries / testfiles to /tmp/<binary>; write them to /tmp/nsim-<branch-name>/<binary>