package core

import (
	"time"

	u "github.com/bcicen/go-units"
)

type Pressure float64
type Velocity float64
type Temperature = u.Value
type Volume = u.Value
type Watts float64
type RPM float64

const TimeStep = time.Millisecond
