package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 33 (component cooling water train lineup) durable assertions,
// per plans/x/scenario/33.txt. The lumped model relaxes the CCW inlet on
// a first-order lag whose anchors come from the spec:
//
//   - "The component cooling water inlet temperature to various components
//     during normal cooldown is permitted to increase to 120°F (49°C),
//     however it must return to 105°F (40°C) after four hours."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:110-112
//   - "Failure of a pump will not affect the time required for cooldown
//     because a standby pump in each train of the Safeguards subsystem is
//     provided."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_105.txt:92-94

// Losing the ultimate heat sink soaks the inlet up to the permitted
// cooldown maximum and never past it: the cap is a hard ceiling of the
// relaxation, not a clamp bolted onto the output.
func TestScenario33_NoServiceWaterSoaksToCooldownCap(t *testing.T) {
	t.Parallel()
	testDt := 10 * time.Second
	c := CCWS{
		InletTemp:   Temperature{Val: CCWSInletSteadyStateTemp, Unit: u.Fahrenheit},
		PumpRunning: true,
		DriveReady:  true,
	}

	for i := 0; i < 3000; i++ { // 30,000 s: far past any transient
		c = c.Step(testDt, Temperature{Val: 75, Unit: u.Fahrenheit}, false)
		assert.LessOrEqual(t, c.InletTemp.Float(), CCWSInletMaxCooldownTemp,
			"the soak must never exceed the permitted cooldown maximum")
	}

	assert.InDelta(t, CCWSInletMaxCooldownTemp, c.InletTemp.Float(), 0.01,
		"the soak must land on the permitted maximum")
}

// Restoring service water returns the inlet to the design value inside the
// spec's four-hour allowance, approaching from above without undershoot.
func TestScenario33_ServiceWaterRestorationRecoversByFourHours(t *testing.T) {
	t.Parallel()
	testDt := time.Minute
	c := CCWS{
		InletTemp:   Temperature{Val: CCWSInletMaxCooldownTemp, Unit: u.Fahrenheit},
		PumpRunning: true,
		DriveReady:  true,
	}

	prev := c.InletTemp.Float()
	for i := 0; i <= int(4*time.Hour/time.Minute); i++ {
		c = c.Step(testDt, Temperature{Val: 75, Unit: u.Fahrenheit}, true)
		assert.LessOrEqual(t, c.InletTemp.Float(), prev,
			"recovery must be monotone toward the design inlet")
		assert.GreaterOrEqual(t, c.InletTemp.Float(), CCWSInletSteadyStateTemp,
			"the HX cannot cool below the design inlet floor")
		prev = c.InletTemp.Float()
	}

	assert.LessOrEqual(t, c.InletTemp.Float(), 106.0,
		"the inlet must have returned to the design value inside four hours, got %v",
		c.InletTemp.Float())
}

// The pump auto-replacement analog: while an ESF bus is energised the
// plant re-latches a cleared pump latch on its own, so a pump failure does
// not extend cooldown for want of a start signal.
func TestScenario33_PumpLatchAutoReplacesWhileBusUp(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning = false

	stepN(&p, 1, DefaultTimeStep)

	require.True(t, p.nextPlant.Electrical.ESFBusA.Energized ||
		p.nextPlant.Electrical.ESFBusB.Energized,
		"precondition: an ESF bus is energised")
	assert.True(t, p.AuxiliaryBuilding.ComponentCoolingWaterSystem.PumpRunning,
		"the plant must re-latch the CCW pump while bus power is present")
}
