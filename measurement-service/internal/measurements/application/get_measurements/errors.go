package getmeasurements

import (
	"errors"
	"time"
)

var (
	ErrAssetIDRequired      = errors.New("asset id is required")
	ErrInvalidTimeRange     = errors.New("from must not be after to")
	ErrQueryRangeTooLarge   = errors.New("query time range exceeds maximum allowed window")
	ErrTimestampZero        = errors.New("from and to must be set")
	ErrReadModelUnavailable = errors.New("measurements read model unavailable")
)

const DefaultMaxQueryRange = 15 * time.Minute
