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
	if *svgPath != "" {
		f, err := os.Create(*svgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to create svg file:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := svg.WritePlantSVG(f, &p); err != nil {
			fmt.Fprintln(os.Stderr, "failed to export svg:", err)
			os.Exit(1)
		}
		return
	}
	dump(reflect.ValueOf(p), 0)
}