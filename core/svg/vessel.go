package svg

// Vessel symbols: reactor pressure vessel, pressurizer,
// steam generator, condenser. Each symbol exposes named anchor
// circles so the composition pass can route pipes between them.
//
// Spec justification:
//   - Reactor vessel geometry: "cylindrical with a hemispherical
//     bottom head and a flanged and gasketed removable upper
//     head."
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_045.txt:35-39
//   - Pressurizer (cylindrical, immersion heaters, spray nozzle,
//     safety + relief valves):
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_068.txt:10-21
//   - Pressurizer principal design data (78 heaters, 1800 kW,
//     2 power-operated relief, 3 self-actuating safety):
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_069.txt:24-148
//   - Steam generator (vertical U-tube + integral steam drum):
//     specs/the_westinghouse_pressurized_water_react/chapter_3/page_064.txt:39-48
//   - Condenser (single-pass deaerating, straight-through tubes):
//     specs/the_westinghouse_pressurized_water_react/chapter_15/page_216.txt:13-22

// ReactorVesselSymbol is the SVG markup for the reactor
// pressure vessel. The symbol viewBox spans -50..50 horizontal
// and -120..120 vertical.
const ReactorVesselSymbol = `<symbol id="reactor_vessel" viewBox="-50 -120 100 240">` +
	`<title>Reactor Pressure Vessel</title>` +
	// hemispherical bottom head
	`<path d="M -40 60 Q -40 110 0 110 Q 40 110 40 60" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	// cylindrical shell
	`<rect x="-40" y="-80" width="80" height="140" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	// flanged removable upper head
	`<rect x="-44" y="-92" width="88" height="12" ` +
	`fill="#cccccc" stroke="black" stroke-width="1.5"/>` +
	`<ellipse cx="0" cy="-92" rx="40" ry="6" ` +
	`fill="#bdbdbd" stroke="black" stroke-width="1"/>` +
	// CRDM stubs (4)
	`<g stroke="black" stroke-width="1">` +
	`<rect x="-30" y="-110" width="6" height="18"/>` +
	`<rect x="-12" y="-110" width="6" height="18"/>` +
	`<rect x="6" y="-110" width="6" height="18"/>` +
	`<rect x="24" y="-110" width="6" height="18"/>` +
	`</g>` +
	// hot-leg nozzle (right)
	`<rect x="40" y="0" width="20" height="14" ` +
	`fill="#cccccc" stroke="black"/>` +
	// cold-leg nozzle (left)
	`<rect x="-60" y="0" width="20" height="14" ` +
	`fill="#cccccc" stroke="black"/>` +
	// anchors
	`<circle id="rv-anchor-surge" cx="0" cy="-110" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="rv-anchor-hotleg" cx="60" cy="7" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="rv-anchor-coldleg" cx="-60" cy="7" r="2" ` +
	`fill="none" stroke="none"/>` +
	`</symbol>`

