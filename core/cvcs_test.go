package core

import (
	"testing"
	"time"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
)

// Volume control mode is the default: charging and letdown
// balance and the boron concentration is unchanged.
//
// Spec justification:
//   - "The pressurizer level is controlled by balancing the
//     charging flow to the letdown flow."
//     specs/the_westinghouse_pressurized_water_react/chapter_7/page_136.txt:11-12
func TestCVCS_VolumeControlKeepsBoronSteady(t *testing.T) {
	c := (&CVCS{
		Mode:                CVCSVolumeControl,
		VCTLevel:            Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration:  CVCSInitialBoron,
		ChargingPumpRunning: true,
		LetdownValveOpen:    true,
	}).Step(Volume{Val: 0, Unit: u.Gallon})
	assert.Equal(t, CVCSInitialBoron, c.BoronConcentration,
		"want BoronConcentration unchanged in VolumeControl, got %v",
		c.BoronConcentration)
	assert.InDelta(t, CVCSDesignChargingFlow.Float(), c.ChargingFlow.Float(), 1e-9,
		"want ChargingFlow=CVCSDesignChargingFlow, got %v", c.ChargingFlow)
	assert.InDelta(t, CVCSDesignLetdownFlow.Float(), c.LetdownFlow.Float(), 1e-9,
		"want LetdownFlow=CVCSDesignLetdownFlow, got %v", c.LetdownFlow)
}

// In boration mode (BTRSRecirculationPumpRunning), the RCS boron
// concentration rises at CVCSBorationRate per second.
//
// Spec justification:
//   - "for boration, an increase in the boric acid concentration
//     of the reactor coolant is accomplished by sending the flow
//     from the VCT, at relatively high temperatures (140°F)
//     (60°C), to the thermal regeneration demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:16-19
func TestCVCS_BorationIncreasesBoronConcentration(t *testing.T) {
	c := (&CVCS{
		Mode:                         CVCSBoration,
		VCTLevel:                     Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration:           CVCSInitialBoron,
		BTRSRecirculationPumpRunning: true,
		ChargingPumpRunning:          true,
		LetdownValveOpen:             true,
	}).Step(Volume{Val: 0, Unit: u.Gallon})
	assert.InDelta(t, CVCSBorationRate, c.BorationRate, 1e-9,
		"want BorationRate=CVCSBorationRate, got %v", c.BorationRate)
	assert.Greater(t, c.BoronConcentration, CVCSInitialBoron,
		"want BoronConcentration > initial after boration step, got %v",
		c.BoronConcentration)
}

// In dilution mode (BTRSRecirculationPumpRunning), the RCS boron
// concentration falls at CVCSDilutionRate per second.
//
// Spec justification:
//   - "for dilution, a decrease in the boric acid concentration
//     in the reactor coolant is accomplished by sending the flow
//     from the VCT, at relatively low temperatures (50°F) (10°C),
//     to the thermal regeneration demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:26-30
func TestCVCS_DilutionDecreasesBoronConcentration(t *testing.T) {
	c := (&CVCS{
		Mode:                         CVCSDilution,
		VCTLevel:                     Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration:           CVCSInitialBoron,
		BTRSRecirculationPumpRunning: true,
		ChargingPumpRunning:          true,
		LetdownValveOpen:             true,
	}).Step(Volume{Val: 0, Unit: u.Gallon})
	assert.InDelta(t, CVCSDilutionRate, c.DilutionRate, 1e-9,
		"want DilutionRate=CVCSDilutionRate, got %v", c.DilutionRate)
	assert.Less(t, c.BoronConcentration, CVCSInitialBoron,
		"want BoronConcentration < initial after dilution step, got %v",
		c.BoronConcentration)
}

// VCT level rises above VCTHighLevel forces CVCSLetdown mode,
// which halts charging and letdown so the VCT level can
// stabilise. Per the spec, the BTRS outflow is diverted to the
// BRS holdup tanks in this condition.
//
// Spec justification:
//   - "the volume control tank level controller automatically
//     diverts the outflow from the BTRS into the BRS holdup
//     tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
func TestCVCS_HighVCTLevelSwitchesToLetdownMode(t *testing.T) {
	c := (&CVCS{
		Mode:                CVCSVolumeControl,
		VCTLevel:            Volume{Val: VCTHighLevel+1, Unit: u.Gallon},
		BoronConcentration:  CVCSInitialBoron,
		ChargingPumpRunning: true,
		LetdownValveOpen:    true,
	}).Step(Volume{Val: 0, Unit: u.Gallon})
	assert.Equal(t, CVCSLetdown, c.Mode,
		"want CVCSLetdown with VCTLevel>VCTHighLevel, got %v", c.Mode)
	assert.Zero(t, c.ChargingFlow.Float(),
		"want ChargingFlow=0 in CVCSLetdown, got %v", c.ChargingFlow)
	assert.Zero(t, c.LetdownFlow.Float(),
		"want LetdownFlow=0 in CVCSLetdown, got %v", c.LetdownFlow)
}

