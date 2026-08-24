Selects which protection channel is isolated for calibration or test: **None**, or one of the monitored channels. While a channel is under test its trip signals are blocked from the coincidence matrix.

"The protection system normally uses 2/4 logic but reverts to a safe 2/3 logic when part of the system is being tested" - that reversion is why only one channel may be pulled at a time, and why the crew confirms [ChannelUnderTest](ReactorBuilding/ReactorProtectionSystem/ChannelUnderTest) matches the selector before believing any trip arithmetic during surveillance. Testing two channels simultaneously takes you below the minimum coincidence for some functions; the plant cannot protect itself that way.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118

Related controls: [ManualSI](ReactorBuilding/ReactorProtectionSystem/ManualSI)
Related sensors: [ChannelUnderTest](ReactorBuilding/ReactorProtectionSystem/ChannelUnderTest)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118
