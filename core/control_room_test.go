package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// At construction the control room has electrical power, the
// PCC and auxiliary panels are operational, all displays are
// active, control inputs are accepted, and no alarm is
// sounding.
func TestControlRoom_InitialStateHasPowerAndOperational(t *testing.T) {
	c := PlantControlRoom{
		PowerAvailable: true,
	}
	c = c.Step(false, true, true)
	assert.True(t, c.PCCAvailable,
		"want PCCAvailable=true with power, got %v", c.PCCAvailable)
	assert.True(t, c.AuxiliaryPanelsAvailable,
		"want AuxiliaryPanelsAvailable=true with power, got %v",
		c.AuxiliaryPanelsAvailable)
	assert.True(t, c.GraphicDisplaysAvailable,
		"want GraphicDisplaysAvailable=true with power, got %v",
		c.GraphicDisplaysAvailable)
	assert.True(t, c.DedicatedDisplaysAvailable,
		"want DedicatedDisplaysAvailable=true with power, got %v",
		c.DedicatedDisplaysAvailable)
	assert.True(t, c.ControlInputsAvailable,
		"want ControlInputsAvailable=true with power, got %v",
		c.ControlInputsAvailable)
	assert.True(t, c.SafetySystemStationsAvailable,
		"want SafetySystemStationsAvailable=true with power, got %v",
		c.SafetySystemStationsAvailable)
	assert.False(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=false at startup, got %v",
		c.AnnunciatorAlarm)
}

// When electrical power is unavailable every display and
// control is dark, and the safety-system stations are not
// reporting.
func TestControlRoom_NoPowerDegradesAllSubsystems(t *testing.T) {
	c := PlantControlRoom{
		PowerAvailable: false,
	}
	c = c.Step(false, true, true)
	assert.False(t, c.PCCAvailable,
		"want PCCAvailable=false without power, got %v",
		c.PCCAvailable)
	assert.False(t, c.AuxiliaryPanelsAvailable,
		"want AuxiliaryPanelsAvailable=false without power, got %v",
		c.AuxiliaryPanelsAvailable)
	assert.False(t, c.GraphicDisplaysAvailable,
		"want GraphicDisplaysAvailable=false without power, got %v",
		c.GraphicDisplaysAvailable)
	assert.False(t, c.DedicatedDisplaysAvailable,
		"want DedicatedDisplaysAvailable=false without power, got %v",
		c.DedicatedDisplaysAvailable)
	assert.False(t, c.ControlInputsAvailable,
		"want ControlInputsAvailable=false without power, got %v",
		c.ControlInputsAvailable)
	assert.False(t, c.SafetySystemStationsAvailable,
		"want SafetySystemStationsAvailable=false without power, got %v",
		c.SafetySystemStationsAvailable)
}

// An alarm condition latches AnnunciatorAlarm = true until
// the operator acknowledges it; the condition must persist
// across the acknowledgement transition for the alarm to
// stay latched.
func TestControlRoom_AlarmLatchesAndAcknowledges(t *testing.T) {
	c := PlantControlRoom{PowerAvailable: true}
	c = c.Step(true, true, true)
	assert.True(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=true with alarm condition, got %v",
		c.AnnunciatorAlarm)

	// Operator acknowledges the alarm; the alarm clears
	// because the condition has stopped.
	c.AcknowledgeAlarm()
	c = c.Step(false, true, true)
	assert.False(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=false after ack and condition cleared, got %v",
		c.AnnunciatorAlarm)
}

// A new alarm condition after a previous acknowledgement
// re-triggers the alarm (the operator must re-acknowledge
// the new condition).
func TestControlRoom_NewAlarmAfterAckRetriggers(t *testing.T) {
	c := PlantControlRoom{PowerAvailable: true}
	c = c.Step(true, true, true)
	c.AcknowledgeAlarm()
	c = c.Step(false, true, true)
	assert.False(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=false after ack, got %v",
		c.AnnunciatorAlarm)

	c = c.Step(true, true, true)
	assert.True(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=true on new condition, got %v",
		c.AnnunciatorAlarm)
	assert.False(t, c.AlarmAcknowledged,
		"want AlarmAcknowledged=false on new condition, got %v",
		c.AlarmAcknowledged)
}

// Safety-system stations are available when the auxiliary
// panels are powered and at least one ESF bus is energized.
func TestControlRoom_SafetyStationsTrackESFBuses(t *testing.T) {
	c := PlantControlRoom{PowerAvailable: true}
	c = c.Step(false, false, true)
	assert.True(t, c.SafetySystemStationsAvailable,
		"want SafetySystemStationsAvailable=true with ESF B on, got %v",
		c.SafetySystemStationsAvailable)
	c = c.Step(false, false, false)
	assert.False(t, c.SafetySystemStationsAvailable,
		"want SafetySystemStationsAvailable=false with no ESF bus, got %v",
		c.SafetySystemStationsAvailable)
}

// When the plant wires PlantControlRoom into Plant.Step,
// the initial offsite-healthy state leaves the control room
// fully operational with no active alarm.
func TestPlant_ControlRoom_InitialStatePreserved(t *testing.T) {
	p := MakePlant()
	p.Step()
	p.Update()
	c := p.ReactorBuilding.PlantControlRoom
	assert.True(t, c.PCCAvailable,
		"want PCCAvailable=true after Plant step, got %v",
		c.PCCAvailable)
	assert.True(t, c.GraphicDisplaysAvailable,
		"want GraphicDisplaysAvailable=true after Plant step, got %v",
		c.GraphicDisplaysAvailable)
	assert.False(t, c.AnnunciatorAlarm,
		"want AnnunciatorAlarm=false at Plant startup, got %v",
		c.AnnunciatorAlarm)
}
