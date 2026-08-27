Latches when the diesel generator bus is within start voltage or is already supplying power. High drive-ready does not mean the pump is running—it means the operator's [Running](Outside/DieselAFWPump/Running) command will succeed and the pump will turn over.

The diesel generator is started by the safety injection signal or total loss of offsite power; until the engine is up to speed and closing its breaker, the pump motor sees a dead bus and DriveReady stays low.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:18-20

Related controls: [Running](Outside/DieselAFWPump/Running)