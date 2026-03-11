package query

import "time"

type MeasurementPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Setpoint    float64   `json:"setpoint"`
	ActivePower float64   `json:"active_power"`
}

type MeasurementSeries struct {
	AssetID string             `json:"asset_id"`
	Points  []MeasurementPoint `json:"points"`
}
