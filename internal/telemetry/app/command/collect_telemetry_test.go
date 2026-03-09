package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"stellar/internal/telemetry/domain"
)

func TestCollectTelemetryHandlerHandle(t *testing.T) {
	t.Parallel()

	collectedAt := time.Date(2026, time.March, 9, 12, 0, 0, 0, time.UTC)
	sourceErr := errors.New("source unavailable")
	repositoryErr := errors.New("repository unavailable")

	tests := []struct {
		name                 string
		reading              TelemetryReading
		sourceErr            error
		repositoryErr        error
		wantErr              error
		wantDomainErr        error
		wantSavedCount       int
		wantSavedMeasurement domain.Measurement
	}{
		{
			name: "valid reading gets saved",
			reading: TelemetryReading{
				AssetID:     domain.DefaultAssetID,
				Setpoint:    100,
				ActivePower: 80,
			},
			wantSavedCount: 1,
			wantSavedMeasurement: domain.Measurement{
				AssetID:     domain.DefaultAssetID,
				Setpoint:    100,
				ActivePower: 80,
				CollectedAt: collectedAt,
			},
		},
		{
			name: "invalid reading does not get saved",
			reading: TelemetryReading{
				AssetID:     domain.DefaultAssetID,
				Setpoint:    10,
				ActivePower: 20,
			},
			wantErr:        ErrInvalidTelemetry,
			wantDomainErr:  domain.ErrInvalidMeasurement,
			wantSavedCount: 0,
		},
		{
			name:           "source error is returned",
			sourceErr:      sourceErr,
			wantErr:        sourceErr,
			wantSavedCount: 0,
		},
		{
			name: "repository error is returned",
			reading: TelemetryReading{
				AssetID:     domain.DefaultAssetID,
				Setpoint:    100,
				ActivePower: 80,
			},
			repositoryErr:  repositoryErr,
			wantErr:        repositoryErr,
			wantSavedCount: 1,
			wantSavedMeasurement: domain.Measurement{
				AssetID:     domain.DefaultAssetID,
				Setpoint:    100,
				ActivePower: 80,
				CollectedAt: collectedAt,
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := &stubTelemetrySource{
				reading: tt.reading,
				err:     tt.sourceErr,
			}
			repository := &stubMeasurementRepository{
				err: tt.repositoryErr,
			}

			handler := NewCollectTelemetryHandler(source, repository)
			err := handler.Handle(context.Background(), CollectTelemetry{CollectedAt: collectedAt})

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}

				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
			}

			if tt.wantDomainErr != nil && !errors.Is(err, tt.wantDomainErr) {
				t.Fatalf("expected domain error %v, got %v", tt.wantDomainErr, err)
			}

			if source.calledWith.IsZero() || !source.calledWith.Equal(collectedAt) {
				t.Fatalf("expected source to be called with %v, got %v", collectedAt, source.calledWith)
			}

			if len(repository.saved) != tt.wantSavedCount {
				t.Fatalf("expected %d saved measurements, got %d", tt.wantSavedCount, len(repository.saved))
			}

			if tt.wantSavedCount == 1 {
				if repository.saved[0] != tt.wantSavedMeasurement {
					t.Fatalf("expected saved measurement %+v, got %+v", tt.wantSavedMeasurement, repository.saved[0])
				}
			}
		})
	}
}

type stubTelemetrySource struct {
	reading    TelemetryReading
	err        error
	calledWith time.Time
}

func (s *stubTelemetrySource) Read(_ context.Context, collectedAt time.Time) (TelemetryReading, error) {
	s.calledWith = collectedAt
	if s.err != nil {
		return TelemetryReading{}, s.err
	}

	return s.reading, nil
}

type stubMeasurementRepository struct {
	saved []domain.Measurement
	err   error
}

func (r *stubMeasurementRepository) Save(_ context.Context, measurement domain.Measurement) error {
	r.saved = append(r.saved, measurement)
	if r.err != nil {
		return r.err
	}

	return nil
}
