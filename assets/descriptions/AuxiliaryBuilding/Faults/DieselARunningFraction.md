Simulator construct with no plant control: the diesel's load fraction from stopped (0) to rated conditions (1), filed with the faults and used as a derating premise, not a command. The plant multiplies fuel burn and generator real power by it (core/diesel.go), so setting it below 1 derates the machine: it starts, but it neither burns fuel nor generates at rated output. That is how the model reproduces a degraded or partially available diesel without inventing hardware for it.

No spec passage authorizes an externally settable derating lever. The 30-second design window and load-sequencing text govern full-start timing and breaker step order, not fractional output; they are context, not justification. Setting this below 1 in an exercise is a fault insertion, kept out of the operator surface by design.

Related controls: [StartSignal](AuxiliaryBuilding/DieselGenerator/StartSignal)
Related sensors: [ReadyToLoad](AuxiliaryBuilding/DieselGenerator/ReadyToLoad)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:133-140
