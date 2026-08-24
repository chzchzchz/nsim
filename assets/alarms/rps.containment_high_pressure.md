Containment pressure high - something released mass and energy inside the sealed building, and passive condensation on interior structures is not keeping up. Candidate sources: steam from a broken secondary line, primary discharge through a stuck-open relief once the [PRT rupture disc](ReactorBuilding/PRTRuptureDiscBurst) fails, or a primary leak. Identifying which one drives everything else.

The response ladder follows containment design: stop the source where possible - [steamline isolation](rps.steamline_isolation) for breaks, securing discharge paths for relief flows - then actuate [containment spray](rps.containment_spray) when pressure threatens design targets. Spray condenses steam directly and scrubs airborne iodine while doing it, addressing two consequences at once.

Watch pressure trend against [temperature](ReactorBuilding/ContainmentBuilding/Temperature): pressure responding to actions confirms the source is being managed; pressure ignoring them means the release continues somewhere unfound.

Model note: declared but not yet wired to a process condition; containment pressure sensor and spray status remain the live indicators.

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:56-57
Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:4-9

Related controls: [PORV](ReactorBuilding/PORV/Open)
Related sensors: [ContainmentPressure](ReactorBuilding/ContainmentPressure), [Temperature](ReactorBuilding/ContainmentBuilding/Temperature), [SprayHeaderOpen](AuxiliaryBuilding/ContainmentSpraySystem/SprayHeaderOpen), [PumpRunning](AuxiliaryBuilding/ContainmentSpraySystem/PumpRunning)

Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:56-57
Source: specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:4-9