// VCT level accumulates from charging-pump depletion and BTRS
// recirculation. With the charging pump running the VCT loses
// inventory each step; with the BTRS pump running the loss is
// larger.
//
// Spec justification:
//   - "the volume control tank level controller automatically
//     diverts the outflow from the BTRS into the BRS holdup
//     tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
func TestCVCS_VCTLevelAccumulates(t *testing.T) {
	initial := VCTDesignLevel
	steps := int(1 * time.Second / TimeStep)
	// Charging pump only.
	c := CVCS{
		Mode:                CVCSVolumeControl,
		VCTLevel:            Volume{Val: initial, Unit: u.Gallon},
		BoronConcentration:  CVCSInitialBoron,
		ChargingPumpRunning: true,
		LetdownValveOpen:    true,
	}
	for i := 0; i < steps; i++ {
		c = c.Step(Volume{Val: 0, Unit: u.Gallon})
	}
	wantLoss := CVCSDesignChargingFlow.Float() * 1.0
	assert.InDelta(t, initial-wantLoss, c.VCTLevel.Float(), 1e-3,
		"want VCTLevel=%v after 1 s charging-pump-only, got %v",
		initial-wantLoss, c.VCTLevel.Float())

	// Now with BTRS pump also running.
	c = CVCS{
		Mode:                         CVCSBoration,
		VCTLevel:                     Volume{Val: initial, Unit: u.Gallon},
		BoronConcentration:           CVCSInitialBoron,
		BTRSRecirculationPumpRunning: true,
		ChargingPumpRunning:          true,
		LetdownValveOpen:             true,
	}
	for i := 0; i < steps; i++ {
		c = c.Step(Volume{Val: 0, Unit: u.Gallon})
	}
	wantLossBoth := (CVCSDesignChargingFlow.Float() + VCTBTRSCirculation) * 1.0
	assert.InDelta(t, initial-wantLossBoth, c.VCTLevel.Float(), 1e-3,
		"want VCTLevel=%v after 1 s charging+BTRS, got %v",
		initial-wantLossBoth, c.VCTLevel.Float())
}

// End-to-end: with the reactor at rated power, switching to
// boration mode drives the boron concentration upward at
// CVCSBorationRate per second. The boron-reactivity coefficient
// is negative (boron is a neutron absorber), so the boron
// reactivity contribution (Reactivity term) is negative
// whenever boron is above CVCSInitialBoron.
//
// Spec justification:
//   - Boration mode: "for boration, an increase in the boric
//     acid concentration of the reactor coolant is accomplished
//     by sending the flow from the VCT, at relatively high
//     temperatures (140°F) (60°C), to the thermal regeneration
//     demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:16-19
//   - Boron reactivity: "the boron concentration changes
//     significantly with fuel depletion"
//     specs/the_westinghouse_pressurized_water_react/chapter_2/page_037.txt:118-122
func TestCVCS_BorationIncreasesBoronAtExpectedRate(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	stepN(&p, int(1*time.Second/TimeStep))
	boronBefore := p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration

	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSBoration
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = true
	stepN(&p, int(10*time.Second/TimeStep))
	boronAfter := p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration

	wantRise := CVCSBorationRate * 10.0
	gotRise := boronAfter - boronBefore
	assert.InDelta(t, wantRise, gotRise, wantRise*0.1,
		"want boron rise ≈ %v ppm after 10 s boration, got %v (before=%v after=%v)",
		wantRise, gotRise, boronBefore, boronAfter)
}

