package core

import (
	"strconv"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Flow sensors must present gallons-per-minute while the model's storage
// convention is gallons per second (core/plant.go integrates CVCS flows
// against dt.Seconds(); circulating_water.go divides a gpm figure by 60
// at the storage boundary). Each registration converts x60 at its Get;
// fields already stored in true gpm are listed as pass-throughs so the
// next auditor does not re-derive them.
func TestBridge_FlowSensorsPresentGPM(t *testing.T) {
	t.Parallel()
	p := MakePlant()

	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ChargingFlow =
		Volume{Val: 1, Unit: u.Gallon}
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.LetdownFlow =
		Volume{Val: 2, Unit: u.Gallon}
	p.AuxiliaryBuilding.ResidualHeatRemovalSystem.SolidPlantReliefFlow =
		Volume{Val: 3, Unit: u.Gallon}
	p.AuxiliaryBuilding.BoronRecycleSystem.FeedFlow = 4
	p.Outside.CoolingTower.EvaporationFlow =
		Volume{Val: 5, Unit: u.Gallon}
	p.ReactorBuilding.ReactorMakeupControlValve.Flow =
		Volume{Val: 6, Unit: u.Gallon}

	p.Outside.ServiceWaterSystem.Flow = 20000
	p.Outside.CirculatingWaterSystem.FlowGPM = 550000
	p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.PurificationFlow = 100
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.ProgrammedLevel = 40
	p.AuxiliaryBuilding.SpentFuelPoolCoolingSystem.Turbidity = 12
	p.ReactorBuilding.ContainmentBuilding.HydrogenDetectionSuppression.HydrogenVolumePercent = 2.5

	// BreakFlow stores a PER-STEP volume (gal over one ProductionStep), so
	// the gpm presentation divides out the tick: 0.12 gal/step at the
	// 1 ms tick is the 120 gal/s full-open break rate = 7200 gpm.
	p.ReactorBuilding.ReactorPressureVessel.BreakFlow =
		Volume{Val: 0.12, Unit: u.Gallon}
	// HHSIHotLegFlow stores a rate in gal/s (design 970/60 gal/s x the
	// hot-leg fraction); presented like any other gal/s field.
	p.ReactorBuilding.SafetyInjectionSystem.HHSIHotLegFlow =
		Volume{Val: 8, Unit: u.Gallon}
	// BTRSBedSaturation stores a 0..1 fraction; percent presentation
	// scales to the [0,100] range the kind promises.
	p.AuxiliaryBuilding.ChemicalVolumeControlSystem.BTRSBedSaturation = 0.5

	cases := []struct {
		path string
		kind string
		want float64
	}{
		{"AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlow", "gpm", 60},
		{"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownFlow", "gpm", 120},
		{"AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantReliefFlow", "gpm", 180},
		{"AuxiliaryBuilding/BoronRecycleSystem/FeedFlow", "gpm", 240},
		{"Outside/CoolingTower/EvaporationFlow", "gpm", 300},
		{"ReactorBuilding/ReactorMakeupControlValve/Flow", "gpm", 360},
		{"Outside/ServiceWaterSystem/Flow", "gpm", 20000},
		{"Outside/CirculatingWaterSystem/Flow", "gpm", 550000},
		{"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PurificationFlow", "gpm", 100},
		{"ReactorBuilding/ReactorPressureVessel/BreakFlow", "gpm", 7200},
		{"ReactorBuilding/SafetyInjectionSystem/HHSIHotLegFlow", "gpm", 480},
		{"AuxiliaryBuilding/ChemicalVolumeControlSystem/BTRSBedSaturation", "percent", 50},
		{"AuxiliaryBuilding/ChemicalVolumeControlSystem/ProgrammedLevel", "percent", 40},
		{"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/Turbidity", "percent", 12},
		{"ReactorBuilding/ContainmentBuilding/HydrogenDetectionSuppression/HydrogenPercent", "percent", 2.5},
	}

	m := GetSensorMap(&p)
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			s, ok := m[tc.path]
			require.True(t, ok, "sensor %q must be registered", tc.path)
			assert.Equal(t, tc.kind, s.Kind,
				"%s must declare the kind it presents", tc.path)
			out, err := s.Get()
			require.NoError(t, err)
			got, err := strconv.ParseFloat(out, 64)
			require.NoError(t, err, "sensor emitted %q", out)
			assert.Equal(t, tc.want, got,
				"%s must present its declared unit at the presentation boundary", tc.path)
		})
	}
}

// Pressure sensors must declare the unit their backing field actually
// stores. Five fields hold psia (RPV design 2500 psia, SG shell 920
// psia per chapter_3/page_066.txt:80-82, RHRS solid-plant setpoint
// "(psia)" per core/parts.go) but were labeled psig; the audited-psig
// trio (containment design 60 psig, SFPC header 55-75 psig, bearing oil
// 25 psig) stays. Values pass through unscaled - the badge is the fix.
func TestBridge_PressureSensorsDeclareStoredUnit(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.ReactorBuilding.Pressurizer.Pressure = Pressure(2250)
	p.ReactorBuilding.SteamGenerator.Pressure = Pressure(920)
	p.ReactorBuilding.ReactorPressureVessel.Pressure = Pressure(2500)
	p.TurbineBuilding.Turbine.FirstStagePressure = Pressure(920)
	p.TurbineBuilding.Condenser.Pressure = Pressure(1.5)

	psiaCases := []struct {
		path string
		want float64
	}{
		{"ReactorBuilding/Pressurizer/Pressure", 2250},
		{"ReactorBuilding/SteamGenerator/Pressure", 920},
		{"ReactorBuilding/ReactorPressureVessel/Pressure", 2500},
		{"TurbineBuilding/Turbine/FirstStagePressure", 920},
		{"TurbineBuilding/Condenser/Pressure", 1.5},
	}
	psigCases := []string{
		"ReactorBuilding/ContainmentPressure",
		"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/HeaderPressure",
		"TurbineBuilding/Turbine/BearingOilPressure",
	}

	m := GetSensorMap(&p)
	for _, tc := range psiaCases {
		t.Run(tc.path, func(t *testing.T) {
			s, ok := m[tc.path]
			require.True(t, ok, "sensor %q must be registered", tc.path)
			assert.Equal(t, "psia", s.Kind,
				"%s stores psia and must say so", tc.path)
			out, err := s.Get()
			require.NoError(t, err)
			got, err := strconv.ParseFloat(out, 64)
			require.NoError(t, err, "sensor emitted %q", out)
			assert.Equal(t, tc.want, got,
				"%s passes the psia value through unscaled", tc.path)
		})
	}
	// The RHRS solid-plant pressure setpoint is a psia CONTROL (not a
	// sensor): kind, Get passthrough, and Set validation must all agree.
	t.Run("AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureSetpoint", func(t *testing.T) {
		cm := GetControlMap(&p)
		c, ok := cm["AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantPressureSetpoint"]
		require.True(t, ok, "control must be registered")
		assert.Equal(t, "psia", c.Kind,
			"the setpoint stores psia and must say so")
		require.NoError(t, c.Set("415"))
		got, err := strconv.ParseFloat(c.Get(), 64)
		require.NoError(t, err)
		assert.Equal(t, 415.0, got, "setpoint round-trips unscaled")
	})

	for _, path := range psigCases {
		t.Run(path, func(t *testing.T) {
			s, ok := m[path]
			require.True(t, ok, "sensor %q must be registered", path)
			assert.Equal(t, "psig", s.Kind,
				"%s is an audited-psig quantity and keeps its badge", path)
		})
	}
}
