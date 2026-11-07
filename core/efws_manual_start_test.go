package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 19 regression: a manually started EFW pump is an activated
// train, so its discharge flow-modulating valve rides normally full open
// ("These valves will be normally full open when the system is
// activated.", specs/the_westinghouse_pressurized_water_react/
// chapter_5/page_100.txt:1-2) and the pump delivers immediately. The
// pre-fix bridge left the valve at zero on manual starts, so a running
// pump delivered nothing and scenario 19's SG inventory bled out below
// its floor. The operator retains throttling authority after the start:
// the snap happens only on the starting edge.
func TestEFWS_ManualStartOpensValveAndDelivers(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Pressure = Pressure(920)
	cm := GetControlMap(&p)

	require.NoError(t, cm["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePumpRunning"].Set("1"))
	stepN(&p, 2, DefaultTimeStep)

	assert.Equal(t, 1.0,
		float64(p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.FlowModulatingValve.Open),
		"activation opens the discharge valve to its normal position")
	assert.Greater(t,
		p.AuxiliaryBuilding.EmergencyFeedwaterSystem.DeliveredFlow.Float(), 0.0,
		"the started pump delivers immediately")
}

// Throttling after the start stays with the operator: positioning the
// valve below full open holds that position across steps while the pump
// runs.
func TestEFWS_OperatorThrottlePersistsAfterStart(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.SteamGenerator.Pressure = Pressure(920)
	cm := GetControlMap(&p)

	require.NoError(t, cm["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePumpRunning"].Set("1"))
	require.NoError(t, cm["AuxiliaryBuilding/EmergencyFeedwaterSystem/SubsystemA/TurbinePump/FlowModulatingValve/Open"].Set("0.3"))
	stepN(&p, 5, DefaultTimeStep)

	assert.Equal(t, 0.3,
		float64(p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.TurbinePump.FlowModulatingValve.Open),
		"operator throttle position persists")
}