// End-to-end dilution test: switching to dilution mode drives
// the boron concentration downward at CVCSDilutionRate per
// second.
//
// Spec justification:
//   - "for dilution, a decrease in the boric acid concentration
//     in the reactor coolant is accomplished by sending the flow
//     from the VCT, at relatively low temperatures (50°F) (10°C),
//     to the thermal regeneration demineralizers."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_080.txt:26-30
func TestCVCS_DilutionDecreasesBoronAtExpectedRate(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	stepN(&p, int(1*time.Second/TimeStep))
	boronBefore := p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration

	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.Mode = CVCSDilution
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSRecirculationPumpRunning = true
	stepN(&p, int(10*time.Second/TimeStep))
	boronAfter := p.ReactorBuilding.ReactorPressureVessel.ControlRods.BoronConcentration

	wantFall := CVCSDilutionRate * 10.0
	gotFall := boronBefore - boronAfter
	assert.InDelta(t, wantFall, gotFall, wantFall*0.1,
		"want boron fall ≈ %v ppm after 10 s dilution, got %v (before=%v after=%v)",
		wantFall, gotFall, boronBefore, boronAfter)
}

// When the operator opens the reactor makeup control valve,
// fresh borated water flows to the VCT. The VCT level rises at
// a rate proportional to the valve Open fraction.
//
// Spec justification:
//   - "Because of the influx of makeup water from the reactor
//     makeup control system, the volume control tank level
//     controller automatically diverts the outflow from the
//     BTRS into the BRS holdup tanks."
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:5-9
//   - "The reactor makeup control system dilution is [...]"
//     specs/the_westinghouse_pressurized_water_react/chapter_4/page_082.txt:17
func TestCVCS_MakeupValveRaisesVCTLevel(t *testing.T) {
	p := MakePlant()
	p.ReactorBuilding.ReactorPressureVessel.ControlRods.Insertion = 0
	p.ReactorBuilding.ReactorPressureVessel.ThermalPower = CoreRatedThermalPower
	// Stop the charging pump and letdown valve so the VCT is
	// not being depleted by charging-pump suction; then
	// opening the makeup valve should be the only inflow.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingPumpRunning = false
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownValveOpen = false
	stepN(&p, int(1*time.Second/TimeStep))
	vctBefore := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()

	// Open the makeup valve to half-flow.
	p.ReactorBuilding.ReactorMakeupControlValve.Open = 0.5
	stepN(&p, int(5*time.Second/TimeStep))
	vctAfter := p.AuxiliaryBuilding.ChemicalVolumeControlSystem.VCTLevel.Float()

	wantRise := ReactorMakeupDesignFlow.Float() * 0.5 * 5.0
	assert.InDelta(t, wantRise, vctAfter-vctBefore, wantRise*0.1,
		"want VCT rise ≈ %v gal after 5 s at 0.5 makeup, got %v (before=%v after=%v)",
		wantRise, vctAfter-vctBefore, vctBefore, vctAfter)
}

// The CVCS charging pump is independent of the reactor coolant
// pumps: the plant used to plumb ReactorCoolantPump.Running as
// the CVCS charging-pump gate, which made RCP shutdown kill CVCS
// charging and vice versa. The Step function now reads
// CVCS.ChargingPumpRunning directly.
//
// Spec justification:
//   - "Centrifugal charging pumps (CVCS) [...] supplied from
//     [the 6600V engineered safety features bus]."
//     specs/the_westinghouse_pressurized_water_react/chapter_5/page_103.txt:11-13,
//     specs/the_westinghouse_pressurized_water_react/chapter_11/page_165.txt:73-80
func TestCVCS_ChargingPumpIndependentOfRCP(t *testing.T) {
	// Charging pump on, RCP off.
	c := (&CVCS{
		Mode:                CVCSVolumeControl,
		VCTLevel:            Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration:  CVCSInitialBoron,
		ChargingPumpRunning: true,
		LetdownValveOpen:    true,
	}).Step(Volume{Val: 0, Unit: u.Gallon})
	assert.InDelta(t, CVCSDesignChargingFlow.Float(),
		c.ChargingFlow.Float(), 1e-9,
		"want ChargingFlow>0 with ChargingPumpRunning=true regardless of RCP")

	// Charging pump off, RCP would be on.
	c = (&CVCS{
		Mode:                CVCSVolumeControl,
		VCTLevel:            Volume{Val: VCTDesignLevel, Unit: u.Gallon},
		BoronConcentration:  CVCSInitialBoron,
		ChargingPumpRunning: false,
		LetdownValveOpen:    true,
	}).Step(Volume{Val: 0, Unit: u.Gallon})
	assert.Zero(t, c.ChargingFlow.Float(),
		"want ChargingFlow=0 with ChargingPumpRunning=false regardless of RCP")
}
