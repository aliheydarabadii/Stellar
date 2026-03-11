package domain

import (
	"errors"
	"time"
)

type Measurement struct {
	AssetID     AssetID
	Setpoint    float64
	ActivePower float64
	CollectedAt time.Time
}

func NewMeasurement(assetID AssetID, setpoint, activePower float64, collectedAt time.Time) (Measurement, error) {
	measurement := Measurement{
		AssetID:     assetID,
		Setpoint:    setpoint,
		ActivePower: activePower,
		CollectedAt: collectedAt,
	}

	if err := ValidateMeasurement(measurement); err != nil {
		return Measurement{}, err
	}

	return measurement, nil
}

func ValidateMeasurement(measurement Measurement) error {
	if measurement.Setpoint < 0 {
		return errors.Join(ErrInvalidMeasurement, ErrNegativeSetpoint)
	}

	if measurement.ActivePower < 0 {
		return errors.Join(ErrInvalidMeasurement, ErrNegativeActivePower)
	}

	if measurement.ActivePower > measurement.Setpoint {
		return errors.Join(ErrInvalidMeasurement, ErrActivePowerExceedsSetpoint)
	}

	return nil
}
