package core

import (
	"encoding/json"
	"testing"
)

func TestPlantMarshalUnmarshal(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.Energized = true
	p.Electrical.OffsiteSourceA.Voltage = DieselRatedVoltage

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}

	var p2 Plant
	if err := json.Unmarshal(b, &p2); err != nil {
		t.Fatal(err)
	}

	if p.Electrical.ESFBusA.Energized != p2.Electrical.ESFBusA.Energized {
		t.Fatalf("round-trip failed: %v != %v", p.Electrical.ESFBusA.Energized, p2.Electrical.ESFBusA.Energized)
	}
	if p.Electrical.OffsiteSourceA.Voltage != p2.Electrical.OffsiteSourceA.Voltage {
		t.Fatalf("round-trip failed: %v != %v", p.Electrical.OffsiteSourceA.Voltage, p2.Electrical.OffsiteSourceA.Voltage)
	}
	if p2.nextPlant != nil {
		t.Fatal("nextPlant should be nil after unmarshal")
	}
}
