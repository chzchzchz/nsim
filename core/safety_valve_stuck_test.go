package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stuck-open safety-valve fault latches the relief path open
// regardless of pressure, per the pwr_sim malfunction vocabulary
// ("a valve commanded shut that sticks open", chapter_3/page_068.txt
// context; no safety-valve exercise exists in the cached text,
// so the latch is a simulator construct symmetric with
// PORVStuckOpen). Latch on Set("1"), clear on Set("0"); relay
// back through the aggregate valve state that the step recomputes.
func TestPressurizerSafetyValveStuckOpenLatchesAndClears(t *testing.T) {
	t.Parallel()
	const dt = 100 * time.Millisecond

	p := MakePlant()
	m := GetControlMap(&p)
	fault := m["ReactorBuilding/Faults/PressurizerSafetyValve"]

	// Default plant pressure (2,250 psia) is below the 2,600
	// setpoint, so the unlatched valve rests closed.
	assert.Equal(t, "0", fault.Get())

	require.NoError(t, fault.Set("1"))
	assert.Equal(t, "1", fault.Get())

	// The latch survives steps far below the setpoint.
	p.ReactorBuilding.Pressurizer.Pressure = 1500
	for i := 0; i < 50; i++ {
		p.Step(dt)
		p.Update()
	}
	assert.Equal(t, 1.0, p.ReactorBuilding.SafetyValve.Open,
		"stuck-open latch must pin the valve open below the setpoint")

	// Clearing the latch returns the aggregate to the pressure law.
	require.NoError(t, fault.Set("0"))
	assert.Equal(t, "0", fault.Get())
	p.Step(dt)
	p.Update()
	assert.Equal(t, 0.0, p.ReactorBuilding.SafetyValve.Open,
		"below the setpoint the unlatched valve must reseat")
}

// Same latch contract on the secondary side: the SG safety valve
// stays open regardless of SG header pressure until cleared.
func TestSGSafetyValveStuckOpenLatchesAndClears(t *testing.T) {
	t.Parallel()
	const dt = 100 * time.Millisecond

	p := MakePlant()
	m := GetControlMap(&p)
	fault := m["ReactorBuilding/Faults/SGSafetyValve"]

	// Full-load shell pressure (920 psia) is below the 985 psia
	// setpoint, so the unlatched valve rests closed.
	assert.Equal(t, "0", fault.Get())

	require.NoError(t, fault.Set("1"))
	assert.Equal(t, "1", fault.Get())

	for i := 0; i < 50; i++ {
		p.Step(dt)
		p.Update()
	}
	assert.Equal(t, 1.0, p.ReactorBuilding.SteamGenerator.SafetyValve.Open,
		"stuck-open latch must pin the SG valve open below the setpoint")

	require.NoError(t, fault.Set("0"))
	p.Step(dt)
	p.Update()
	assert.Equal(t, 0.0, p.ReactorBuilding.SteamGenerator.SafetyValve.Open,
		"below the reseat fraction the unlatched valve must close")
}
