package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// At construction the plant computer is on, every display
// format is available, SPDS is active, post-trip review has
// not fired, and no alarm is sounding.
func TestPlantComputer_InitialStateHasAllDisplaysAndSPDS(t *testing.T) {
	c := PlantComputer{}
	c = c.Step(true, false, false, false, true, false)
	assert.True(t, c.DataAcquisitionAvailable,
		"want DataAcquisitionAvailable=true with power, got %v",
		c.DataAcquisitionAvailable)
	assert.True(t, c.MimicDisplaysAvailable,
		"want MimicDisplaysAvailable=true with power, got %v",
		c.MimicDisplaysAvailable)
	assert.True(t, c.BarChartDisplaysAvailable,
		"want BarChartDisplaysAvailable=true with power, got %v",
		c.BarChartDisplaysAvailable)
	assert.True(t, c.TabularDisplaysAvailable,
		"want TabularDisplaysAvailable=true with power, got %v",
		c.TabularDisplaysAvailable)
	assert.True(t, c.GraphicalDisplaysAvailable,
		"want GraphicalDisplaysAvailable=true with power, got %v",
		c.GraphicalDisplaysAvailable)
	assert.True(t, c.DataTrendsAvailable,
		"want DataTrendsAvailable=true with power, got %v",
		c.DataTrendsAvailable)
	assert.True(t, c.PolarDisplaysAvailable,
		"want PolarDisplaysAvailable=true with power, got %v",
		c.PolarDisplaysAvailable)
	assert.True(t, c.SPCSActive,
		"want SPCSActive=true with power and safety on, got %v",
		c.SPCSActive)
	assert.True(t, c.SequenceOfEventsRecording,
		"want SequenceOfEventsRecording=true with power, got %v",
		c.SequenceOfEventsRecording)
	assert.False(t, c.AlarmActive,
		"want AlarmActive=false at startup, got %v", c.AlarmActive)
	assert.False(t, c.PostTripReviewPrinted,
		"want PostTripReviewPrinted=false at startup, got %v",
		c.PostTripReviewPrinted)
}

// When electrical power is unavailable, all six display
// formats are dark and SPDS is not active.
func TestPlantComputer_NoPowerDegradesAllDisplaysAndSPDS(t *testing.T) {
	c := PlantComputer{}
	c = c.Step(false, false, false, false, true, false)
	assert.False(t, c.DataAcquisitionAvailable,
		"want DataAcquisitionAvailable=false without power, got %v",
		c.DataAcquisitionAvailable)
	assert.False(t, c.MimicDisplaysAvailable,
		"want MimicDisplaysAvailable=false without power, got %v",
		c.MimicDisplaysAvailable)
	assert.False(t, c.SPCSActive,
		"want SPCSActive=false without power, got %v", c.SPCSActive)
	assert.False(t, c.SequenceOfEventsRecording,
		"want SequenceOfEventsRecording=false without power, got %v",
		c.SequenceOfEventsRecording)
}

// SPDS is the AND of computer power and safety-systems-
// available; even with power on, SPDS drops if any major
// safeguard is inoperable.
func TestPlantComputer_SPDSRequiresSafetyAvailable(t *testing.T) {
	c := PlantComputer{}
	c = c.Step(true, false, false, false, false, false)
	assert.False(t, c.SPCSActive,
		"want SPCSActive=false with safety unavailable, got %v",
		c.SPCSActive)
}

// A reactor trip latches PostTripReviewPrinted = true on
// the rising edge; the printer does not re-fire on
// subsequent steps while the reactor remains tripped.
func TestPlantComputer_PostTripReviewLatchesOnTrip(t *testing.T) {
	c := PlantComputer{}
	c = c.Step(true, true, true, false, true, false)
	assert.True(t, c.PostTripReviewPrinted,
		"want PostTripReviewPrinted=true after trip, got %v",
		c.PostTripReviewPrinted)
	c = c.Step(true, true, true, false, true, false)
	assert.True(t, c.PostTripReviewPrinted,
		"want PostTripReviewPrinted=true on second step, got %v",
		c.PostTripReviewPrinted)
}

// An alarm condition latches AlarmActive = true until the
// operator acknowledges it via the control room.
func TestPlantComputer_AlarmLatchesAndAcknowledges(t *testing.T) {
	c := PlantComputer{}
	c = c.Step(true, false, false, true, true, false)
	assert.True(t, c.AlarmActive,
		"want AlarmActive=true with alarm condition, got %v",
		c.AlarmActive)
	c = c.Step(true, false, false, false, true, true)
	assert.False(t, c.AlarmActive,
		"want AlarmActive=false after ack, got %v", c.AlarmActive)
}

// The Bypassed / Inoperable Status Indication display is
// active only when both computer power and the safety-
// system stations are available.
func TestPlantComputer_BISIRequiresSafetyAvailable(t *testing.T) {
	c := PlantComputer{}
	c = c.Step(true, false, false, false, false, false)
	assert.False(t, c.BypassedInoperableStatusIndication,
		"want BISI=false without safety available, got %v",
		c.BypassedInoperableStatusIndication)
	c = c.Step(true, false, false, false, true, false)
	assert.True(t, c.BypassedInoperableStatusIndication,
		"want BISI=true with safety available, got %v",
		c.BypassedInoperableStatusIndication)
}

// When the plant wires PlantComputer into Plant.Step, the
// initial offsite-healthy state leaves the computer fully
// operational with SPDS active and no alarm.
func TestPlant_PlantComputer_InitialStatePreserved(t *testing.T) {
	p := MakePlant()
	p.Step()
	p.Update()
	c := p.ReactorBuilding.PlantComputer
	assert.True(t, c.DataAcquisitionAvailable,
		"want DataAcquisitionAvailable=true after Plant step, got %v",
		c.DataAcquisitionAvailable)
	assert.True(t, c.SPCSActive,
		"want SPCSActive=true after Plant step, got %v",
		c.SPCSActive)
	assert.False(t, c.AlarmActive,
		"want AlarmActive=false at Plant startup, got %v",
		c.AlarmActive)
}
