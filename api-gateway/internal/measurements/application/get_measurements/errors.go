package getmeasurements

import (
	"errors"
	"strings"
)

var (
	ErrAssetIDRequired               = errors.New("asset id is required")
	ErrInvalidTimeRange              = errors.New("from must not be after to")
	ErrTimestampZero                 = errors.New("from and to must be set")
	ErrMeasurementsClientRequired    = errors.New("measurements client is required")
	ErrMeasurementsCacheRequired     = errors.New("measurements cache is required")
	ErrCacheKeyBuilderRequired       = errors.New("cache key builder is required")
	ErrCacheTTLInvalid               = errors.New("cache ttl must be positive")
	ErrMeasurementServiceUnavailable = errors.New("measurement service unavailable")
	ErrDownstreamInvalidRequest      = errors.New("measurement service rejected request")
)

type downstreamInvalidRequestError struct {
	message string
}

func NewDownstreamInvalidRequestError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return ErrDownstreamInvalidRequest
	}

	return downstreamInvalidRequestError{message: message}
}

func (e downstreamInvalidRequestError) Error() string {
	return e.message
}

func (e downstreamInvalidRequestError) Is(target error) bool {
	return target == ErrDownstreamInvalidRequest
}
