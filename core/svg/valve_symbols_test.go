package svg

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// SVG <symbol> establishes a viewport: any geometry outside the
// viewBox is silently clipped when the symbol is instanced through
// <use> (plans/SKILL-clipping.md). This test pins every valve
// glyph inside its box; it would have caught valve-modulating
// drawing its actuator arrow two units above the box top and
// valve-safety resting its spring on the boundary.
//
// The geometry reader understands the element set this asset
// family uses - polygon/polyline point lists, line, rect, circle,
// text anchors, and absolute-movement path data. Arcs or relative
// path commands require extending the reader before the invariant
// stays honest for new glyphs.
//
// Containment is enforced strictly (boundary counts as outside):
// a shape lying exactly on the viewBox edge clips half its stroke.

var (
	valveSymRe   = regexp.MustCompile(`(?s)<symbol id="(valve-[\w-]+)"([^>]*)>(.*?)</symbol>`)
	viewBoxRe    = regexp.MustCompile(`viewBox="([^"]+)"`)
	elementRe    = regexp.MustCompile(`<(polygon|polyline|line|rect|circle|text|path)\b[^>]*>?`)
	pointsAttrRe = regexp.MustCompile(`points="([^"]+)"`)
	pathDAttrRe  = regexp.MustCompile(`\bd="([^"]+)"`)
	numberRe     = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
)

func attrFloat(el string, name string) (float64, bool) {
	m := regexp.MustCompile(name + `="(-?\d+(?:\.\d+)?)"`).FindStringSubmatch(el)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	return v, err == nil
}

// glyphPoints reduces one drawable element to the extreme points
// that must lie inside the viewBox.
func glyphPoints(tag string, el string) [][2]float64 {
	var pts [][2]float64
	add := func(x, y float64) { pts = append(pts, [2]float64{x, y}) }
	if tag == "polygon" || tag == "polyline" {
		if m := pointsAttrRe.FindStringSubmatch(el); m != nil {
			n := numberRe.FindAllString(m[1], -1)
			for i := 0; i+1 < len(n); i += 2 {
				x, _ := strconv.ParseFloat(n[i], 64)
				y, _ := strconv.ParseFloat(n[i+1], 64)
				add(x, y)
			}
		}
		return pts
	}
	if tag == "path" {
		if m := pathDAttrRe.FindStringSubmatch(el); m != nil {
			n := numberRe.FindAllString(m[1], -1)
			for i := 0; i+1 < len(n); i += 2 {
				x, _ := strconv.ParseFloat(n[i], 64)
				y, _ := strconv.ParseFloat(n[i+1], 64)
				add(x, y)
			}
		}
		return pts
	}
	if x, ok := attrFloat(el, `\bx1`); ok {
		y1, _ := attrFloat(el, `\by1`)
		x2, _ := attrFloat(el, `\bx2`)
		y2, _ := attrFloat(el, `\by2`)
		add(x, y1)
		add(x2, y2)
		return pts
	}
	if cx, ok := attrFloat(el, `\bcx`); ok {
		cy, _ := attrFloat(el, `\bcy`)
		r, _ := attrFloat(el, `\br`)
		add(cx - r, cy)
		add(cx + r, cy)
		add(cx, cy - r)
		add(cx, cy + r)
		return pts
	}
	if x, ok := attrFloat(el, `\bx`); ok {
		y, _ := attrFloat(el, `\by`)
		if w, okW := attrFloat(el, `\bwidth`); okW {
			h, _ := attrFloat(el, `\bheight`)
			add(x, y)
			add(x+w, y+h)
		} else {
			add(x, y)
		}
	}
	return pts
}

func TestValveSymbolGeometryInsideViewBox(t *testing.T) {
	matches := valveSymRe.FindAllStringSubmatch(ValveAsset, -1)
	assert.NotEmpty(t, matches, "valve asset parsed")
	const eps = 1e-6
	for _, sm := range matches {
		id := sm[1]
		vb := numberRe.FindAllString(viewBoxRe.FindStringSubmatch(sm[2])[1], -1)
		if len(vb) != 4 {
			t.Fatalf("%s: unparsable viewBox %v", id, vb)
		}
		minX, _ := strconv.ParseFloat(vb[0], 64)
		minY, _ := strconv.ParseFloat(vb[1], 64)
		w, _ := strconv.ParseFloat(vb[2], 64)
		h, _ := strconv.ParseFloat(vb[3], 64)
		maxX, maxY := minX+w, minY+h

		for _, em := range elementRe.FindAllStringSubmatch(sm[3], -1) {
			for _, p := range glyphPoints(em[1], em[0]) {
				assert.GreaterOrEqualf(t, p[0], minX-eps,
					"%s: %s x=%g left of viewBox [%g,%g]", id, em[1], p[0], minX, maxX)
				assert.LessOrEqualf(t, p[0], maxX+eps,
					"%s: %s x=%g right of viewBox [%g,%g]", id, em[1], p[0], minX, maxX)
				assert.GreaterOrEqualf(t, p[1], minY-eps,
					"%s: %s y=%g above viewBox [%g,%g]", id, em[1], p[1], minY, maxY)
				assert.LessOrEqualf(t, p[1], maxY+eps,
					"%s: %s y=%g below viewBox [%g,%g]", id, em[1], p[1], minY, maxY)
			}
		}
	}
}
