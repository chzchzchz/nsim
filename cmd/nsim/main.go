package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/chzchzchz/nsim/core"
	"github.com/chzchzchz/nsim/core/svg"
)

func dump(v reflect.Value, depth int) {
	t := v.Type()
	sp := strings.Repeat(" ", depth)
	to_str := func(i any) string { return fmt.Sprintf("%+v", i) }
	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			if !sf.IsExported() {
				continue
			}
			vv := v.Field(i)
			isStruct := sf.Type.Kind() == reflect.Struct
			isUnits := sf.Type.String() == "units.Value"
			if !isStruct || isUnits {
				fmt.Println(sp, sf.Name, vv)
				continue
			}
			fmt.Println(sp, to_str(sf))
			dump(vv, depth+1)
		}
	}
}

func main() {
	dotPath := flag.String("dot", "", "write plant graph in DOT format to file")
	svgPath := flag.String("svg", "", "write plant P&ID SVG to file")
	svgDotPath := flag.String("svg-dot", "", "write graphviz DOT for SVG layout to file and exit")
	svgLayoutPath := flag.String("svg-layout", "", "compute graphviz layout and write dot.svg to file and exit")
	svgDebugPath := flag.String("svg-debug", "", "compute layout, write debug overlay with bbox+anchors to file and exit")
	svgAssetsPath := flag.String("svg-assets", "", "export symbol SVG files to a directory and exit")
	flag.Parse()

	p := core.MakePlant()
	if *dotPath != "" {
		f, err := os.Create(*dotPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to create dot file:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := p.ExportDot(f); err != nil {
			fmt.Fprintln(os.Stderr, "failed to export dot:", err)
			os.Exit(1)
		}
		return
	}
	if *svgDebugPath != "" {
		g := p.BuildGraphFor(&p)
		layout, err := svg.ComputeLayout(&p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to compute layout: %v\n", err)
			os.Exit(1)
		}
		pos := make(map[string]svg.Anchor)
		for id, a := range layout.Pos {
			pos[id] = a
		}
		f, err := os.Create(*svgDebugPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to create svg-debug file:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := svg.DebugLayout(f, g, pos); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write debug layout: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *svgAssetsPath != "" {
		if err := svg.WriteAssetFiles(*svgAssetsPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write svg assets: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *svgDotPath != "" {
		g := p.BuildGraphFor(&p)
		dot, err := svg.BuildDot(g, &p, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to build svg-dot: %v\n", err)
			os.Exit(1)
		}
		f, err := os.Create(*svgDotPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to create svg-dot file:", err)
			os.Exit(1)
		}
		defer f.Close()
		if _, err := f.WriteString(dot); err != nil {
			fmt.Fprintln(os.Stderr, "failed to write svg-dot:", err)
			os.Exit(1)
		}
		return
	}
	// Shared helper: compute layout so the SVG writers render with
	// graphviz positions instead of the embedded default.
	computeLayout := func() (*svg.Layout, error) {
		layout, err := svg.ComputeLayout(&p)
		if err != nil {
			return nil, fmt.Errorf("compute layout: %w", err)
		}
		return layout, nil
	}

	if *svgLayoutPath != "" {
		layout, err := computeLayout()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to compute layout: %v\n", err)
			os.Exit(1)
		}
		f, err := os.Create(*svgLayoutPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to create svg-layout file:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := svg.WritePlantSVGWith(f, &p, layout); err != nil {
			fmt.Fprintln(os.Stderr, "failed to export svg-layout:", err)
			os.Exit(1)
		}
		return
	}
	if *svgPath != "" {
		layout, err := computeLayout()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to compute layout: %v\n", err)
			os.Exit(1)
		}
		f, err := os.Create(*svgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to create svg file:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := svg.WritePlantSVGWith(f, &p, layout); err != nil {
			fmt.Fprintln(os.Stderr, "failed to export svg:", err)
			os.Exit(1)
		}
		return
	}
	dump(reflect.ValueOf(p), 0)
}