package influxdb

import (
	"fmt"
	"sort"
	"time"

	"stellar/internal/measurements/app/query"
)

func mapRecordsToPoints(records recordIterator) ([]query.MeasurementPoint, error) {
	type secondBucket struct {
		timestamp      time.Time
		setpoint       float64
		activePower    float64
		hasSetpoint    bool
		hasActivePower bool
	}

	grouped := make(map[time.Time]*secondBucket)

	for records.Next() {
		record := records.Record()
		second := record.Time.UTC().Truncate(time.Second)

		bucket, ok := grouped[second]
		if !ok {
			bucket = &secondBucket{timestamp: second}
			grouped[second] = bucket
		}

		value, err := toFloat64(record.Value)
		if err != nil {
			return nil, err
		}

		switch record.Field {
		case "setpoint":
			bucket.setpoint = value
			bucket.hasSetpoint = true
		case "active_power":
			bucket.activePower = value
			bucket.hasActivePower = true
		default:
			return nil, fmt.Errorf("unexpected measurement field %q", record.Field)
		}
	}

	seconds := make([]time.Time, 0, len(grouped))
	for second := range grouped {
		seconds = append(seconds, second)
	}
	sort.Slice(seconds, func(i, j int) bool {
		return seconds[i].Before(seconds[j])
	})

	points := make([]query.MeasurementPoint, 0, len(seconds))
	for _, second := range seconds {
		bucket := grouped[second]
		if !bucket.hasSetpoint || !bucket.hasActivePower {
			// A response point requires both fields; missing values are not invented.
			continue
		}

		points = append(points, query.MeasurementPoint{
			Timestamp:   bucket.timestamp,
			Setpoint:    bucket.setpoint,
			ActivePower: bucket.activePower,
		})
	}

	return points, nil
}

func toFloat64(value any) (float64, error) {
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case uint:
		return float64(typed), nil
	case uint32:
		return float64(typed), nil
	case uint64:
		return float64(typed), nil
	default:
		return 0, fmt.Errorf("unsupported measurement value type %T", value)
	}
}
