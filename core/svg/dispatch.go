package svg

import "github.com/chzchzchz/nsim/core"

// SymbolForKind returns the symbol id for a Valve.Kind value.
// Unknown kinds fall back to "valve-gate". The id must match
// one of the <symbol id="..."> declarations in
// assets/valve.svg.
func SymbolForKind(kind string) string {
	switch kind {
	case "swing_check":
		return "valve-swing_check"
	case "safety":
		return "valve-safety"
	case "relief":
		return "valve-relief"
	case "porv":
		return "valve-porv"
	case "stop":
		return "valve-stop"
	case "modulating":
		return "valve-modulating"
	}
	return "valve-gate"
}

// SymbolForPump returns a pump symbol id from a DriveSource.
// Used by tests that already know the drive source directly.
func SymbolForPump(isRCP bool, drive core.DriveSource) string {
	if isRCP {
		return "pump-rcp"
	}
	if drive == core.DriveSteam {
		return "pump-turbine"
	}
	return "pump-horizontal"
}

// SymbolForPumpByID picks a pump symbol using the graph-edge
// pump pointer and the source node ID. The source ID alone
// identifies the AFW pump kind (DieselAFWPump / MotorAFWPump /
// TurbineAFWPump), so the diagram does not need a Kind field on
// Pump itself.
func SymbolForPumpByID(p *core.Plant, pump *core.Pump, fromID string) string {
	if pump == &p.ReactorBuilding.ReactorCoolantPump {
		return "pump-rcp"
	}
	switch fromID {
	case "TurbineAFWPump":
		return "pump-turbine"
	}
	return "pump-horizontal"
}
