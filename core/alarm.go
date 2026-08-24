package core

import (
	"fmt"
	"time"
)

// Severity classifies how an alarm affects plant behaviour.
type Severity uint8

const (
	// SeverityInfo is informative only — no automatic action.
	// "Monitors the plant for abnormal conditions and alerts
	// the operator to take appropriate action."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt:55-72
	SeverityInfo Severity = iota
	// SeverityWarning requires operator attention — no automatic action.
	SeverityWarning
	// SeverityTrip latches a protective trip until reset.
	SeverityTrip
)

// Alarm is a single alarm condition visible to the operator.
//
// The struct is the wire shape used by both Plant.Alarms()
// (read-only aggregation) and PlantControlRoom.AlarmSet
// (acknowledged-and-stale state). All fields are populated by
// the aggregator and consumed by the HMI; the simulation loop
// never modifies an Alarm value directly.
type Alarm struct {
	ID           AlarmID
	Description  string
	Severity     Severity
	Active       bool
	Acknowledged bool
	Timestamp    time.Duration
}

// AlarmID is a stable string key for each alarm condition.
// Values use reverse-DNS form: <subsystem>.<condition>.
type AlarmID string

const (
	AlarmIDOPCAlarm              AlarmID = "turbine.opc_alarm"
	AlarmIDOPCTrip               AlarmID = "turbine.opc_trip"
	AlarmIDReactorTrip           AlarmID = "rps.reactor_trip"
	AlarmIDSCCWLowFlow           AlarmID = "generator.sccw_low_flow"
	AlarmIDSCCWNoFlow            AlarmID = "generator.sccw_no_flow"
	AlarmIDDieselCoolant         AlarmID = "diesel.coolant_high_temp"
	AlarmIDDieselTripped         AlarmID = "diesel.tripped"
	AlarmIDDieselLowFuel         AlarmID = "diesel.low_fuel"
	AlarmIDContainmentHighPress  AlarmID = "rps.containment_high_pressure"
	AlarmIDSGLowLevel            AlarmID = "rps.sg_low_level"
	AlarmIDSGHighLevel           AlarmID = "rps.sg_high_level"
	AlarmIDLowSteamlinePressure  AlarmID = "rps.low_steamline_pressure"
	AlarmIDLowTcold              AlarmID = "rps.low_tcold"
	AlarmIDTurbineTripVacuum     AlarmID = "turbine.trip_vacuum"
	AlarmIDTurbineTripBearingOil AlarmID = "turbine.trip_bearing_oil"
	AlarmIDTurbineStepback       AlarmID = "turbine.stepback_active"
	AlarmIDGeneratorMVALimit     AlarmID = "generator.mva_limit"
	AlarmIDSIActuated            AlarmID = "rps.si_actuated"
	AlarmIDSteamlineIsolation    AlarmID = "rps.steamline_isolation"
	AlarmIDContainmentSpray      AlarmID = "rps.containment_spray"
	AlarmIDContainmentIsolA      AlarmID = "rps.containment_isolation_phase_a"
	AlarmIDContainmentIsolB      AlarmID = "rps.containment_isolation_phase_b"
	AlarmIDAFWStart              AlarmID = "rps.afw_start"
	AlarmIDFeedwaterIsolation    AlarmID = "rps.feedwater_isolation"
)

// String returns the human-readable one-line summary used by
// the HMI to render the alarm list.
func (a Alarm) String() string {
	sev := "INFO"
	switch a.Severity {
	case SeverityWarning:
		sev = "WARN"
	case SeverityTrip:
		sev = "TRIP"
	}
	return fmt.Sprintf("[%s] %s: %s", sev, a.ID, a.Description)
}

// alarmDescription is a small lookup table that keeps the
// alarm-list string formatting localised to this file. Anything
// not listed here falls back to the AlarmID literal.
func alarmDescription(id AlarmID) string {
	switch id {
	case AlarmIDOPCAlarm:
		return "Turbine overspeed permissive control alarm"
	case AlarmIDOPCTrip:
		return "Turbine overspeed permissive control trip"
	case AlarmIDReactorTrip:
		return "Reactor protection system trip"
	case AlarmIDSCCWLowFlow:
		return "Stator coil cooling water low flow"
	case AlarmIDSCCWNoFlow:
		return "Stator coil cooling water flow lost"
	case AlarmIDDieselCoolant:
		return "Diesel generator coolant over-temperature"
	case AlarmIDDieselTripped:
		return "Diesel generator protective trip"
	case AlarmIDDieselLowFuel:
		return "Diesel generator fuel oil low"
	case AlarmIDContainmentHighPress:
		return "Containment pressure high"
	case AlarmIDSGLowLevel:
		return "Steam generator level low"
	case AlarmIDSGHighLevel:
		return "Steam generator level high"
	case AlarmIDLowSteamlinePressure:
		return "Steamline pressure low"
	case AlarmIDLowTcold:
		return "Reactor coolant cold-leg temperature low"
	case AlarmIDTurbineTripVacuum:
		return "Turbine trip: condenser vacuum low"
	case AlarmIDTurbineTripBearingOil:
		return "Turbine trip: bearing oil pressure low"
	case AlarmIDTurbineStepback:
		return "Turbine-trip reactor stepback active"
	case AlarmIDGeneratorMVALimit:
		return "Generator MVA limit exceeded"
	case AlarmIDSIActuated:
		return "Safety injection actuated"
	case AlarmIDSteamlineIsolation:
		return "Steamline isolation actuated"
	case AlarmIDContainmentSpray:
		return "Containment spray actuated"
	case AlarmIDContainmentIsolA:
		return "Containment isolation phase A actuated"
	case AlarmIDContainmentIsolB:
		return "Containment isolation phase B actuated"
	case AlarmIDAFWStart:
		return "Auxiliary feedwater pumps start actuated"
	case AlarmIDFeedwaterIsolation:
		return "Feedwater isolation actuated"
	}
	return string(id)
}

