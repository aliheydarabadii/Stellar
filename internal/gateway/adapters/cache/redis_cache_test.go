package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"api_gateway/internal/gateway/app/query"
)

func TestRedisCacheSetAndGet(t *testing.T) {
	t.Parallel()

	redisServer := miniredis.RunT(t)
	cache := newTestRedisCache(t, redisServer)

	series := query.MeasurementSeries{
		AssetID: "asset-1",
		Points: []query.MeasurementPoint{
			{
				Timestamp:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
				Setpoint:    100,
				ActivePower: 55,
			},
		},
	}

	key := MeasurementsKey("asset-1", series.Points[0].Timestamp, series.Points[0].Timestamp.Add(time.Minute))
	if err := cache.Set(context.Background(), key, series, 5*time.Minute); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	got, found, err := cache.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !found {
		t.Fatal("expected cache hit")
	}
	if got.AssetID != series.AssetID || len(got.Points) != 1 {
		t.Fatalf("expected stored series, got %+v", got)
	}
}

func TestRedisCacheExpiredEntriesReturnMiss(t *testing.T) {
	t.Parallel()

	redisServer := miniredis.RunT(t)
	cache := newTestRedisCache(t, redisServer)

	key := MeasurementsKey("asset-1", time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), time.Date(2026, 3, 10, 12, 5, 0, 0, time.UTC))
	if err := cache.Set(context.Background(), key, query.MeasurementSeries{AssetID: "asset-1"}, time.Second); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	redisServer.FastForward(2 * time.Second)

	_, found, err := cache.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if found {
		t.Fatal("expected cache miss after expiry")
	}
}

func TestMeasurementsKeyDifferentiatesRequests(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 3, 10, 12, 0, 0, 123, time.UTC)

	keys := map[string]struct{}{
		MeasurementsKey("asset-1", base, base.Add(time.Minute)):                  {},
		MeasurementsKey("asset-2", base, base.Add(time.Minute)):                  {},
		MeasurementsKey("asset-1", base.Add(time.Second), base.Add(time.Minute)): {},
		MeasurementsKey("asset-1", base, base.Add(time.Minute+time.Second)):      {},
		MeasurementsKey(" asset-1 ", base, base.Add(time.Minute)):                {},
	}

	if len(keys) != 4 {
		t.Fatalf("expected 4 unique keys with trimming, got %d", len(keys))
	}
}

func TestRedisCacheSerializationRoundTrip(t *testing.T) {
	t.Parallel()

	redisServer := miniredis.RunT(t)
	cache := newTestRedisCache(t, redisServer)

	series := query.MeasurementSeries{
		AssetID: "asset-1",
		Points: []query.MeasurementPoint{
			{
				Timestamp:   time.Date(2026, 3, 10, 12, 0, 0, 123456789, time.UTC),
				Setpoint:    12.5,
				ActivePower: 11.75,
			},
		},
	}

	key := MeasurementsKey(series.AssetID, series.Points[0].Timestamp, series.Points[0].Timestamp)
	if err := cache.Set(context.Background(), key, series, 5*time.Minute); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	got, found, err := cache.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !found {
		t.Fatal("expected cache hit")
	}

	point := got.Points[0]
	if !point.Timestamp.Equal(series.Points[0].Timestamp) {
		t.Fatalf("expected timestamp %v, got %v", series.Points[0].Timestamp, point.Timestamp)
	}
	if point.Setpoint != 12.5 || point.ActivePower != 11.75 {
		t.Fatalf("expected point values 12.5/11.75, got %v/%v", point.Setpoint, point.ActivePower)
	}
}

func newTestRedisCache(t *testing.T, redisServer *miniredis.Miniredis) *RedisCache {
	t.Helper()

	cache, err := NewRedisCache(context.Background(), redisServer.Addr(), "", "", 0)
	if err != nil {
		t.Fatalf("expected redis cache, got %v", err)
	}
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("close redis cache: %v", err)
		}
	})

	return cache
}
