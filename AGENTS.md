# Design

* Only implement what is important for simulation
* Prefer generalization over specialization
* Trace plan/implementation details back to specifications (referencing full file paths / line numbers)

# Specifications

##  Westinghouse PWR document

* Table of Contents: specs/the_westinghouse_pressurized_water_react/toc.md
* Glossary: specs/the_westinghouse_pressurized_water_react/glossary.md

# Tools

* Set "description" in tool json if tool is failing for no reason.

# Tests

* Avoid writing slow tests (e.g., >1 second runtime)