package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 30 (CVCS charging and letdown control through the operating
// cycle) durable assertions, per plans/x/scenario/30.txt:
//
//   - "Charging flow is initiated and the normal letdown flow path is
//     established through the letdown orifices."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_076.txt:33-35
//   - "A boron dilution is started to bring the boron concentration to the
//     estimated critical concentration."
//     specs/the_westinghouse_pressurized_water_react/chapter_16/page_225.txt:36-38
//   - "Low-level signals from the volume control tank initiate reactor
//     makeup control [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_075.txt:1-4

func TestScenario30_ChargingLetdownLineupDelivers(t *testing.T) {
	t.Parallel()

	// Wiring check: the expected charging magnitude is recomputed from
	// the same design constant production uses (tier-3 per
	// plans/CHECK_SKILL.md section 8); letdown asserts existence and
	// cap only, since the sqrt-of-pressure delivery curve is a model
	// convention.
	c := CVCS{
		ChargingPumpRunning: true,
		ChargingFlowDemand:  0.5,
		LetdownValveOpen:    true,
	}
	c = c.Step(Volume{}, Temperature{Val: 557, Unit: u.Fahrenheit}, 0.25, 2250)

	assert.InDelta(t, 0.5*CVCSDesignChargingFlow.Float(),
		c.ChargingFlow.Float(), 0.05*CVCSDesignChargingFlow.Float(),
		"charging flow must follow the manual throttle")
	assert.Greater(t, c.LetdownFlow.Float(), 0.0,
		"an open letdown valve must establish letdown flow")
	assert.LessOrEqual(t, c.LetdownFlow.Float(), CVCSDesignLetdownFlow.Float(),
		"letdown delivery is capped by the orifice design flow")
}

func TestScenario30_BoronSetpointTrackingRampsAndReleases(t *testing.T) {
	t.Parallel()

	c := CVCS{BoronConcentration: CVCSInitialBoron}
	c.BoronTargetConcentration = CVCSInitialBoron - 20
	c.BoronSetpointTracking = true

	rate := CVCSDilutionRate // ppm/s ceiling for the evolution
	steps := int(60 * time.Second / TimeStep)
	for i := 0; i < steps; i++ {
		c = c.Step(Volume{}, Temperature{}, 0.25, 2250)
		if !c.BoronSetpointTracking {
			break
		}
	}

	assert.False(t, c.BoronSetpointTracking,
		"tracking must release once inside the tolerance band")
	assert.InDelta(t, c.BoronTargetConcentration, c.BoronConcentration,
		CVCSBoronSetpointTolerancePpm,
		"the ramp must land on the operator's target")
	assert.GreaterOrEqual(t, c.BoronConcentration,
		CVCSInitialBoron-rate*60.0-CVCSBoronSetpointTolerancePpm,
		"the dilution must respect the BTRS flow-rate clock")
}

func TestScenario30_VCTLowLevelInitiatesMakeup(t *testing.T) {
	t.Parallel()

	p := MakePlant()
	mv := &p.ReactorBuilding.ReactorMakeupControlValve
	mv.Auto = true
	mv.Open = 0

	mv.applyAutoMakeup(250)
	require.True(t, mv.Initiated,
		"a VCT level below VCTLowLevel must latch the makeup initiation")
	assert.Equal(t, 1.0, mv.effectiveOpen(),
		"the initiation forces full makeup regardless of the manual position")

	mv.applyAutoMakeup(VCTDesignLevel)
	assert.False(t, mv.Initiated,
		"restoring the design level releases the initiation latch")

	mv.applyAutoMakeup(400)
	assert.Equal(t, 0.0, mv.effectiveOpen(),
		"between the anchors the previous latch state holds, so no chatter")
}
