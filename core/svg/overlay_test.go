package svg

import (
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strings"
	"testing"

	u "github.com/bcicen/go-units"
	"github.com/chzchzchz/nsim/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderPlant(t *testing.T, p *core.Plant) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, WritePlantSVG(&b, p))
	return b.String()
}

// TestDomeWash_AppearsAboveDesignPressure checks the containment
// outline carries the translucent red wash only while pressure
// exceeds the design value.
func TestDomeWash_AppearsAboveDesignPressure(t *testing.T) {
	p := core.MakePlant()
	assert.NotContains(t, renderPlant(t, &p), "dome-wash")

	p.ReactorBuilding.ContainmentPressure =
		core.AtmosphericPressure + core.ContainmentDesignPressure + 5
	assert.Contains(t, renderPlant(t, &p), `class="dome-wash"`)
}

// TestOverlay_Coverage forces each readout/ring/tint condition and
// asserts the expected overlay fragment appears or disappears.
func TestOverlay_Coverage(t *testing.T) {
	t.Run("diesel running draws green ring", func(t *testing.T) {
		p := core.MakePlant()
		assert.NotContains(t, renderPlant(t, &p),
			`fill="none" stroke="green"`)
		p.AuxiliaryBuilding.DieselGenerator.State = core.DieselRunning
		assert.Contains(t, renderPlant(t, &p),
			`fill="none" stroke="green"`)
	})

	t.Run("opc alarm ambers turbine", func(t *testing.T) {
		p := core.MakePlant()
		assert.Empty(t, strings.Count(renderPlant(t, &p),
			`stroke="#f0a500"`))
		p.TurbineBuilding.Turbine.OPCAlarm = true
		assert.Contains(t, renderPlant(t, &p), `stroke="#f0a500"`)
	})

	t.Run("reactor trip reds core parts", func(t *testing.T) {
		p := core.MakePlant()
		assert.Empty(t, strings.Count(renderPlant(t, &p),
			`stroke="#e02020"`))
		p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
		assert.Positive(t, strings.Count(renderPlant(t, &p),
			`stroke="#e02020"`))
	})

	t.Run("halos flash until acknowledged", func(t *testing.T) {
		p := core.MakePlant()
		p.TurbineBuilding.Turbine.OPCAlarm = true
		out := renderPlant(t, &p)
		assert.Contains(t, out, `stroke="#f0a500"`)
		assert.Contains(t, out, `class="blink"`)

		p.ReactorBuilding.PlantControlRoom.AlarmSet = map[core.AlarmID]core.Alarm{
			core.AlarmIDOPCAlarm: {Active: true, Acknowledged: true},
		}
		out = renderPlant(t, &p)
		assert.Contains(t, out, `stroke="#f0a500"`,
			"acknowledged alarm must hold a steady halo")
		assert.NotContains(t, out, `class="blink"`)
	})

	t.Run("sg readout gates on volume", func(t *testing.T) {
		p := core.MakePlant()
		sg := &p.ReactorBuilding.SteamGenerator
		water, volume := sg.Water, sg.Volume
		sg.Water, sg.Volume = core.Volume{}, core.Volume{}
		assert.NotRegexp(t, `SG \d+%`, renderPlant(t, &p))
		sg.Water, sg.Volume = water, volume
		assert.Regexp(t, `SG \d+%`, renderPlant(t, &p))
	})

	t.Run("brs evaporator overlays gate on state", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		assert.NotContains(t, out, "dist divert")
		assert.NotContains(t, out, "brs-carryover")

		brs := &p.AuxiliaryBuilding.BoronRecycleSystem
		brs.FeedPumpRunning = true
		brs.FeedFlow = 10
		assert.Regexp(t, `10\.0 gpm`, renderPlant(t, &p),
			"feed flow reads while the panel latch is on")

		brs.Carryover = true
		assert.Contains(t, renderPlant(t, &p), `class="brs-carryover"`)

		brs.DistillateDiverted = true
		assert.Contains(t, renderPlant(t, &p), "dist divert")
	})

	t.Run("pzr pressure and level readouts", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		assert.Regexp(t, `PZR \d+ psig`, out)
		assert.Regexp(t, `lvl \d+%`, out)
	})

	t.Run("core operator numbers under the RPV", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		assert.Regexp(t, `PWR \d+%`, out)
		assert.NotRegexp(t, `rods \d+%.*rods`, out)

		rpv := &p.ReactorBuilding.ReactorPressureVessel
		rpv.ControlRods.Insertion = 0.42
		rpv.ControlRods.BoronConcentration = 1200
		out = renderPlant(t, &p)
		assert.Regexp(t, `rods 42%`, out)
		assert.Regexp(t, `B 1200 ppm`, out)
	})

	t.Run("generator demand tracks the dispatcher", func(t *testing.T) {
		p := core.MakePlant()
		p.TurbineBuilding.Generator.BreakerClosed = true
		p.ReactorBuilding.Grid.LoadDemand = 0.7
		assert.Regexp(t, `\d+ MW / dem 700 MW`, renderPlant(t, &p))
	})

	t.Run("relief-kind lifts tint red, throttling does not", func(t *testing.T) {
		p := core.MakePlant()
		base := strings.Count(renderPlant(t, &p), `fill="#e02020"`)

		p.ReactorBuilding.PORV.Open = 0.3
		lifted := strings.Count(renderPlant(t, &p), `fill="#e02020"`)
		assert.Greater(t, lifted, base,
			"a lifting PORV must read as relief, not throttle")

		p2 := core.MakePlant()
		p2.ReactorBuilding.SafetyValve.Open = 0.2
		assert.Greater(t,
			strings.Count(renderPlant(t, &p2), `fill="#e02020"`), base,
			"edge-carried safety valves lift red too")

		p3 := core.MakePlant()
		p3.TurbineBuilding.Turbine.GoverningValve.Open = 0.5
		assert.Equal(t, base,
			strings.Count(renderPlant(t, &p3), `fill="#e02020"`),
			"a modulating valve at partial travel is not a lift")
	})

	t.Run("part tooltips carry label and live values", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		assert.Regexp(t, `<title>PZR: PZR \d+ psig, lvl \d+%</title>`, out)
		assert.Regexp(t,
			`<title>RPV: T \d+°F, PWR \d+%, rods \d+%, B \d+ ppm</title>`, out)
		assert.Contains(t, out, `<title>Turbine</title>`,
			"parts without readouts still get a labelled tooltip")
	})

	t.Run("aria labels distinguish trains", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		assert.Contains(t, out, `aria-label="RHRS A"`)
		assert.Contains(t, out, `aria-label="RHRS B"`)
	})

	t.Run("building groups expose their cluster", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		for _, name := range []string{
			"cluster_containment", "cluster_turbine",
			"cluster_auxiliary", "cluster_electrical",
		} {
			assert.Contains(t, out, fmt.Sprintf(`<g data-cluster="%s">`, name))
		}
	})

	t.Run("electrical alignment dots", func(t *testing.T) {
		p := core.MakePlant()
		before := strings.Count(renderPlant(t, &p), `fill="green"`)
		p.Electrical.ESFBusA.Energized = true
		p.ReactorBuilding.Grid.Voltage = 24000
		assert.Greater(t,
			strings.Count(renderPlant(t, &p), `fill="green"`), before,
			"energized ESF bus and hot grid draw green run dots")
	})

	t.Run("generator mw readout when breaker closed", func(t *testing.T) {
		p := core.MakePlant()
		assert.NotRegexp(t, `\d+ MW<`, renderPlant(t, &p))
		p.TurbineBuilding.Generator.BreakerClosed = true
		assert.Regexp(t, `\d+(\.\d+)? MW<`, renderPlant(t, &p))
	})

	t.Run("standalone valves render live position", func(t *testing.T) {
		p := core.MakePlant()
		p.TurbineBuilding.Turbine.GoverningValve.Open = 0.5
		out := renderPlant(t, &p)
		assert.Contains(t, out,
			`data-part="TurbineBuilding/Turbine/GoverningValve"`)
		assert.Contains(t, out, "<title>50%</title>")
	})

	t.Run("off-graph pumps render with labels", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		for _, label := range []string{"CW Pump", "CCW Pump", "SFP PA", "SFP PB"} {
			assert.Contains(t, out, ">"+label+"<")
		}
	})

	t.Run("flow indication follows the valves", func(t *testing.T) {
		p := core.MakePlant()
		base := renderPlant(t, &p)

		deadBlack := func(doc string) int {
			n := 0
			for _, pl := range regexp.MustCompile(`<polyline [^>]*/>`).
				FindAllString(doc, -1) {
				if strings.Contains(pl, `class="idle"`) &&
					strings.Contains(pl, `stroke="black"`) {
					n++
				}
			}
			return n
		}
		baseDead := deadBlack(base)

		mfp := &p.TurbineBuilding.MainFeedwaterPump
		mfp.Running, mfp.Velocity = true, 1000
		fw := &p.TurbineBuilding.MainFeedwaterControlValve
		fw.Open = 1
		flowing := renderPlant(t, &p)
		assert.Greater(t, strings.Count(flowing, `class="flow"`),
			strings.Count(base, `class="flow"`),
			"a running pump with an open valve must animate its pipe")
		assert.Less(t, strings.Count(flowing, `class="idle"`),
			strings.Count(base, `class="idle"`))
		assert.Equal(t, baseDead-1, deadBlack(flowing),
			"the feedwater leg must lift out of black while passing water")

		// Shutting the regulator kills the dashes and blacks the
		// leg: with the pump running but the valve shut nothing is
		// moving, and that is now stated outright.
		fw.Open = 0
		shut := renderPlant(t, &p)
		assert.Equal(t, strings.Count(base, `class="flow"`),
			strings.Count(shut, `class="flow"`),
			"a closed inline valve must stop the dash animation")
		assert.Equal(t, baseDead, deadBlack(shut),
			"a running pump behind a shut valve must black its leg")

		// Passive legs (no pump, no valve) keep dimmed fluid colour:
		// the model cannot observe them, so they never go black.
		assert.Equal(t, baseDead, deadBlack(base))
	})

	t.Run("tank level gauges track inventory", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		require.Equal(t, len(levelAccessors),
			strings.Count(out, `class="level-gauge"`),
			"every tank-backed part renders exactly one gauge")

		// Clamping: an overfilled tank renders identically to an
		// exactly-full one.
		cst := &p.Outside.CondensateStorageTank
		cst.Level = cst.Capacity
		full := renderPlant(t, &p)
		cst.Level = core.Volume{Val: 1e9, Unit: u.Gallon}
		assert.Equal(t, full, renderPlant(t, &p))
	})
}

