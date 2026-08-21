package bridge

type GetSensorFunc func() (string, error)

type SensorMap map[string]*Sensor

type Sensor struct {
	Part string
	Label string
	Kind string
	Get GetSensorFunc
}
