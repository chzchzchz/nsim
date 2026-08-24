package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 23 loss of normal feedwater: pressing the manual
// safety-injection pushbutton must vote a reactor trip and auto-start
// the motor-driven emergency feedwater pumps, delivering within one
// minute of the actuation signal.
//
//   - Catalog: plans/x/scenarios.txt item 23.
//   - Table 7-1 Safety Injection triggers include "Manual safety
//     injection":
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_140.txt:8-15
//   - Auto-start signal table: "Safety injection / Motor driven pumps."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:77-80
//   - Delivery within one minute of the actuation signal:
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_101.txt:4-9
func TestScenario23_ManualSIStartsEFWSMotorPumps(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	cm := GetControlMap(&p)
	c, ok := cm["ReactorBuilding/ReactorProtectionSystem/ManualSI"]
	require.True(t, ok, "ManualSI control not registered")
	require.NoError(t, c.Set("1"), "set ManualSI")

	// One second of plant time already shows delivery, far inside
	// the one-minute window, and keeps the test fast.
	stepN(&p, int(1*time.Second/TimeStep))

	rps := p.ReactorBuilding.ReactorProtectionSystem
	assert.True(t, rps.SafetyInjectionActuated,
		"want SafetyInjectionActuated=true after manual SI")
	assert.True(t, rps.AuxiliaryFeedPumpsStartActuated,
		"want AuxiliaryFeedPumpsStartActuated=true after manual SI")
	assert.True(t, rps.ReactorTripped,
		"manual SI votes the reactor trip")
	assert.True(t, p.ReactorBuilding.ReactorPressureVessel.Trip,
		"reactor trip reaches the vessel latch")

	efws := p.AuxiliaryBuilding.EmergencyFeedwaterSystem
	assert.True(t, efws.Active, "want EFWS autostart latch set")
	assert.True(t, efws.SubsystemA.MotorPump.Running,
		"want SubsystemA motor pump started by SI")
	assert.True(t, efws.SubsystemB.MotorPump.Running,
		"want SubsystemB motor pump started by SI")
	assert.False(t, efws.SubsystemA.TurbinePump.Running,
		"SI starts only the motor-driven pumps")
	assert.GreaterOrEqual(t, p.ReactorBuilding.EFWSFeedwaterFlow.Float(),
		2*EFWSMotorPumpFlow.Float(),
		"both motor pumps delivering within one minute")
}
