Feedwater isolation has actuated: normal feedwater valves closed to protect the steam generators or turbine from a feed fault - excessive level pushing water toward the lines, a broken feed line, or protection logic calling for dry-out prevention.

With normal feed isolated, decay heat removal shifts immediately to [emergency feedwater](rps.afw_start); verify that handoff happened before anything else, because minutes matter once the isolation is real. Restoring normal feed prematurely can defeat whatever the isolation was protecting - re-open only against procedure logic, not optimism.

Watch [SG levels](ReactorBuilding/SteamGenerator/Water) throughout: they tell you whether isolation plus EFW is holding the heat sink steady or whether level is still moving the wrong way.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:12-16

Related controls: [MainFeedwaterControlValve](TurbineBuilding/MainFeedwaterControlValve/Open), [MotorPumpRunning](AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/MotorPumpRunning)
Related sensors: [Water](ReactorBuilding/SteamGenerator/Water), [MainFeedwaterPump](TurbineBuilding/MainFeedwaterPump/Running)