// TestViewportGroupWrapsContent ensures all diagram content sits
// inside g#viewport so embedders can pan/zoom without touching
// document markup.
func TestViewportGroupWrapsContent(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	open := strings.Index(out, `<g id="viewport">`)
	firstPart := strings.Index(out, `data-part=`)
	require.Positive(t, open)
	require.Positive(t, firstPart)
	assert.Less(t, open, firstPart)
	assert.True(t, strings.HasSuffix(strings.TrimRight(out, "\n"), "</g>\n</svg>"),
		"viewport group must close at end of document")
}

// TestLegendRemovalIsIntentional documents that the bottom legend row
// was removed at operator request; if a legend returns it should be
// reconsidered deliberately rather than resurrected by accident.
func TestLegendRemovalIsIntentional(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	assert.NotContains(t, out, `id="legend"`)
	assert.NotContains(t, out, `id="alarms"`)
}

var (
	partGroupTagRe = regexp.MustCompile(`<g [^>]*data-part="[^"]+"[^>]*>`)
)

// TestPartGroupsAreAccessible checks every bound part group is
// announced to assistive tech and tab-focusable, independent of
// attribute order (valve groups append a transform).
func TestPartGroupsAreAccessible(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	tags := partGroupTagRe.FindAllString(out, -1)
	require.NotEmpty(t, tags)
	labelled := 0
	for _, tag := range tags {
		if !assert.Contains(t, tag, `role="img"`) ||
			!assert.Regexp(t, `aria-label="[^"]+"`, tag) ||
			!assert.Contains(t, tag, `tabindex="0"`) {
			continue
		}
		labelled++
	}
	assert.Equal(t, len(tags), labelled,
		"every data-part group needs role, aria-label, and tabindex")
}

