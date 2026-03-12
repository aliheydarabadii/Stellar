package getmeasurements

import "api_gateway/internal/measurements/domain"

type CacheStatus string

const (
	CacheStatusHit    CacheStatus = "hit"
	CacheStatusMiss   CacheStatus = "miss"
	CacheStatusBypass CacheStatus = "bypass"
)

type Result struct {
	Series      domain.MeasurementSeries
	CacheStatus CacheStatus
}
