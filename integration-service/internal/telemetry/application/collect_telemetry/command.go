package collecttelemetry

import "time"

type CollectTelemetry struct {
	CollectedAt time.Time
}

type TelemetryReading struct {
	Setpoint    float64
	ActivePower float64
}