var (
	symbolIDRe = regexp.MustCompile(`<symbol id="([^"]+)"`)
	useRefRe   = regexp.MustCompile(`href="#([^"]+)"`)
)

// assertWellFormed walks the document with a strict XML decoder and
// resolves every use reference against emitted symbol ids.
func assertWellFormed(t *testing.T, out string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(out))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}
	defs := map[string]bool{}
	for _, m := range symbolIDRe.FindAllStringSubmatch(out, -1) {
		defs[m[1]] = true
	}
	for _, m := range useRefRe.FindAllStringSubmatch(out, -1) {
		require.True(t, defs[m[1]], "use ref #%s unresolved", m[1])
	}
	assert.NotContains(t, out, "NaN")
	// fmt renders infinities with a sign; bare "Inf" appears in
	// legitimate symbol titles such as "Infinite Bus".
	assert.NotContains(t, out, "+Inf")
	assert.NotContains(t, out, "-Inf")
}

// TestRender_RandomizedStatesStayWellFormed drives randomized valve
// opens and pump states through the renderer and asserts the
// document stays parseable with every reference resolved.
func TestRender_RandomizedStatesStayWellFormed(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for range 12 {
		p := core.MakePlant()
		g := p.BuildGraphFor(&p)
		for _, e := range g.Edges {
			if e.Valve != nil {
				e.Valve.Open = rng.Float64()
			}
			if e.Pump != nil {
				e.Pump.Running = rng.Intn(2) == 0
				e.Pump.Velocity = core.Velocity(rng.Float64() * 1200)
			}
		}
		assertWellFormed(t, renderPlant(t, &p))
	}
}
