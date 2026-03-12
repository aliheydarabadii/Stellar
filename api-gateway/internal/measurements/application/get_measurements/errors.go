package getmeasurements

import (
	"errors"
	"fmt"
	"strings"
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

type downstreamInvalidRequestError struct {
	message string
}

type MeasurementServiceUnavailableError interface {
	error
	MeasurementServiceUnavailable() bool
}

type DownstreamInvalidRequestError interface {
	error
	DownstreamInvalidRequestMessage() string
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

func (e downstreamInvalidRequestError) DownstreamInvalidRequestMessage() string {
	return e.message
}

func (e downstreamInvalidRequestError) Is(target error) bool {
	return target == ErrDownstreamInvalidRequest
}

func mapMeasurementsReaderError(err error) error {
	if err == nil {
		return nil
	}

	var invalidRequest DownstreamInvalidRequestError
	if errors.As(err, &invalidRequest) {
		return NewDownstreamInvalidRequestError(invalidRequest.DownstreamInvalidRequestMessage())
	}

	var unavailable MeasurementServiceUnavailableError
	if errors.As(err, &unavailable) && unavailable.MeasurementServiceUnavailable() {
		return fmt.Errorf("%w: %v", ErrMeasurementServiceUnavailable, err)
	}

	return err
}