// Alarms returns the current alarm list, aggregated across all
// plant subsystems. The list contains only entries whose
// underlying condition is currently active; the control room
// maintains its own acknowledged-and-stale view in AlarmSet.
//
// Implementation note: this is a read-only sensor — it does not
// mutate any subsystem state. Subsystem step functions own the
// alarm thresholds and latches; this function only mirrors
// their state into a uniform shape for the HMI.
func (p *Plant) Alarms() []Alarm {
	var list []Alarm

	// Turbine OPC alarm (informative, 103%).
	// "OPC alarm (103%) [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:71-75
	if p.TurbineBuilding.Turbine.OPCAlarm {
		list = append(list, Alarm{
			ID:          AlarmIDOPCAlarm,
			Description: alarmDescription(AlarmIDOPCAlarm),
			Severity:    SeverityInfo,
			Active:      true,
		})
	}
	// Turbine OPC trip (protective, 110%).
	// "OPC trip (110%) [...]"
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:71-75
	if p.TurbineBuilding.Turbine.OPCTrip {
		list = append(list, Alarm{
			ID:          AlarmIDOPCTrip,
			Description: alarmDescription(AlarmIDOPCTrip),
			Severity:    SeverityTrip,
			Active:      true,
		})
	}
	// RPS reactor trip (protective).
	// "Monitors the plant for abnormal conditions [...]
	// Automatically provides reactor trip (shutdown) signals."
	// specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt:55-72
	if p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped {
		list = append(list, Alarm{
			ID:          AlarmIDReactorTrip,
			Description: alarmDescription(AlarmIDReactorTrip),
			Severity:    SeverityTrip,
			Active:      true,
		})
	}

	// Stator coil cooling water alarm/trip.
	// "Demineralized cooling water is provided for the
	// stator by a closed recirculating water system."
	// specs/the_westinghouse_pressurized_water_react/chapter_15/page_209.txt:48-50
	sccw := SCCWAlarm(
		p.TurbineBuilding.Generator.SCCWPumpRunning,
		p.TurbineBuilding.Generator.SCCWFlow,
	)
	if sccw.Trip {
		list = append(list, Alarm{
			ID:          AlarmIDSCCWNoFlow,
			Description: alarmDescription(AlarmIDSCCWNoFlow),
			Severity:    SeverityTrip,
			Active:      true,
		})
	} else if sccw.Alarm {
		list = append(list, Alarm{
			ID:          AlarmIDSCCWLowFlow,
			Description: alarmDescription(AlarmIDSCCWLowFlow),
			Severity:    SeverityWarning,
			Active:      true,
		})
	}

	// Diesel alarms. The simplified model collapses the
	// various protective trips (overspeed, coolant
	// over-temperature, loss of fuel, loss of lube oil)
	// into a single DieselTripped latch
	// (core/diesel.go:62-71,121,147). The coolant and
	// fuel conditions also raise dedicated warning
	// alarms so the operator can see the precursor
	// state separately from the latched trip. Both trains'
	// diesels alarm into the same annunciators.
	for _, d := range []DieselGenerator{
		p.AuxiliaryBuilding.DieselGenerator,
		p.AuxiliaryBuilding.DieselGeneratorB,
	} {
		if d.State == DieselTripped {
			list = append(list, Alarm{
				ID:          AlarmIDDieselTripped,
				Description: alarmDescription(AlarmIDDieselTripped),
				Severity:    SeverityTrip,
				Active:      true,
			})
		}
		if d.CoolantTemp.Float() >= DieselCoolantTripTemp.Float() {
			list = append(list, Alarm{
				ID:          AlarmIDDieselCoolant,
				Description: alarmDescription(AlarmIDDieselCoolant),
				Severity:    SeverityWarning,
				Active:      true,
			})
		}
		if d.Fuel.Level.Float() <= DieselLowFuelLevel.Float() {
			list = append(list, Alarm{
				ID:          AlarmIDDieselLowFuel,
				Description: alarmDescription(AlarmIDDieselLowFuel),
				Severity:    SeverityWarning,
				Active:      true,
			})
		}
	}

	// Turbine emergency-trip devices beyond OPC. Each latches the
	// machine into Tripped with the stop valves shut.
	//   "Low vacuum trip" / "Low bearing oil pressure trip"
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_206.txt:25-26
	turb := p.TurbineBuilding.Turbine
	for _, c := range []struct {
		latched bool
		id      AlarmID
	} {
		{turb.TripByVacuum, AlarmIDTurbineTripVacuum},
		{turb.TripByBearingOil, AlarmIDTurbineTripBearingOil},
	} {
		if c.latched {
			list = append(list, Alarm{
				ID:          c.id,
				Description: alarmDescription(c.id),
				Severity:    SeverityTrip,
				Active:      true,
			})
		}
	}

	// Automatic turbine-trip stepback in progress.
	//   "The reactor neutron power will be reduced quickly to 60 % by
	//    rapid insertion of control rods"
	//   specs/pwr_sim/chapter_3/page_041.txt:28-29
	if p.PlantOperation.StepbackActive {
		list = append(list, Alarm{
			ID:          AlarmIDTurbineStepback,
			Description: alarmDescription(AlarmIDTurbineStepback),
			Severity:    SeverityWarning,
			Active:      true,
		})
	}

	// Generator apparent-power limit clamp.
	//   "It [...] monitors output power [...]."
	//   specs/the_westinghouse_pressurized_water_react/chapter_15/page_204.txt:33-36
	if p.TurbineBuilding.Generator.GeneratorMVALimitAlarm {
		list = append(list, Alarm{
			ID:          AlarmIDGeneratorMVALimit,
			Description: alarmDescription(AlarmIDGeneratorMVALimit),
			Severity:    SeverityWarning,
			Active:      true,
		})
	}

	// ESFAS actuation latches per Table 7-1.
	//   "Automatically provides reactor trip (shutdown) signals and
	//    engineered safeguards actuation signals [...]"
	//   specs/the_westinghouse_pressurized_water_react/chapter_7/page_137.txt:55-72
	rps := p.ReactorBuilding.ReactorProtectionSystem
	for _, c := range []struct {
		latched bool
		id      AlarmID
		sev     Severity
	} {
		{rps.SafetyInjectionActuated, AlarmIDSIActuated, SeverityTrip},
		{rps.SteamlineIsolationActuated, AlarmIDSteamlineIsolation, SeverityTrip},
		{rps.ContainmentSprayActuated, AlarmIDContainmentSpray, SeverityTrip},
		{rps.ContainmentIsolationPhaseAActuated, AlarmIDContainmentIsolA, SeverityTrip},
		{rps.ContainmentIsolationPhaseBActuated, AlarmIDContainmentIsolB, SeverityTrip},
		{rps.AuxiliaryFeedPumpsStartActuated, AlarmIDAFWStart, SeverityWarning},
		{rps.FeedwaterIsolationActuated, AlarmIDFeedwaterIsolation, SeverityTrip},
	} {
		if c.latched {
			list = append(list, Alarm{
				ID:          c.id,
				Description: alarmDescription(c.id),
				Severity:    c.sev,
				Active:      true,
			})
		}
	}

	// Overlay the activation timestamps the control room
	// recorded in AlarmSet; the per-condition reads above
	// construct fresh values and carry no timing of their own.
	set := p.ReactorBuilding.PlantControlRoom.AlarmSet
	for i := range list {
		if stamped, ok := set[list[i].ID]; ok {
			list[i].Timestamp = stamped.Timestamp
			list[i].Acknowledged = stamped.Acknowledged
		}
	}

	return list
}

