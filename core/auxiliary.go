package core

// AuxiliaryBuilding models the auxiliary building that surrounds the
// containment. It groups the redundant electrical and fluid auxiliary
// systems required for safe shutdown and normal plant operation.
//
// Spec justification:
//   - "auxiliary building around the containment"
//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_025.txt:75-77
//   - "houses redundant electrical and fluid auxiliary systems"
//     specs/the_westinghouse_pressurized_water_react/chapter_1/page_027.txt:14-22
type AuxiliaryBuilding struct {
	// ChemicalVolumeControlSystem models the CVCS — the primary
	// chemistry and volume-regulation loop. Physically in the
	// auxiliary building, connected to the RCS cold legs via
	// penetration.
	// "The Chemical and Volume Control System (CVCS) shown in
	// Figure 4.1-1 is designed to perform the following
	// functions in support of the Reactor Coolant System (RCS)."
	// specs/the_westinghouse_pressurized_water_react/chapter_4/page_073.txt:42-45
	ChemicalVolumeControlSystem CVCS

	// ResidualHeatRemovalSystem models the RHRS — post-shutdown
	// decay-heat removal. Pumps and heat exchangers are outside
	// the containment for maintenance access.
	// "The HHSI and RHR pumps [...] are located outside the
	// containment to provide for ease of access for
	// maintenance."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_097.txt:8-10
	ResidualHeatRemovalSystem RHRS

	// ComponentCoolingWaterSystem models the CCWS — the
	// intermediate cooling loop for safeguards equipment.
	// "The Safeguards subsystem is divided into two separate
	// and redundant safeguards trains and provides cooling
	// water to safeguards equipment such as the residual
	// heat removal heat exchangers."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_104.txt:33-39
	ComponentCoolingWaterSystem CCWS

	// ContainmentSpraySystem models the CSS — engineered
	// safeguard that limits peak containment pressure. The
	// spray headers are in the dome; the pumps and storage
	// tank are in the auxiliary building.
	// "The Containment Spray System (CSS) [...] is an
	// engineered safeguard designed to limit the peak
	// pressure in the reactor containment building."
	// specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:75-78
	ContainmentSpraySystem ContainmentSpraySystem

	// RCFCUnits are the four parallel fan units of the
	// Reactor Containment Fan Cooler System. While the units
	// themselves are inside the containment dome, their
	// controls and power supplies are in the auxiliary
	// building.
	RCFCUnits [RCFCUnitCount]RCFCUnit

	// EmergencyFeedwaterSystem is the secondary-side
	// emergency feedwater supply. The tanks and pumps are
	// in the auxiliary building near the containment.
	// "The primary function of the Emergency Feedwater
	// System (EFWS) [...] is to supply feedwater to the
	// steam generators following accident or transient
	// conditions."
	// specs/the_westinghouse_pressurized_water_react/chapter_5/page_099.txt:23-27
	EmergencyFeedwaterSystem EFWS

	// DieselGenerator models the train-A emergency diesel. Located
	// on site, physically and electrically independent from
	// offsite power.
	// "auxiliary power required for safe shutdown will be
	// supplied from physically and electrically independent
	// diesel generators located on the site."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:28-31
	DieselGenerator `json:"DieselGenerator"`

	// DieselGeneratorB models the train-B emergency diesel,
	// independent of the train-A unit and of the train-A ESF bus:
	// "Each power source, diesel generator and offsite, up to
	// the point of connection to the engineered safety features
	// system power buses, is physically and electrically
	// independent."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:30-36
	DieselGeneratorB DieselGenerator `json:"DieselGeneratorB"`

	// Transformer models the station auxiliary transformer
	// that steps down offsite grid voltage to plant
	// distribution voltage (normally 6600 V).
	// "This supply is transformed to the plant electrical
	// distribution bus voltage, normally 6600 volts, by
	// one or more system auxiliary transformers."
	// specs/the_westinghouse_pressurized_water_react/chapter_11/page_164.txt:45-49
	Transformer `json:"Transformer"`

	// SpentFuelPoolCoolingSystem models the SFP cooling.
	// The spent fuel pool is in a separate compartment on
	// the common base mat adjacent to the auxiliary
	// building.
	// "A separate compartment on the common base is for
	// storage of new and spent fuel."
	// specs/the_westinghouse_pressurized_water_react/chapter_1/page_027.txt:14-17
	SpentFuelPoolCoolingSystem SpentFuelPoolCoolingSystem
}

// Step advances each subsystem in place. Pure on the input receiver;
// writes only to the returned copy.
//
// Cross-struct inputs (e.g. CCWS OutletTemp feeding the RHRS
// heat-exchanger inlet) are wired by Plant.Step before and after this
// call. EFWSFeedwaterFlow (the per-step flow to the steam generators)
// stays on ReactorBuilding because the SG consumes it inside the
// containment. swSupplyTemp is the prior-step service water supply
// temperature (Outside.ServiceWaterSystem.InletTemp), the cold-side
// input to the component cooling heat exchangers.
// swFlowGPM is the prior-step Service Water flow rate
// from Outside.ServiceWaterSystem.Flow (in gpm), fed in explicitly
// by Plant.Step so this step does not have to reach into another
// building; zero flow also strips the CCWS of its ultimate heat
// sink. The diesel jacket cooler only sees cooling water when
// the SWS pumps have developed actual flow — PumpRunning alone is
// not enough if the ESF bus is de-energised.
//
// sfpcDriveReady is the upstream-power readiness signal for the
// SFPC pump motors (480 V MCC downstream of an ESF bus). It is
// plumbed in by Plant.Step so this step does not have to reach into
// the plant electrical model. See plans/x/uat_sat_gating.md.
func (a *AuxiliaryBuilding) Step(
	swSupplyTemp Temperature,
	swFlowGPM float64,
	sfpcDriveReady bool,
) AuxiliaryBuilding {
	ret := *a
	ret.ComponentCoolingWaterSystem =
		ret.ComponentCoolingWaterSystem.Step(swSupplyTemp, swFlowGPM > 0)
	ret.SpentFuelPoolCoolingSystem =
		ret.SpentFuelPoolCoolingSystem.Step(sfpcDriveReady)
	ret.DieselGenerator = ret.DieselGenerator.Step(swFlowGPM)
	ret.DieselGeneratorB = ret.DieselGeneratorB.Step(swFlowGPM)
	return ret
}

// anyDieselStartSignal reports whether either train's diesel carries
// the given start signal. The safety systems keyed on the diesel start
// signal (HHSI, SWS, EFWS, RCFC low-speed switchover) actuate per
// signal regardless of which train's diesel received it.
func (a *AuxiliaryBuilding) anyDieselStartSignal(sig DieselStartCause) bool {
	return a.DieselGenerator.StartSignal == sig ||
		a.DieselGeneratorB.StartSignal == sig
}
