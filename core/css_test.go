package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// At construction the CSS pumps are off, both the CSST and SAT
// are full at their design capacities, the spray header is
// closed, and no spray is flowing. Suction is on the CSST (not
// sump) and sump pH has not yet been adjusted.
func TestCSS_InitialStateIsIdle(t *testing.T) {
	p := MakePlant()
	css := p.AuxiliaryBuilding.ContainmentSpraySystem
	assert.False(t, css.Running,
		"want Running=false at startup, got %v", css.Running)
	assert.False(t, css.SprayHeaderOpen,
		"want SprayHeaderOpen=false at startup, got %v", css.SprayHeaderOpen)
	assert.False(t, css.SuctionSump,
		"want SuctionSump=false at startup, got %v", css.SuctionSump)
	assert.False(t, css.SumpPHAdjusted,
		"want SumpPHAdjusted=false at startup, got %v",
		css.SumpPHAdjusted)
	assert.InDelta(t, CSSTCapacity.Float(),
		css.CSSTLevel.Float(), 1e-9,
		"want CSSTLevel=CSSTCapacity at startup, got %v",
		css.CSSTLevel.Float())
	assert.InDelta(t, SATCapacity.Float(),
		css.SATLevel.Float(), 1e-9,
		"want SATLevel=SATCapacity at startup, got %v",
		css.SATLevel.Float())
	assert.Zero(t, css.SprayFlow,
		"want SprayFlow=0 at startup, got %v", css.SprayFlow)
	assert.InDelta(t, float64(AtmosphericPressure),
		float64(p.ReactorBuilding.ContainmentPressure), 1e-9,
		"want ContainmentPressure=AtmosphericPressure at startup, got %v",
		p.ReactorBuilding.ContainmentPressure)
}

// While running with the spray header open the CSST drains at
// the design flow rate (CSSDesignFlow converted to gal/s).
//
// Spec justification:
//   - "Suction is provided to the spray pump from the CSST, with
//     the SAT providing sodium hydroxide solution."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:61-63
func TestCSS_CSSTDepletesWhileRunning(t *testing.T) {
	c := ContainmentSpraySystem{
		Running:         true,
		DriveReady:      true,
		SprayHeaderOpen: true,
		CSSTLevel:       CSSTCapacity,
		SATLevel:        SATCapacity,
	}
	dt := TimeStep.Seconds()
	wantLoss := (CSSDesignFlow / 60.0) * dt
	c2 := c.Step()
	assert.InDelta(t, CSSTCapacity.Float()-wantLoss,
		c2.CSSTLevel.Float(), 1e-9,
		"want CSSTLevel=%v after one step, got %v",
		CSSTCapacity.Float()-wantLoss,
		c2.CSSTLevel.Float())
}

// The SAT drains proportionally with the CSST so the spray
// additive ratio is preserved.
//
// Spec justification:
//   - "The SAT drains into each spray pump's suction header from
//     the CSST; this mixing results in a solution pH that is
//     conducive to the entrainment of iodine."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:64-66
func TestCSS_SATDepletesProportionallyWithCSST(t *testing.T) {
	c := ContainmentSpraySystem{
		Running:         true,
		DriveReady:      true,
		SprayHeaderOpen: true,
		CSSTLevel:       CSSTCapacity,
		SATLevel:        SATCapacity,
	}
	c2 := c.Step()
	csstLoss := CSSTCapacity.Float() - c2.CSSTLevel.Float()
	satLoss := SATCapacity.Float() - c2.SATLevel.Float()
	wantRatio := SATCapacity.Float() / CSSTCapacity.Float()
	assert.InDelta(t, csstLoss*wantRatio, satLoss, 1e-9,
		"want SAT loss / CSST loss = SATCapacity/CSSTCapacity, got %v / %v",
		satLoss, csstLoss)
}

// When the spray header is closed the miniflow line recirculates
// CSSMiniflowFraction of pump discharge back to the CSST so the
// net drain rate is reduced.
//
// Spec justification:
//   - "A pump miniflow line, containing a heat exchanger and
//     flow limiting orifice."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:20-22
func TestCSS_MiniflowReducesCSSTDrainWhenHeaderClosed(t *testing.T) {
	openHeader := ContainmentSpraySystem{
		Running:         true,
		DriveReady:      true,
		SprayHeaderOpen: true,
		CSSTLevel:       CSSTCapacity,
		SATLevel:        SATCapacity,
	}
	closedHeader := openHeader
	closedHeader.SprayHeaderOpen = false
	dt := TimeStep.Seconds()
	wantLossOpen := (CSSDesignFlow / 60.0) * dt
	wantLossClosed := wantLossOpen * (1.0 - CSSMiniflowFraction)
	openStep := openHeader.Step()
	closedStep := closedHeader.Step()
	openLoss := CSSTCapacity.Float() - openStep.CSSTLevel.Float()
	closedLoss := CSSTCapacity.Float() - closedStep.CSSTLevel.Float()
	assert.Less(t, closedLoss, openLoss,
		"want miniflow loss < open-header loss, got %v vs %v",
		closedLoss, openLoss)
	assert.InDelta(t, wantLossClosed, closedLoss, 1e-9,
		"want closed-header loss=%v, got %v", wantLossClosed, closedLoss)
}

