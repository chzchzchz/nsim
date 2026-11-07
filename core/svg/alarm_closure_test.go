package svg

import (
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/require"
)

// TestAlarmID_Closure: every declared AlarmID (a) is either halo-mapped
// in alarmParts or carries a non-empty exemption rationale, and
// (b) has a raise site in Plant.Alarms().
//
// This guards the declared-but-unwired failure the G5 audit found
// (halo-true, raise-false for five ids). The compiler does not check
// raise sites; the hand-reviewed table in svg.go is the appeal.
func TestAlarmID_Closure(t *testing.T) {
	t.Parallel()
	// Declared set enumerates core.AllAlarmIDs (mirroring the const
	// block in core/alarm.go in block order).
	require.Len(t, core.AllAlarmIDs, 25,
		"AllAlarmIDs must mirror the 25 declared const rows in core/alarm.go")
	for _, id := range core.AllAlarmIDs {
		// (a) halo-or-exemption
		if _, ok := alarmParts[id]; !ok {
			exempt := false
			for _, ex := range alarmHaloExempt {
				if ex.ID == id {
					exempt = true
					require.NotEmpty(t, ex.Rationale,
						"alarm %s: exemption without rationale", id)
				}
			}
			require.True(t, exempt,
				"alarm %s: no halo target and no exemption (svg.go alarmParts)", id)
		}
		// (b) raise site
		require.True(t, alarmRaiseSite[id],
			"alarm %s: no raise site in Plant.Alarms() (core/alarm.go)", id)
	}
	// Every entry in the exemption slice must name a declared id.
	for _, ex := range alarmHaloExempt {
		require.Contains(t, core.AllAlarmIDs, ex.ID,
			"exempt id %s not in AllAlarmIDs", ex.ID)
	}
	// Every row in the raise-site registry must name a declared id.
	for id := range alarmRaiseSite {
		require.Contains(t, core.AllAlarmIDs, id,
			"raise-site id %s not in AllAlarmIDs", id)
	}
}

// TestHalo_RPSInitiatingSignals checks that the three G5 halo rows
// render their mapped parts with the trip stroke when the alarm is
// unacknowledged, and steady when acknowledged.
func TestHalo_RPSInitiatingSignals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// plantValue plants the raw process value directly, no Step.
		plantValue func(p *core.Plant)
	}{
		{
			name: "low steamline pressure ambers/red SG",
			plantValue: func(p *core.Plant) {
				p.ReactorBuilding.SteamGenerator.Pressure = 599
			},
		},
		{
			name: "low Tcold reds RPV",
			plantValue: func(p *core.Plant) {
				p.ReactorBuilding.ReactorPressureVessel.Inlet = core.Temperature{Val: 529, Unit: u.Fahrenheit}
			},
		},
		{
			name: "containment high pressure reds CIS",
			plantValue: func(p *core.Plant) {
				p.ReactorBuilding.ContainmentBuilding.Pressure =
					core.AtmosphericPressure + core.ContainmentDesignPressure + 1
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := core.MakePlant()
			tc.plantValue(&p)
			out := renderPlant(t, &p)
			require.Contains(t, out, `stroke="#e02020"`,
				"rps initiating signal must red its part")
			require.Contains(t, out, `class="blink"`,
				"unacknowledged alarm must flash")
		})
	}
	// Ack leg: acknowledged -> steady, no blink.
	p := core.MakePlant()
	p.ReactorBuilding.SteamGenerator.Pressure = 599
	p.ReactorBuilding.PlantControlRoom.AlarmSet = map[core.AlarmID]core.Alarm{
		core.AlarmIDLowSteamlinePressure: {Active: true, Acknowledged: true},
	}
	out := renderPlant(t, &p)
	require.Contains(t, out, `stroke="#e02020"`,
		"acknowledged alarm must hold steady")
	require.NotContains(t, out, `class="blink"`,
		"acknowledged alarm must not blink")
}

