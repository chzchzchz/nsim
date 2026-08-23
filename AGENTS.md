# Design

* Only implement what is important for simulation
* Prefer generalization over specialization
* Trace implementation details back to specifications in specs/ (referencing full file paths / line numbers)

# Specifications

##  Westinghouse PWR document

* Table of Contents: specs/the_westinghouse_pressurized_water_react/toc.md
* Glossary: specs/the_westinghouse_pressurized_water_react/glossary.md
* This is the source of truth.

# Tools

* Set "description" in tool json if tool is failing for no reason.
* Prefer gopls mcp's lsp tool commands for mechanical code changes.

# Tests

* Avoid writing slow tests (e.g., >1 second runtime)
* Use testify/require and testify/assert packages for tests instead of t.Fatal or t.Error