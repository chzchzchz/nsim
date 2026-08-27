Latches when ESF bus A is energized. The bus is fed from one of two Safety Class 1E emergency diesel generators after loss of offsite power, so DriveReady remains high as long as at least one diesel is online.

High drive-ready does not mean the pump is running—it means the operator's [Running](Outside/MotorAFWPump/Running) command will succeed. If both diesels are down or the ESF bus has been shed, DriveReady drops and the motor-driven AFW pump will not respond.

Source: specs/the_westinghouse_pressurized_water_react/chapter_5/page_098.txt:152-156

Related controls: [Running](Outside/MotorAFWPump/Running)