// When CSSTLevel drops below the Lo-Lo setpoint the SumpPHAdjusted
// flag latches true.
//
// Spec justification:
//   - "the sodium hydroxide (spray additive) that is delivered
//     through the injection phase is sufficient to raise the
//     long-term sump pH to a level adequate for inhibiting
//     chloride stress corrosion/cracking of stainless steel
//     components, and for keeping the iodine entrained in the
//     water."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:40-49
func TestCSS_SumpPHAdjustedLatchesBelowLoLo(t *testing.T) {
	c := ContainmentSpraySystem{
		Running:         true,
		DriveReady:      true,
		SprayHeaderOpen: true,
		CSSTLevel:       Volume{Val: CSSLowLevelSetpoint-10, Unit: u.Gallon},
		SATLevel:        Volume{Val: SATCapacity.Float()/CSSTCapacity.Float()*(CSSLowLevelSetpoint-10), Unit: u.Gallon},
	}
	c2 := c.Step()
	assert.True(t, c2.SumpPHAdjusted,
		"want SumpPHAdjusted=true after CSST drains below Lo-Lo, got %v",
		c2.SumpPHAdjusted)
}

// When CSSTLevel drops below the Lo-Lo setpoint the operator
// switches to sump recirculation mode (SuctionSump latches true)
// and the CSST stops draining.
//
// Spec justification:
//   - "when the CSST is drained to the "Lo-Lo" setpoint, the spray
//     pumps are manually switched to the recirculation mode of
//     spray operation."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:18-21
//   - "Spray additive is not injected during this mode."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_115.txt:69-70
func TestCSS_SwitchoverToSumpStopsCSSTDrain(t *testing.T) {
	c := ContainmentSpraySystem{
		Running:         true,
		DriveReady:      true,
		SprayHeaderOpen: true,
		CSSTLevel:       Volume{Val: CSSLowLevelSetpoint-10, Unit: u.Gallon},
		SATLevel:        Volume{Val: 100, Unit: u.Gallon},
	}
	c2 := c.Step()
	assert.True(t, c2.SuctionSump,
		"want SuctionSump=true after Lo-Lo, got %v",
		c2.SuctionSump)
	// Once SuctionSump is latched true, subsequent steps must not
	// drain the CSST further.
	c3 := c2.Step()
	assert.InDelta(t, c2.CSSTLevel.Float(), c3.CSSTLevel.Float(), 1e-9,
		"want CSSTLevel frozen after switchover, got %v → %v",
		c2.CSSTLevel.Float(), c3.CSSTLevel.Float())
}

// SprayFlow tracks CSSDesignFlow when the spray header is open,
// and zero otherwise.
func TestCSS_SprayFlowTracksHeaderState(t *testing.T) {
	c := ContainmentSpraySystem{
		Running:         true,
		DriveReady:      true,
		SprayHeaderOpen: true,
		CSSTLevel:       CSSTCapacity,
		SATLevel:        SATCapacity,
	}
	c2 := c.Step()
	assert.InDelta(t, CSSDesignFlow/60.0, c2.SprayFlow, 1e-9,
		"want SprayFlow=CSSDesignFlow/60, got %v", c2.SprayFlow)
	c.SprayHeaderOpen = false
	c3 := c.Step()
	assert.Zero(t, c3.SprayFlow,
		"want SprayFlow=0 with header closed, got %v", c3.SprayFlow)
}

// End-to-end via Plant.Step: opening the PORV (high RCS pressure)
// drives ContainmentPressure above CSSContainmentPressureSetpoint;
// once the diesel reaches ReadyToLoad the CSS starts and drains
// the CSST.
//
// Spec justification:
//   - "Initial Injection Mode – Containment spray is initiated
//     automatically in response to a containment high pressure
//     signal."
//     specs/the_westinghouse_pressurized_water_react/chapter_6/page_114.txt:55-57
func TestCSS_EndToEndHighContainmentPressureStartsCSS(t *testing.T) {
	p := MakePlant()
	p.AuxiliaryBuilding.DieselGenerator = enoughFuel()
	p.AuxiliaryBuilding.DieselGenerator.StartSignal = DieselStartSafetyInjection
	// Drive containment pressure above the setpoint by forcing
	// the PORV open; the ReactorBuilding.Step containment model
	// rises by 100 psig/s while PORV.Open == 1.
	p.ReactorBuilding.PORV.Open = 1
	p.ReactorBuilding.ContainmentPressure = CSSContainmentPressureSetpoint + 10
	// Step long enough for the diesel to reach ReadyToLoad.
	stepN(&p, int(15*time.Second/TimeStep))
	css := p.AuxiliaryBuilding.ContainmentSpraySystem
	assert.True(t, css.Running,
		"want Running=true after containment high pressure + diesel ready, got %v",
		css.Running)
	assert.True(t, css.SprayHeaderOpen,
		"want SprayHeaderOpen=true after auto-start, got %v",
		css.SprayHeaderOpen)
	assert.Less(t, css.CSSTLevel.Float(), CSSTCapacity.Float(),
		"want CSSTLevel < CSSTCapacity after stepping with CSS running, got %v",
		css.CSSTLevel.Float())
}
