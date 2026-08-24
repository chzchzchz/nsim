package svg

import (
	"encoding/xml"
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

// stripLegend drops the docked legend row so overlay assertions
// count only mimic fragments, not legend swatches reusing the
// same colours.
func stripLegend(out string) string {
	if before, _, ok := strings.Cut(out, `<g id="legend">`); ok {
		return before
	}
	return out
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
		assert.NotContains(t, stripLegend(renderPlant(t, &p)),
			`fill="none" stroke="green"`)
		p.AuxiliaryBuilding.DieselGenerator.State = core.DieselRunning
		assert.Contains(t, renderPlant(t, &p),
			`fill="none" stroke="green"`)
	})

	t.Run("opc alarm ambers turbine", func(t *testing.T) {
		p := core.MakePlant()
		assert.Empty(t, strings.Count(stripLegend(renderPlant(t, &p)),
			`stroke="#f0a500"`))
		p.TurbineBuilding.Turbine.OPCAlarm = true
		assert.Contains(t, renderPlant(t, &p), `stroke="#f0a500"`)
	})

	t.Run("reactor trip reds core parts", func(t *testing.T) {
		p := core.MakePlant()
		assert.Empty(t, strings.Count(stripLegend(renderPlant(t, &p)),
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

	t.Run("pzr pressure and level readouts", func(t *testing.T) {
		p := core.MakePlant()
		out := renderPlant(t, &p)
		assert.Regexp(t, `PZR \d+ psig`, out)
		assert.Regexp(t, `lvl \d+%`, out)
	})

	t.Run("generator mw readout when breaker closed", func(t *testing.T) {
		p := core.MakePlant()
		assert.NotRegexp(t, `\d+ MW<`, renderPlant(t, &p))
		p.TurbineBuilding.Generator.BreakerClosed = true
		assert.Regexp(t, `\d+(\.\d+)? MW<`, renderPlant(t, &p))
	})

	t.Run("alarm panel appears only with alarms", func(t *testing.T) {
		p := core.MakePlant()
		assert.NotContains(t, renderPlant(t, &p), `<g id="alarms">`)

		p.ReactorBuilding.ReactorProtectionSystem.ReactorTripped = true
		p.TurbineBuilding.Turbine.OPCAlarm = true
		out := renderPlant(t, &p)
		require.Contains(t, out, `<g id="alarms">`)
		assert.Contains(t, out, "rps.reactor_trip")
		// Trip severity sorts above the OPC info alarm.
		tripIdx := strings.Index(out, "[TRIP] rps.reactor_trip")
		infoIdx := strings.Index(out, "[INFO] turbine.opc_alarm")
		require.Positive(t, tripIdx)
		require.Positive(t, infoIdx)
		assert.Less(t, tripIdx, infoIdx)
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

// TestLegendMirrorsSharedTables verifies the docked legend is
// generated from pipeColours and the status constants rather than
// its own copy of the semantics.
func TestLegendMirrorsSharedTables(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	require.Contains(t, out, `<g id="legend">`)
	for _, c := range pipeColours {
		assert.Contains(t, out, ">"+pipeLegendLabels[c]+"<")
	}
	assert.Contains(t, out, ">running<")
	assert.Contains(t, out, ">standby<")
	assert.Contains(t, out, ">warning<")
	assert.Contains(t, out, ">trip<")
}

var (
	partGroupRe    = regexp.MustCompile(`<g data-part="[^"]+" role="img" aria-label="[^"]+" tabindex="0">`)
	dataPartCountR = regexp.MustCompile(`<g data-part="`)
)

// TestPartGroupsAreAccessible checks every bound part group is
// announced to assistive tech and tab-focusable.
func TestPartGroupsAreAccessible(t *testing.T) {
	p := core.MakePlant()
	out := renderPlant(t, &p)
	total := len(dataPartCountR.FindAllString(out, -1))
	labelled := len(partGroupRe.FindAllString(out, -1))
	require.Positive(t, total)
	assert.Equal(t, total, labelled,
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
