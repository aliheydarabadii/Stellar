package query

import "time"

type MeasurementPoint struct {
	Timestamp   time.Time
	Setpoint    float64
	ActivePower float64
}

type MeasurementSeries struct {
	AssetID string
	Points  []MeasurementPoint
}
