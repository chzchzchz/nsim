package core

import (
	u "github.com/bcicen/go-units"
)

func (r *ReactorBuilding) Step() ReactorBuilding {
	return *r
}

func (t *TurbineBuilding) Step() TurbineBuilding {
	return *t
}

// Plant represents all state relevant to the facility.
type Plant struct {
	ReactorBuilding
	TurbineBuilding
	Outside
	nextPlant *Plant
}

func (p *Plant) Step() {
	p.nextPlant.ReactorBuilding = p.ReactorBuilding.Step()
	p.nextPlant.TurbineBuilding = p.TurbineBuilding.Step()
	p.nextPlant.Outside = p.Outside.Step()
}

func (p *Plant) Update() {
	p.ReactorBuilding = p.nextPlant.ReactorBuilding
	p.TurbineBuilding = p.nextPlant.TurbineBuilding
	p.Outside = p.nextPlant.Outside
}

func MakePlant() Plant {
	p := Plant{nextPlant: &Plant{}}
	p.TurbineBuilding.HeaterDrainTank = Tank{
		Level:    u.NewValue(0, u.Gallon),
		Capacity: u.NewValue(13120, u.Gallon),
		Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
	}
	p.Outside = Outside{
		CondensateStorageTank: Tank{
			Level:    u.NewValue(300000, u.Gallon),
			Capacity: u.NewValue(450000, u.Gallon),
			Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
		},
		DieselGenerator: DieselGenerator{
			Speed: 0,
			Fuel: Tank{
				Level: u.NewValue(0, u.Gallon),
				// Can run feed pump for 24 hours
				// Can run generators for 4 days
				Capacity: u.NewValue(500, u.Gallon),
				Part:     Part{Temperature: u.NewValue(80, u.Fahrenheit)},
			},
			CoolantTemp: u.NewValue(80, u.Fahrenheit),
		},
		RiverTemp: u.NewValue(75, u.Fahrenheit),
	}
	return p
}
