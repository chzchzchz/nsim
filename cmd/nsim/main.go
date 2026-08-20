package main

import (
	"reflect"
	"strings"
	"fmt"

	"github.com/chzchzchz/nsim/core"
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

/*
The safety-related diesel- and turbine-driven auxiliary feedwater pumps are identical
six-stage, horizontal, centrifugal pumps designed for 2000 psig, 70°F water. They
operate with a design flow rate of 960 gpm, including 80 gpm recirculation, at a 3400
ft (1472 psid) head. The rated speed of each pump is 4560 RPM.
*/

func main() {
	p := core.MakePlant()
	dump(reflect.ValueOf(p), 0)
	/*
	for {
		p.Step()
		p.Update()
	}*/
}