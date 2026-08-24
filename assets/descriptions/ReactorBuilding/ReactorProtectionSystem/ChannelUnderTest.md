Indicates which protection channel is currently isolated for testing - the readout side of the [channel selector](ReactorBuilding/ReactorProtectionSystem/ChannelTest).

Treat any disagreement between this indication and the physical jumpers or test switches as an unreviewed safety question: coincidence logic is counting on the isolation being where this says it is. With 2/4 logic reverted to 2/3 on the tested function, every real transient while a channel is out gets one less vote of margin before actuation.

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118

Related controls: [ChannelTest](ReactorBuilding/ReactorProtectionSystem/ChannelTest)

Source: specs/the_westinghouse_pressurized_water_react/chapter_7/page_138.txt:114-118
