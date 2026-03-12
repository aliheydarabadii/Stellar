package application

import (
	"errors"
	"fmt"
	"strings"

	"api_gateway/internal/measurements"
)

var (
	ErrAssetIDRequired               = errors.New("asset id is required")
	ErrInvalidTimeRange              = errors.New("from must not be after to")
	ErrTimestampZero                 = errors.New("from and to must be set")
	ErrMeasurementsReaderRequired    = errors.New("measurements reader is required")
	ErrMeasurementsCacheRequired     = errors.New("measurements cache is required")
	ErrCacheKeyBuilderRequired       = errors.New("cache key builder is required")
	ErrCacheTTLInvalid               = errors.New("cache ttl must be positive")
	ErrMeasurementServiceUnavailable = errors.New("measurement service unavailable")
	ErrDownstreamInvalidRequest      = errors.New("measurement service rejected request")
)

func mapMeasurementsReaderError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, measurements.ErrMeasurementsReaderInvalidRequest) {
		message := strings.TrimSpace(strings.TrimPrefix(err.Error(), measurements.ErrMeasurementsReaderInvalidRequest.Error()))
		message = strings.TrimSpace(strings.TrimPrefix(message, ":"))
		if message == "" {
			return ErrDownstreamInvalidRequest
		}

		return fmt.Errorf("%w: %s", ErrDownstreamInvalidRequest, message)
	}

	if errors.Is(err, measurements.ErrMeasurementsReaderUnavailable) {
		return fmt.Errorf("%w: %v", ErrMeasurementServiceUnavailable, err)
	}

	return err
}
