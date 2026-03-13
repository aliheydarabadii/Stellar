package redis

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

func MeasurementsKey(assetID string, from, to time.Time) string {
	return fmt.Sprintf(
		"measurements:%s:%s:%s",
		url.QueryEscape(strings.TrimSpace(assetID)),
		from.UTC().Format(time.RFC3339Nano),
		to.UTC().Format(time.RFC3339Nano),
	)
}

func LatestMeasurementsKey(assetID string) string {
	return fmt.Sprintf(
		"measurements:latest:%s",
		url.QueryEscape(strings.TrimSpace(assetID)),
	)
}
