Steam generator level below limits - the condition whose low-low variant auto-starts emergency feedwater. Level is the primary-to-secondary heat transfer medium - water carries core heat out as boiling steam - so falling level progressively strips the primary of its heat sink. Tavg and pressurizer pressure climb in response until the reactor trips itself if nothing intervenes first.

Work the feed path: [control valve position](TurbineBuilding/MainFeedwaterControlValve/Open), [regulator demand](TurbineBuilding/FeedwaterRegulator/ValveDemand), pump status. If normal feedwater cannot recover level, [emergency feedwater](rps.afw_start) starts automatically and must be verified delivering - minimum required flow to two effective generators within one minute is the design credit, continuing indefinitely without operator action for the first 30 minutes.

Remember swell and shrink: apparent level swings with pressure transients faster than real inventory changes, so read level trends against steam pressure before concluding mass is truly leaving.

Model note: this annunciator is declared but not yet wired to a process condition in the simulator - the emergency feedwater auto-start it describes is real, so watch SG level directly.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:1-9
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:33-36

Related controls: [MainFeedwaterControlValve](TurbineBuilding/MainFeedwaterControlValve/Open), [ValveDemand](TurbineBuilding/FeedwaterRegulator/ValveDemand), [MotorPumpRunning](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning)
Related sensors: [Water](ReactorBuilding/SteamGenerator/Water), [Pressure](ReactorBuilding/SteamGenerator/Pressure), [DemandedPowerSetpoint](PlantOperation/DemandedPowerSetpoint)

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:1-9
Source: specs/the_westinghouse_pressurized_water_react/chapter_16/page_226.txt:33-36
