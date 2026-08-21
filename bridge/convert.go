package bridge

import (
	"errors"
	"fmt"
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
	case "rpm", "psig", "fahrenheit", "gallon", "watt", "ppm", "volt", "hz":
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
// Kind taxonomy. Accepted Go types: bool, float64, float32, string. Other
// types return an error.
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
	case "normalized", "percent", "rpm", "psig", "fahrenheit", "gallon", "watt", "ppm", "volt", "hz":
		var f float64
		switch x := v.(type) {
		case float64:
			f = x
		case float32:
			f = float64(x)
		default:
			return "", fmt.Errorf("%s kind requires float value, got %T", kind, v)
		}
		if kind != "fahrenheit" && f < 0 {
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