// ResetAlarm is the operator-initiated entry point that
// acknowledges a single alarm by ID. The alarm remains in the
// AlarmSet until the underlying condition has cleared; once the
// condition has cleared the alarm is removed on the next step.
//
// "Turned off by operator acknowledgement."
// specs/the_westinghouse_pressurized_water_react/chapter_8/page_145.txt:66-67
func (c *PlantControlRoom) ResetAlarm(id AlarmID) {
	if c.AlarmSet == nil {
		return
	}
	a, ok := c.AlarmSet[id]
	if !ok {
		return
	}
	a.Acknowledged = true
	c.AlarmSet[id] = a
	// Clear the legacy single-bit acknowledgement when the
	// operator has acknowledged the only outstanding alarm.
	// This keeps the existing AcknowledgeAlarm() / BlanketAck
	// semantics consistent for tests that still look at the
	// flat bool.
	c.AlarmAcknowledged = true
	if !c.AnnunciatorAlarm {
		c.AlarmAcknowledged = true
	}
}

// AcknowledgeAll acknowledges every entry in the AlarmSet at
// once. Useful for the common operator pattern of accepting the
// full annunciator panel after a transient alarm burst.
func (c *PlantControlRoom) AcknowledgeAll() {
	for id, a := range c.AlarmSet {
		if a.Active && !a.Acknowledged {
			a.Acknowledged = true
			c.AlarmSet[id] = a
		}
	}
	c.AlarmAcknowledged = true
}
