package modbus

import (
	"context"
	"time"

	"stellar/internal/telemetry/app/command"
	"stellar/internal/telemetry/domain"
)

type Source struct {
	mapper  *AddressMapper
	decoder *Decoder
}

func NewSource(mapper *AddressMapper, decoder *Decoder) *Source {
	return &Source{
		mapper:  mapper,
		decoder: decoder,
	}
}

func (s *Source) Read(_ context.Context, collectedAt time.Time) (command.TelemetryReading, error) {
	_ = s.mapper
	_ = s.decoder
	_ = collectedAt

	// TODO: replace with real Modbus polling, decoding, and domain mapping.
	return command.TelemetryReading{
		AssetID:     domain.DefaultAssetID,
		Setpoint:    0,
		ActivePower: 0,
	}, nil
}
