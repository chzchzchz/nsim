# Design

* Only implement what is important for simulation
* Prefer generalization over specialization
* Trace implementation details back to specifications in specs/ (referencing full file paths / line numbers)
* DO NOT commit files in specs/* or plans/*

# Specifications

Full path (shown below) references to specification files (with line numbers) should be cited when commenting to soundly justify plant/physics modeling decisions. Do not truncate the path with ellipses.

##  Westinghouse PWR document

* Table of Contents: specs/the_westinghouse_pressurized_water_react/toc.md
* Glossary: specs/the_westinghouse_pressurized_water_react/glossary.md
* This is the source of truth.

## Nuclear Physics

* Physics questions may be resolved by checking wikpedia.
* Access wikpedia articles via (non-agentic) ordinary web fetch tool of url https://en.wikipedia.org/wiki/<ARTICLE_NAME>
* Cache wikipedia articles in specs/wiki/<ARTICLE_NAME>.txt
* Article "Category:Nuclear_technology" is a good place to start.
* Note that the Westinghouse PWR document is the primary source of truth.

# Tools

* Set "description" in tool json if tool is failing for no reason.
* Prefer godoc mcp tool commands to analyze source code.
* godoc mcp must query local packages by relative path with working_dir set to this repo root, e.g. path "./core", working_dir "/home/chz/go/src/github.com/chzchzchz/nsim-alt6". Never pass the import path (github.com/chzchzchz/nsim/core) as path: the module is not published, so the tool tries a remote git fetch and fails with "could not read Username for 'https://github.com'". Local resolution still reports the correct import path because go.mod declares module github.com/chzchzchz/nsim.
* Prefer gopls mcp's lsp tool commands for mechanical code changes.
* cmd/nsimrun can simulate a Plant using plant json (see cmd/nsimrun/README.md)

# Writing Tests

* Avoid writing slow tests (e.g., >1 second runtime)
* Use testify/require and testify/assert packages for tests instead of t.Fatal or t.Error