// PressurizerSymbol is the SVG markup for the pressurizer.
// Sized 7 ft 8 in OD x 52 ft 9 in tall (4-loop plant).
const PressurizerSymbol = `<symbol id="pressurizer" viewBox="-30 -160 60 320">` +
	`<title>Pressurizer</title>` +
	// hemispherical top + bottom heads
	`<path d="M -22 -150 Q -22 -160 0 -160 Q 22 -160 22 -150" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	`<path d="M -22 150 Q -22 160 0 160 Q 22 160 22 150" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	// shell
	`<rect x="-22" y="-150" width="44" height="300" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	// 78 heater stubs (schematic: 6 rows of 13)
	`<g stroke="black" stroke-width="0.5">` +
	`<line x1="-18" y1="140" x2="-18" y2="148"/>` +
	`<line x1="-14" y1="140" x2="-14" y2="148"/>` +
	`<line x1="-10" y1="140" x2="-10" y2="148"/>` +
	`<line x1="-6" y1="140" x2="-6" y2="148"/>` +
	`<line x1="-2" y1="140" x2="-2" y2="148"/>` +
	`<line x1="2" y1="140" x2="2" y2="148"/>` +
	`<line x1="6" y1="140" x2="6" y2="148"/>` +
	`<line x1="10" y1="140" x2="10" y2="148"/>` +
	`<line x1="14" y1="140" x2="14" y2="148"/>` +
	`<line x1="18" y1="140" x2="18" y2="148"/>` +
	`</g>` +
	// surge line anchor (bottom)
	`<circle id="pzr-anchor-surge" cx="0" cy="160" r="2" ` +
	`fill="none" stroke="none"/>` +
	`</symbol>`

// SteamGeneratorSymbol is the SVG markup for the steam
// generator (vertical U-tube + integral steam drum).
const SteamGeneratorSymbol = `<symbol id="steam_generator" viewBox="-95 -215 190 430">` +
	`<title>Steam Generator (Model F, vertical U-tube)</title>` +
	// channel head (lower)
	`<path d="M -65 110 Q -65 140 -45 140 L 45 140 Q 65 140 65 110 Z" ` +
	`fill="#dcdcdc" stroke="black" stroke-width="1.5"/>` +
	// primary partition plate
	`<line x1="0" y1="110" x2="0" y2="140" ` +
	`stroke="black" stroke-width="1"/>` +
	// lower shell (smaller OD)
	`<rect x="-45" y="-100" width="90" height="210" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	// upper shell (wider OD, the steam drum)
	`<path d="M -65 -100 L -65 -180 Q -65 -195 -50 -195 ` +
	`L 50 -195 Q 65 -195 65 -180 L 65 -100 Z" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	// U-tube bundle (cross-hatch)
	`<g stroke="#888" stroke-width="0.3">` +
	`<line x1="-40" y1="-80" x2="-40" y2="100"/>` +
	`<line x1="-20" y1="-80" x2="-20" y2="100"/>` +
	`<line x1="0" y1="-80" x2="0" y2="100"/>` +
	`<line x1="20" y1="-80" x2="20" y2="100"/>` +
	`<line x1="40" y1="-80" x2="40" y2="100"/>` +
	`</g>` +
	// moisture-separator chevrons in the steam drum
	`<g stroke="black" stroke-width="0.6" fill="none">` +
	`<path d="M -50 -160 L -45 -150 L -50 -140"/>` +
	`<path d="M -35 -160 L -30 -150 L -35 -140"/>` +
	`<path d="M -20 -160 L -15 -150 L -20 -140"/>` +
	`<path d="M -5 -160 L 0 -150 L -5 -140"/>` +
	`<path d="M 10 -160 L 15 -150 L 10 -140"/>` +
	`<path d="M 25 -160 L 30 -150 L 25 -140"/>` +
	`<path d="M 40 -160 L 45 -150 L 40 -140"/>` +
	`</g>` +
	// feedwater nozzle (top side)
	`<rect x="-15" y="-205" width="30" height="10" ` +
	`fill="#cccccc" stroke="black"/>` +
	// steam outlet nozzle (top)
	`<rect x="-8" y="-210" width="16" height="14" ` +
	`fill="#cccccc" stroke="black"/>` +
	// primary inlet (left of channel head)
	`<rect x="-90" y="115" width="25" height="12" ` +
	`fill="#cccccc" stroke="black"/>` +
	// primary outlet (right of channel head)
	`<rect x="65" y="115" width="25" height="12" ` +
	`fill="#cccccc" stroke="black"/>` +
	// anchors
	`<circle id="sg-anchor-steam" cx="0" cy="-210" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="sg-anchor-feedwater" cx="0" cy="-205" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="sg-anchor-primary-in" cx="-90" cy="121" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="sg-anchor-primary-out" cx="90" cy="121" r="2" ` +
	`fill="none" stroke="none"/>` +
	`</symbol>`

// CondenserSymbol is the SVG markup for the main condenser.
// Single-pass deaerating type with straight-through tubes.
const CondenserSymbol = `<symbol id="condenser" viewBox="-200 -50 400 100">` +
	`<title>Main Condenser (single-pass deaerating)</title>` +
	// horizontal shell
	`<rect x="-160" y="-30" width="320" height="60" ` +
	`fill="#e6e6e6" stroke="black" stroke-width="1.5"/>` +
	// end waterboxes (CW inlet / outlet)
	`<rect x="-180" y="-40" width="20" height="80" ` +
	`fill="#cccccc" stroke="black"/>` +
	`<rect x="160" y="-40" width="20" height="80" ` +
	`fill="#cccccc" stroke="black"/>` +
	// steam inlet from turbine (top centre)
	`<rect x="-12" y="-50" width="24" height="20" ` +
	`fill="#cccccc" stroke="black"/>` +
	// hotwell drain (bottom centre)
	`<rect x="-6" y="30" width="12" height="20" ` +
	`fill="#cccccc" stroke="black"/>` +
	// anchors
	`<circle id="cond-anchor-steam-in" cx="0" cy="-50" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="cond-anchor-hotwell" cx="0" cy="50" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="cond-anchor-cw-in" cx="-180" cy="0" r="2" ` +
	`fill="none" stroke="none"/>` +
	`<circle id="cond-anchor-cw-out" cx="180" cy="0" r="2" ` +
	`fill="none" stroke="none"/>` +
	`</symbol>`
