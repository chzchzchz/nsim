package core

import (
	"encoding/json"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// At construction the alarm aggregator reports no active
// conditions and the operator hasn't acknowledged anything.
func TestPlant_Alarms_EmptyAtConstruction(t *testing.T) {
	p := MakePlant()
	assert.Empty(t, p.Alarms(),
		"want empty alarm list at startup, got %v", p.Alarms())
}

// Pressing the manual-trip pushbutton (chapter 7) raises a
// trip-severity reactor-trip alarm and the annunciator rings.
func TestPlant_Alarms_ReactorTripAppears(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.ReactorBuilding.ReactorProtectionSystem.ManualTrip = true
	p.Step(DefaultTimeStep)
	p.Update()
	found := false
	for _, a := range p.Alarms() {
		if a.ID == AlarmIDReactorTrip {
			found = true
			assert.Equal(t, SeverityTrip, a.Severity,
				"want SeverityTrip, got %v", a.Severity)
			assert.True(t, a.Active,
				"want Active=true, got %v", a.Active)
			assert.False(t, a.Acknowledged,
				"want Acknowledged=false, got %v", a.Acknowledged)
			assert.NotEmpty(t, a.Description)
		}
	}
	require.True(t, found,
		"want AlarmIDReactorTrip in alarm list, got %v", p.Alarms())
}

// Pinning the turbine overspeed trip flag raises the matching
// alarm ID with SeverityTrip. The OPC alarm flag is
// recomputed by the Turbine step from velocity, so we
// exercise it separately by raising Velocity above
// TurbineOPCALarmFraction * rated.
func TestPlant_Alarms_OPCAlarmAndTrip(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.State = TurbineRunning
	p.TurbineBuilding.Turbine.Velocity = Velocity(
		float64(TurbineRatedSpeed) * TurbineOPCALarmFraction * 1.01)
	p.Step(DefaultTimeStep)
	p.Update()
	have := map[AlarmID]Severity{}
	for _, a := range p.Alarms() {
		have[a.ID] = a.Severity
	}
	assert.Equal(t, SeverityInfo, have[AlarmIDOPCAlarm],
		"want OPCAlarm=SeverityInfo, got %v", have[AlarmIDOPCAlarm])

	// Trip case: push velocity above 110% to latch OPCTrip.
	p.TurbineBuilding.Turbine.Velocity = Velocity(
		float64(TurbineRatedSpeed) * TurbineOPCTripFraction * 1.01)
	p.Step(DefaultTimeStep)
	p.Update()
	have = map[AlarmID]Severity{}
	for _, a := range p.Alarms() {
		have[a.ID] = a.Severity
	}
	assert.Equal(t, SeverityTrip, have[AlarmIDOPCTrip],
		"want OPCTrip=SeverityTrip, got %v", have[AlarmIDOPCTrip])
}

// Tripping the diesel engine (e.g. on coolant over-temperature
// or fuel exhaustion) raises the diesel-tripped alarm. Set
// StartSignal to Manual so the diesel stays tripped across
// the next step (otherwise DieselTripped transitions back to
// DieselOff at zero speed and zero start signal).
func TestPlant_Alarms_DieselTripped(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.AuxiliaryBuilding.DieselGenerator.State = DieselTripped
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartManual
	p.Step(DefaultTimeStep)
	p.Update()
	found := false
	for _, a := range p.Alarms() {
		if a.ID == AlarmIDDieselTripped {
			found = true
			assert.Equal(t, SeverityTrip, a.Severity,
				"want SeverityTrip, got %v", a.Severity)
		}
	}
	require.True(t, found,
		"want AlarmIDDieselTripped in alarm list, got %v", p.Alarms())
}

// SCCW flow lost with the pump commanded running trips the
// generator; the alarm list surfaces this as AlarmIDSCCWNoFlow.
func TestPlant_Alarms_SCCWAlarm(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	// MainFeedwaterPump drives SCCWPumpRunning per core/plant.go.
	p.TurbineBuilding.MainFeedwaterPump.Running = true
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Generator.SCCWPumpRunning = true
	p.TurbineBuilding.Generator.SCCWFlow = 0
	// Plant.Alarms() reads the live Generator fields; the
	// next step would overwrite them, so call Alarms()
	// directly without stepping again.
	found := false
	for _, a := range p.Alarms() {
		if a.ID == AlarmIDSCCWNoFlow {
			found = true
			assert.Equal(t, SeverityTrip, a.Severity,
				"want SeverityTrip, got %v", a.Severity)
		}
	}
	require.True(t, found,
		"want AlarmIDSCCWNoFlow in alarm list, got %v", p.Alarms())
}

// Acknowledging a specific alarm by ID sets Acknowledged=true;
// the alarm only disappears from the next Alarms() call if the
// underlying condition has cleared. Trip-severity alarms keep
// the annunciator latched even after acknowledgement.
func TestPlantControlRoom_ResetAlarm_ClearsAcknowledged(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.OPCTrip = true
	p.Step(DefaultTimeStep)
	p.Update()

	c := p.ReactorBuilding.PlantControlRoom
	require.NotEmpty(t, c.AlarmSet,
		"want AlarmSet non-empty after step with active alarm")
	c.ResetAlarm(AlarmIDOPCTrip)
	a := c.AlarmSet[AlarmIDOPCTrip]
	require.NotNil(t, a, "want OPCTrip entry after ResetAlarm")
	assert.True(t, a.Acknowledged,
		"want Acknowledged=true after ResetAlarm, got %v",
		a.Acknowledged)
}

// AnnunciatorAlarm is true iff at least one trip-severity
// alarm is active and unacknowledged.
func TestPlantControlRoom_AlarmSet_AnnunciatorLatch(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.OPCTrip = true
	p.Step(DefaultTimeStep)
	p.Update()

	c := p.ReactorBuilding.PlantControlRoom
	assert.True(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=true with active trip, got %v",
		c.AnnunciatorAlarm)

	p.ReactorBuilding.PlantControlRoom.ResetAlarm(AlarmIDOPCTrip)
	p.Step(DefaultTimeStep)
	p.Update()
	c = p.ReactorBuilding.PlantControlRoom
	assert.True(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=true even after ack while "+
			"condition persists, got %v", c.AnnunciatorAlarm)

	p.TurbineBuilding.Turbine.OPCTrip = false
	p.Step(DefaultTimeStep)
	p.Update()
	c = p.ReactorBuilding.PlantControlRoom
	assert.False(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=false once condition clears, got %v",
		c.AnnunciatorAlarm)
}

// AcknowledgeAll latches every entry at once; common operator
// pattern after a transient alarm burst.
func TestPlantControlRoom_AcknowledgeAll(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.OPCAlarm = true
	p.TurbineBuilding.Turbine.OPCTrip = true
	p.Step(DefaultTimeStep)
	p.Update()

	c := p.ReactorBuilding.PlantControlRoom
	c.AcknowledgeAll()
	for id, a := range c.AlarmSet {
		if a.Active {
			assert.True(t, a.Acknowledged,
				"want %v acknowledged after AcknowledgeAll, got %v",
				id, a)
		}
	}
}

// The bridge sensor serialises the alarm list as JSON; the HMI
// is responsible for decoding it back into objects.
func TestBridge_AlarmsActiveSensor_JSONShape(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.OPCAlarm = true
	m := GetSensorMap(&p)
	s, ok := m["Alarms/Active"]
	require.True(t, ok, "want Alarms/Active sensor")
	out, err := s.Get()
	require.NoError(t, err)
	assert.Contains(t, out, "turbine.opc_alarm",
		"want JSON payload to contain OPCAlarm ID, got %q", out)
}

// The bridge reset control routes an AlarmID through to the
// control room.
func TestBridge_AlarmsResetControl(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.OPCTrip = true
	p.Step(DefaultTimeStep)
	p.Update()
	m := GetControlMap(&p)
	c, ok := m["Alarms/Reset"]
	require.True(t, ok, "want Alarms/Reset control")
	require.NoError(t, c.Set(string(AlarmIDOPCTrip)))
	a := p.ReactorBuilding.PlantControlRoom.AlarmSet[AlarmIDOPCTrip]
	require.NotNil(t, a)
	assert.True(t, a.Acknowledged,
		"want Acknowledged=true after reset, got %v", a)
}

// Plant.Alarms() ignores engines that have not entered a trip
// state, regardless of how long they have been running. This
// is a sanity check on the diesel coolant trip threshold being
// derived from the same constant as the trip latch.
func TestPlant_Alarms_DieselCoolantWarning(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.AuxiliaryBuilding.DieselGenerator.CoolantTemp =
		Temperature{Val: DieselCoolantTripTemp.Float() + 1, Unit: u.Fahrenheit}
	p.AuxiliaryBuilding.DieselGenerator.State = DieselRunning
	p.Step(DefaultTimeStep)
	p.Update()
	found := false
	for _, a := range p.Alarms() {
		if a.ID == AlarmIDDieselCoolant {
			found = true
			assert.Equal(t, SeverityWarning, a.Severity,
				"want SeverityWarning, got %v", a.Severity)
		}
	}
	require.True(t, found,
		"want AlarmIDDieselCoolant warning, got %v", p.Alarms())
}

// A newly seen alarm is stamped with the control room's
// simulation clock at the step that detected it, so the
// operator sees when the condition came in.
func TestPlantControlRoom_AlarmSet_TimestampsActivationTime(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.OPCTrip = true
	p.Step(DefaultTimeStep)
	p.Update()

	c := p.ReactorBuilding.PlantControlRoom
	require.NotEmpty(t, c.AlarmSet)
	a := c.AlarmSet[AlarmIDOPCTrip]
	require.NotNil(t, a)
	assert.Equal(t, int64(DefaultTimeStep), int64(a.Timestamp),
		"want activation stamp at one tick after start, got %v ns",
		a.Timestamp)
	assert.Equal(t, 2*int64(DefaultTimeStep), c.SimTimeNs,
		"clock should advance one ProductionStep per Step call")

	// The condition persists; the stamp must not move.
	p.Step(DefaultTimeStep)
	p.Update()
	assert.Equal(t, int64(DefaultTimeStep),
		int64(p.ReactorBuilding.PlantControlRoom.AlarmSet[AlarmIDOPCTrip].Timestamp),
		"stamp should hold while the condition stays active")
}

// The Plant/Alarms bridge sensor carries the activation time in
// its time_s field so the HMI can render when each alarm fired.
func TestBridge_PlantAlarms_TimeSReflectsActivation(t *testing.T) {
	p := MakePlant()
	p.Step(DefaultTimeStep)
	p.Update()
	p.TurbineBuilding.Turbine.OPCTrip = true
	p.Step(DefaultTimeStep)
	p.Update()

	m := GetSensorMap(&p)
	s, ok := m["Plant/Alarms"]
	require.True(t, ok, "want Plant/Alarms sensor registered")
	out, err := s.Get()
	require.NoError(t, err)

	type row struct {
		ID    string  `json:"id"`
		TimeS float64 `json:"time_s"`
		Acked bool    `json:"acknowledged"`
	}
	var list []row
	require.NoError(t, json.Unmarshal([]byte(out), &list))
	var trip *row
	for i := range list {
		if list[i].ID == string(AlarmIDOPCTrip) {
			trip = &list[i]
		}
	}
	require.NotNil(t, trip, "want opc_trip in %v", out)
	wantS := DefaultTimeStep.Seconds()
	assert.InDelta(t, wantS, trip.TimeS, 1e-9,
		"want time_s %v, got %v", wantS, trip.TimeS)
	assert.False(t, trip.Acked, "fresh alarm starts unacknowledged")
}
