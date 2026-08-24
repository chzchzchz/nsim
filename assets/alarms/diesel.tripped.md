The diesel generator tripped on protective relaying while running. Whatever operated - engine fault, generator electrical fault, loss of its own lube or fuel auxiliaries - the bus it was feeding is now dark unless another source caught it. The immediate question is not the diesel but what the bus lost: ESF pumps stopped mid-accident are an emergency inside the emergency.

Restarting into the same fault accomplishes nothing except proving the protection works. Find and clear the cause first; the start signal remains latched by design, so once conditions permit, the machine returns to [ready-to-load](AuxiliaryBuilding/DieselGenerator/ReadyToLoad) quickly - the 30-second clock restarts from the new start signal, not from the trip.

Check both ESF buses ([A](Electrical/ESFBusA/Energized), [B](Electrical/ESFBusB/Energized)) to confirm which loads transferred, shed, or stalled.

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-8

Related controls: [StartSignal](AuxiliaryBuilding/DieselGenerator/StartSignal), [BreakerClosed](AuxiliaryBuilding/DieselGenerator/Generator/BreakerClosed)
Related sensors: [ReadyToLoad](AuxiliaryBuilding/DieselGenerator/ReadyToLoad), [Energized](Electrical/ESFBusA/Energized), [Energized](Electrical/ESFBusB/Energized)

Source: specs/the_westinghouse_pressurized_water_react/chapter_11/page_166.txt:1-8
