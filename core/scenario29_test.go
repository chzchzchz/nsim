package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 29 containment spray actuation and recirculation
// switchover: the latched containment-high-pressure ESFAS output
// auto-starts the spray pumps and opens the spray header, the CSST
// drains at the design rate while the SAT follows proportionally, the
// fan coolers shift to low speed when the EDG safety-injection start
// signal arrives, and the operator switchover to sump suction stops
// tank depletion (no further additive injection) without interrupting
// spray delivery to the ring headers.
//
//   - Catalog: plans/x/scenarios.txt item 29.
//   - Auto-start on the containment high pressure signal:
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-61
//   - 30 min additive sizing and Lo-Lo switchover trigger:
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:16-21
//   - Fan coolers to low speed on LOCA sensed:
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:51-61
//   - Manual recirculation mode, additive stopped:
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:63-70
func TestScenario29_LatchedSignalStartsSprayAndSwitchoverStopsAdditive(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	cm := GetControlMap(&p)

	// Time-zero ESFAS output latched exactly as the catalog Apply and
	// the scenario start.json do (core/scenarios.go "containment-spray").
	p.ReactorBuilding.ReactorProtectionSystem.ContainmentSprayActuated = true

	// Safeguards electrical response: both EDGs receive the SI start
	// signal ("The onsite emergency diesel generators are started.",
	// chapter_5/page_097.txt:109-118); this is also the model's
	// LOCA-sensed input for the fan-cooler speed shift.
	require.NoError(t, cm["AuxiliaryBuilding/DieselGenerator/StartSignal"].Set("2"))
	require.NoError(t, cm["AuxiliaryBuilding/DieselGeneratorB/StartSignal"].Set("2"))

	stepN(&p, int(50*time.Millisecond/DefaultTimeStep), DefaultTimeStep)

	css := &p.AuxiliaryBuilding.ContainmentSpraySystem
	assert.True(t, css.Running, "spray pumps must auto-start on the latched signal")
	assert.True(t, css.SprayHeaderOpen, "header isolation valves A must open")
	assert.True(t, css.DriveReady, "offsite-fed ESF bus must ready the drive")
	assert.InDelta(t, CSSDesignFlow/60.0, css.SprayFlow, 0.01,
		"spray delivery must run at the design rate")
	for i := range p.AuxiliaryBuilding.RCFCUnits {
		assert.True(t, p.AuxiliaryBuilding.RCFCUnits[i].LowSpeed,
			"fan cooler %d must shift to low speed once LOCA is sensed", i)
	}

	// Injection-phase tank dynamics through the part's own Step: the
	// CSST drains at design flow with the SAT following at the
	// capacity ratio while suction stays on the tanks. The rate
	// expectations below are wiring checks against production's own
	// CSSDesignFlow constant, not independently sourced numbers.
	injecting := ContainmentSpraySystem{
		Running:                true,
		DriveReady:             true,
		SprayHeaderOpen:        true,
		SATIsolationValveBOpen: true,
		CSSTLevel:              CSSTCapacity,
		SATLevel:               SATCapacity,
	}
	for i := 0; i < int(1*time.Second/DefaultTimeStep); i++ {
		injecting = injecting.Step(DefaultTimeStep)
	}
	assert.InDelta(t, CSSTCapacity.Float()-CSSDesignFlow/60.0,
		injecting.CSSTLevel.Float(), 0.5, "CSST must drain at design flow")
	assert.InDelta(t, SATCapacity.Float()-CSSDesignFlow/60.0*
		(SATCapacity.Float()/CSSTCapacity.Float()),
		injecting.SATLevel.Float(), 0.05, "SAT must drain proportionally")
	assert.False(t, injecting.SuctionSump,
		"suction stays on the CSST during injection")
	assert.False(t, injecting.SumpPHAdjusted,
		"sump pH adjustment waits for the Lo-Lo crossing")

	// Operator recirculation switchover at the Lo-Lo setpoint: pumps
	// move to sump suction, additive injection stops, but the header
	// keeps delivering from the sump at the design rate.
	require.NoError(t, cm["AuxiliaryBuilding/ContainmentSpraySystem/SuctionSump"].Set("1"))
	csstAtSwitch := css.CSSTLevel.Float()
	satAtSwitch := css.SATLevel.Float()

	stepN(&p, int(500*time.Millisecond/DefaultTimeStep), DefaultTimeStep)

	assert.Equal(t, csstAtSwitch, css.CSSTLevel.Float(),
		"CSST must stop draining once switched to sump suction")
	assert.Equal(t, satAtSwitch, css.SATLevel.Float(),
		"additive injection must stop in recirculation mode")
	assert.True(t, css.SuctionSump)
	assert.InDelta(t, CSSDesignFlow/60.0, css.SprayFlow, 0.01,
		"spray delivery must continue from the sump at the design rate")
}
