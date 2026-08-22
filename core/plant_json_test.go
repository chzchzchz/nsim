package core

import (
	"encoding/json"
	"testing"
)

func TestPlantMarshalUnmarshal(t *testing.T) {
	p := MakePlant()
	p.Electrical.ESFBusA.Energized = true

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
	if p2.nextPlant != nil {
		t.Fatal("nextPlant should be nil after unmarshal")
	}
}
