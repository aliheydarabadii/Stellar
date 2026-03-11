package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type MeasurementTestSuite struct {
	suite.Suite
	collectedAt time.Time
}

func TestMeasurementTestSuite(t *testing.T) {
	suite.Run(t, new(MeasurementTestSuite))
}

func (s *MeasurementTestSuite) SetupTest() {
	s.collectedAt = time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
}

func (s *MeasurementTestSuite) TestNewMeasurement() {
	tests := []struct {
		name        string
		setpoint    float64
		activePower float64
		wantErr     error
	}{
		{
			name:        "negative setpoint rejected",
			setpoint:    -1,
			activePower: 0,
			wantErr:     ErrNegativeSetpoint,
		},
		{
			name:        "negative active power rejected",
			setpoint:    10,
			activePower: -1,
			wantErr:     ErrNegativeActivePower,
		},
		{
			name:        "active power greater than setpoint rejected",
			setpoint:    10,
			activePower: 11,
			wantErr:     ErrActivePowerExceedsSetpoint,
		},
		{
			name:        "valid measurement accepted",
			setpoint:    10,
			activePower: 8,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			measurement, err := NewMeasurement(DefaultAssetID, tt.setpoint, tt.activePower, s.collectedAt)
			if tt.wantErr == nil {
				s.Require().NoError(err)
				s.Equal(DefaultAssetID, measurement.AssetID)
				s.Equal(tt.setpoint, measurement.Setpoint)
				s.Equal(tt.activePower, measurement.ActivePower)
				s.True(measurement.CollectedAt.Equal(s.collectedAt))
				return
			}

			s.Require().Error(err)
			s.ErrorIs(err, ErrInvalidMeasurement)
			s.ErrorIs(err, tt.wantErr)
			s.True(errors.Is(err, tt.wantErr))
		})
	}
}
