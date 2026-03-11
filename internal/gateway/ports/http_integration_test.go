package ports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cacheadapter "api_gateway/internal/gateway/adapters/cache"
	"api_gateway/internal/gateway/app"
	"api_gateway/internal/gateway/app/query"
	"github.com/alicebob/miniredis/v2"
)

func TestHTTPHandlerCachesIdenticalRequestsWithinTTL(t *testing.T) {
	t.Parallel()

	redisServer := miniredis.RunT(t)
	cache, err := cacheadapter.NewRedisCache(context.Background(), redisServer.Addr(), "", "", 0)
	if err != nil {
		t.Fatalf("create redis cache: %v", err)
	}
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Errorf("close redis cache: %v", err)
		}
	})

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	client := &countingMeasurementsClient{
		series: query.MeasurementSeries{
			AssetID: "asset-1",
			Points: []query.MeasurementPoint{
				{
					Timestamp:   base,
					Setpoint:    42,
					ActivePower: 41,
				},
			},
		},
	}

	application, err := app.New(client, cache, 5*time.Minute, cacheadapter.MeasurementsKey, nil)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	server := httptest.NewServer(NewHTTPHandler(application, nil, 0))
	t.Cleanup(server.Close)

	url := server.URL + "/assets/asset-1/measurements?from=" + base.Format(time.RFC3339) + "&to=" + base.Add(time.Minute).Format(time.RFC3339)

	for i := 0; i < 2; i++ {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i+1, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}

	if client.calls != 1 {
		t.Fatalf("expected client to be called once, got %d", client.calls)
	}
}

type countingMeasurementsClient struct {
	calls  int
	series query.MeasurementSeries
}

func (c *countingMeasurementsClient) GetMeasurements(_ context.Context, assetID string, from, to time.Time) (query.MeasurementSeries, error) {
	c.calls++
	if c.series.AssetID == "" {
		c.series.AssetID = assetID
	}

	return c.series, nil
}
