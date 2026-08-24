package bridge

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

// ErrUnknownKind is returned by ConvertControlValue and FormatSensorValue
// when the Kind string is not recognised.
var ErrUnknownKind = errors.New("unknown bridge kind")

// BoolToStr returns "1" for true values and "0" for false values.
func BoolToStr(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// StrToBool interprets a string as a boolean. The accepted forms are "1",
// "true"/"True" for true, and "0", "false"/"False" for false. Any other
// value returns an error.
func StrToBool(s string) (bool, error) {
	if s == "1" || s == "true" || s == "True" {
		return true, nil
	}
	if s == "0" || s == "false" || s == "False" {
		return false, nil
	}
	return false, fmt.Errorf("invalid bool %q", s)
}

// ConvertControlValue parses and validates a raw string for the given Kind,
// returning the canonical string representation accepted by the corresponding
// SetControlFunc. The returned string is safe to pass directly to Set.
func ConvertControlValue(kind, raw string) (string, error) {
	switch kind {
	case "bool":
		if raw == "1" || raw == "true" || raw == "True" {
			return "1", nil
		}
		if raw == "0" || raw == "false" || raw == "False" {
			return "0", nil
		}
		return "", fmt.Errorf("invalid bool %q", raw)
	case "normalized":
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", err
		}
		if f < 0 || f > 1 {
			return "", fmt.Errorf("normalized %v out of range [0,1]", f)
		}
		return fmt.Sprintf("%v", f), nil
	case "percent":
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", err
		}
		if f < 0 || f > 100 {
			return "", fmt.Errorf("percent %v out of range [0,100]", f)
		}
		return fmt.Sprintf("%v", f), nil
	case "rpm", "psig", "fahrenheit", "gallon", "watt", "ppm", "volt", "hz", "gpm", "mw":
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", err
		}
		if f < 0 {
			return "", fmt.Errorf("%s %v negative", kind, f)
		}
		return fmt.Sprintf("%v", f), nil
	case "enum":
		return raw, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownKind, kind)
	}
}

// FormatSensorValue serialises a sensor reading to a string according to the
// Kind taxonomy. Accepted Go types: bool, float64, float32, and any named
// type whose underlying kind is float64 or float32 (e.g. core.Pressure).
// Other types return an error.
func FormatSensorValue(kind string, v any) (string, error) {
	switch kind {
	case "bool":
		b, ok := v.(bool)
		if !ok {
			return "", fmt.Errorf("bool kind requires bool value, got %T", v)
		}
		if b {
			return "1", nil
		}
		return "0", nil
	case "normalized", "percent", "signedpercent", "rpm", "psig", "fahrenheit", "gallon", "watt", "ppm", "volt", "hz", "gpm":
		f, err := toFloat(v)
		if err != nil {
			return "", err
		}
		// fahrenheit predates the taxonomy and signedpercent exists for
		// observables measured around zero (axial flux difference);
		// every other kind is a nonnegative physical magnitude.
		if kind != "fahrenheit" && kind != "signedpercent" && f < 0 {
			return "", fmt.Errorf("%s %v negative", kind, f)
		}
		return fmt.Sprintf("%v", f), nil
	case "enum":
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("enum kind requires string value, got %T", v)
		}
		return s, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownKind, kind)
	}
}

// toFloat extracts a float64 from either a concrete float64/float32 or any
// defined type whose underlying kind is one of those. This lets callers pass
// `core.Pressure` directly without having to call `.Float()` first.
func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float64:
		return rv.Float(), nil
	case reflect.Float32:
		return rv.Float(), nil
	}
	return 0, fmt.Errorf("requires float value, got %T", v)
}