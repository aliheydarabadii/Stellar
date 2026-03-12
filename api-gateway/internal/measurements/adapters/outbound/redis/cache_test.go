package redis

import (
	"context"
	"testing"
	"time"

	"api_gateway/internal/measurements"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CacheSuite struct {
	suite.Suite
}

func TestCacheSuite(t *testing.T) {
	suite.Run(t, new(CacheSuite))
}

func (s *CacheSuite) TestSetAndGet() {
	redisServer := miniredis.RunT(s.T())
	cache := newTestCache(s.T(), redisServer)

	series := measurements.MeasurementSeries{
		AssetID: "asset-1",
		Points: []measurements.MeasurementPoint{
			{
				Timestamp:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
				Setpoint:    100,
				ActivePower: 55,
			},
		},
	}

	key := MeasurementsKey("asset-1", series.Points[0].Timestamp, series.Points[0].Timestamp.Add(time.Minute))
	err := cache.Set(context.Background(), key, series, 5*time.Minute)
	s.Require().NoError(err)

	got, found, err := cache.Get(context.Background(), key)

	s.Require().NoError(err)
	s.True(found)
	s.Equal(series.AssetID, got.AssetID)
	s.Len(got.Points, 1)
}

func (s *CacheSuite) TestExpiredEntriesReturnMiss() {
	redisServer := miniredis.RunT(s.T())
	cache := newTestCache(s.T(), redisServer)

	key := MeasurementsKey("asset-1", time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), time.Date(2026, 3, 10, 12, 5, 0, 0, time.UTC))
	err := cache.Set(context.Background(), key, measurements.MeasurementSeries{AssetID: "asset-1"}, time.Second)
	s.Require().NoError(err)

	redisServer.FastForward(2 * time.Second)

	_, found, err := cache.Get(context.Background(), key)

	s.Require().NoError(err)
	s.False(found)
}

func (s *CacheSuite) TestMeasurementsKeyDifferentiatesRequests() {
	base := time.Date(2026, 3, 10, 12, 0, 0, 123, time.UTC)

	keys := map[string]struct{}{
		MeasurementsKey("asset-1", base, base.Add(time.Minute)):                  {},
		MeasurementsKey("asset-2", base, base.Add(time.Minute)):                  {},
		MeasurementsKey("asset-1", base.Add(time.Second), base.Add(time.Minute)): {},
		MeasurementsKey("asset-1", base, base.Add(time.Minute+time.Second)):      {},
		MeasurementsKey(" asset-1 ", base, base.Add(time.Minute)):                {},
	}

	s.Len(keys, 4)
}

func (s *CacheSuite) TestSerializationRoundTrip() {
	redisServer := miniredis.RunT(s.T())
	cache := newTestCache(s.T(), redisServer)

	series := measurements.MeasurementSeries{
		AssetID: "asset-1",
		Points: []measurements.MeasurementPoint{
			{
				Timestamp:   time.Date(2026, 3, 10, 12, 0, 0, 123456789, time.UTC),
				Setpoint:    12.5,
				ActivePower: 11.75,
			},
		},
	}

	key := MeasurementsKey(series.AssetID, series.Points[0].Timestamp, series.Points[0].Timestamp)
	err := cache.Set(context.Background(), key, series, 5*time.Minute)
	s.Require().NoError(err)

	got, found, err := cache.Get(context.Background(), key)

	s.Require().NoError(err)
	s.True(found)
	s.Require().Len(got.Points, 1)

	point := got.Points[0]
	s.True(point.Timestamp.Equal(series.Points[0].Timestamp))
	s.Equal(12.5, point.Setpoint)
	s.Equal(11.75, point.ActivePower)
}

func newTestCache(t *testing.T, redisServer *miniredis.Miniredis) *Cache {
	t.Helper()

	cache, err := NewCache(context.Background(), redisServer.Addr(), "", "", 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("close redis cache: %v", err)
		}
	})

	return cache
}
