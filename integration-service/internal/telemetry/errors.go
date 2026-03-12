package telemetry

import "errors"

var (
	ErrInvalidMeasurement         = errors.New("invalid measurement")
	ErrNegativeSetpoint           = errors.New("setpoint must not be negative")
	ErrNegativeActivePower        = errors.New("active power must not be negative")
	ErrActivePowerExceedsSetpoint = errors.New("active power must not exceed setpoint")
)
