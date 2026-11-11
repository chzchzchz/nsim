Simulator construct with no plant control: train B mirror of [DieselARunningFraction](AuxiliaryBuilding/Faults/DieselARunningFraction), filed with the faults and used as a derating premise, not a command. The plant multiplies fuel burn and generator real power by it (core/diesel.go), so setting it below 1 derates the machine: it starts, but it neither burns fuel nor generates at rated output. That is how the model reproduces a degraded or partially available diesel without inventing hardware for it.

No spec passage authorizes an externally settable derating lever. The control-board placement passage cited below governs the governor, voltage regulator, breaker, and synchronization placement on the control board, not fractional output, and the load-sequencing passage governing startup loads governs step order, not output magnitude; they are context, not justification. Setting this below 1 in an exercise is a fault insertion, kept out of the operator surface by design.

Related controls: [StartSignal](AuxiliaryBuilding/DieselGeneratorB/StartSignal)
Related sensors: [ReadyToLoad](AuxiliaryBuilding/DieselGeneratorB/ReadyToLoad)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:2-4
