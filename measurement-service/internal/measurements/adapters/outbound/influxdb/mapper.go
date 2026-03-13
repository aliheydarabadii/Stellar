package influxdb

import (
	"fmt"
	"sort"
	"stellar/internal/measurements"
	"time"
)

type exactTimestampBucket struct {
	setpoint       float64
	activePower    float64
	hasSetpoint    bool
	hasActivePower bool
}

type secondBucket struct {
	byTimestamp map[time.Time]*exactTimestampBucket
}

func mapRecordsToPoints(records recordIterator) ([]measurements.MeasurementPoint, error) {
	grouped := make(map[time.Time]*secondBucket)

	for records.Next() {
		record := records.Record()
		timestamp := record.Time.UTC()
		second := timestamp.Truncate(time.Second)

		bucket, ok := grouped[second]
		if !ok {
			bucket = &secondBucket{
				byTimestamp: make(map[time.Time]*exactTimestampBucket),
			}
			grouped[second] = bucket
		}

		pointBucket, ok := bucket.byTimestamp[timestamp]
		if !ok {
			pointBucket = &exactTimestampBucket{}
			bucket.byTimestamp[timestamp] = pointBucket
		}

		value, err := toFloat64(record.Value)
		if err != nil {
			return nil, err
		}

		switch record.Field {
		case "setpoint":
			pointBucket.setpoint = value
			pointBucket.hasSetpoint = true
		case "active_power":
			pointBucket.activePower = value
			pointBucket.hasActivePower = true
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

	points := make([]measurements.MeasurementPoint, 0, len(seconds))
	for _, second := range seconds {
		point, ok := latestCompletePointWithinSecond(second, grouped[second])
		if ok {
			points = append(points, point)
		}
	}

	return points, nil
}

func latestCompletePointWithinSecond(second time.Time, bucket *secondBucket) (measurements.MeasurementPoint, bool) {
	timestamps := make([]time.Time, 0, len(bucket.byTimestamp))
	for timestamp := range bucket.byTimestamp {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i].After(timestamps[j])
	})

	for _, timestamp := range timestamps {
		pointBucket := bucket.byTimestamp[timestamp]
		if !pointBucket.hasSetpoint || !pointBucket.hasActivePower {
			continue
		}

		return measurements.MeasurementPoint{
			Timestamp:   second,
			Setpoint:    pointBucket.setpoint,
			ActivePower: pointBucket.activePower,
		}, true
	}

	return measurements.MeasurementPoint{}, false
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
