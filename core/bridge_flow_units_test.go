package core

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	u "github.com/bcicen/go-units"
)

// Flow sensors must present gallons-per-minute while the model's storage
// convention is gallons per second (core/plant.go integrates CVCS flows
// against dt.Seconds(); circulating_water.go divides a gpm figure by 60
// at the storage boundary). Each registration converts x60 at its Get;
// fields already stored in true gpm are listed as pass-throughs so the
// next auditor does not re-derive them.
func TestBridge_FlowSensorsPresentGPM(t *testing.T) {
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

	cases := []struct {
		path string
		want float64
	}{
		{"AuxiliaryBuilding/ChemicalVolumeControlSystem/ChargingFlow", 60},
		{"AuxiliaryBuilding/ChemicalVolumeControlSystem/LetdownFlow", 120},
		{"AuxiliaryBuilding/ResidualHeatRemovalSystem/SolidPlantReliefFlow", 180},
		{"AuxiliaryBuilding/BoronRecycleSystem/FeedFlow", 240},
		{"Outside/CoolingTower/EvaporationFlow", 300},
		{"ReactorBuilding/ReactorMakeupControlValve/Flow", 360},
		{"Outside/ServiceWaterSystem/Flow", 20000},
		{"Outside/CirculatingWaterSystem/Flow", 550000},
		{"AuxiliaryBuilding/SpentFuelPoolCoolingSystem/PurificationFlow", 100},
	}

	m := GetSensorMap(&p)
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			s, ok := m[tc.path]
			require.True(t, ok, "sensor %q must be registered", tc.path)
			out, err := s.Get()
			require.NoError(t, err)
			got, err := strconv.ParseFloat(out, 64)
			require.NoError(t, err, "sensor emitted %q", out)
			assert.Equal(t, tc.want, got,
				"%s must present gpm at the presentation boundary", tc.path)
		})
	}
}
