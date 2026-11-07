package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlantMarshalUnmarshal(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.Electrical.ESFBusA.Energized = true
	p.Electrical.OffsiteSourceA.Voltage = DieselRatedVoltage
	p.Outside.RiverTemp.Val = 88
	p.ReactorBuilding.ReactorCoolantPumps.SetAllRunning(true)
	p.ReactorBuilding.ReactorCoolantPump.Damage = 0.25
	p.Outside.CondensateStorageTank.Level.Val = 123456
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.CSTAligned = true
	p.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.CSTAligned = true

	b, err := json.Marshal(p)
	require.NoError(t, err)

	var p2 Plant
	require.NoError(t, json.Unmarshal(b, &p2))

	assert.Nil(t, p2.nextPlant)
	assert.Equal(t, p.Electrical.ESFBusA.Energized, p2.Electrical.ESFBusA.Energized,
		"ESF bus energized flag must survive the round trip")
	assert.Equal(t, p.Electrical.OffsiteSourceA.Voltage, p2.Electrical.OffsiteSourceA.Voltage,
		"offsite source voltage must survive the round trip")
	assert.True(t, p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemA.CSTAligned,
		"CSTAligned for subsystem A must survive the round trip")
	assert.True(t, p2.AuxiliaryBuilding.EmergencyFeedwaterSystem.SubsystemB.CSTAligned,
		"CSTAligned for subsystem B must survive the round trip")

	// nextPlant is internal simulation scratch state: excluded from
	// JSON by design and rebuilt by Step, so it is not part of the
	// round-trip contract.
	p.nextPlant = nil
	assert.Equal(t, p, p2,
		"every plant field must survive the round trip")

	b2, err := json.Marshal(&p2)
	require.NoError(t, err)
	require.Equal(t, b, b2, "re-marshaled plant must be byte-identical")
}

// Recorded start states are hand-authored or regenerated artifacts
// (plans/x/scenario/17/start.json builds on a scenario-15 dump), so a
// stale turbine setpoint memory can slip into an ingested state. The
// dump controller compares that memory against the current setpoint to
// detect a sudden load reduction, so ingest must resume edge-neutral:
// the rejection decision belongs to commands written after the load,
// not to whatever the file happens to carry.
//
//	"These valves open under a sudden reduction in turbine-generator
//	 load (in excess of 10%) or a turbine trip."
//	specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:9-11
func TestPlant_UnmarshalResumesWithoutSetpointEdge(t *testing.T) {
	t.Parallel()
	p := MakePlant()
	p.TurbineBuilding.Turbine.Setpoint = 1.0
	b, err := json.Marshal(p)
	require.NoError(t, err)

	var recorded map[string]any
	require.NoError(t, json.Unmarshal(b, &recorded))
	turbineBuilding := recorded["TurbineBuilding"].(map[string]any)
	turbine := turbineBuilding["Turbine"].(map[string]any)
	turbine["LastSetpoint"] = 0.0
	stale, err := json.Marshal(recorded)
	require.NoError(t, err)

	var p2 Plant
	require.NoError(t, json.Unmarshal(stale, &p2))

	assert.Equal(t, p2.TurbineBuilding.Turbine.Setpoint,
		p2.TurbineBuilding.Turbine.LastSetpoint,
		"ingested states must resume with no pending setpoint edge")
}
