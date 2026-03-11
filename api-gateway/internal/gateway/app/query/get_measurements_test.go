package query

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestNewGetMeasurementsHandlerRejectsInvalidDependencies(t *testing.T) {
	t.Parallel()

	buildKey := func(assetID string, from, to time.Time) string {
		return assetID + from.String() + to.String()
	}

	testCases := []struct {
		name    string
		client  MeasurementsClient
		cache   MeasurementsCache
		ttl     time.Duration
		keyFunc CacheKeyBuilder
		want    error
	}{
		{
			name:    "missing client",
			cache:   &fakeMeasurementsCache{},
			ttl:     5 * time.Minute,
			keyFunc: buildKey,
			want:    ErrMeasurementsClientRequired,
		},
		{
			name:    "missing cache",
			client:  &fakeMeasurementsClient{},
			ttl:     5 * time.Minute,
			keyFunc: buildKey,
			want:    ErrMeasurementsCacheRequired,
		},
		{
			name:   "missing key builder",
			client: &fakeMeasurementsClient{},
			cache:  &fakeMeasurementsCache{},
			ttl:    5 * time.Minute,
			want:   ErrCacheKeyBuilderRequired,
		},
		{
			name:    "invalid ttl",
			client:  &fakeMeasurementsClient{},
			cache:   &fakeMeasurementsCache{},
			ttl:     0,
			keyFunc: buildKey,
			want:    ErrCacheTTLInvalid,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewGetMeasurementsHandler(tc.client, tc.cache, tc.ttl, tc.keyFunc, slog.Default())
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected error %v, got %v", tc.want, err)
			}
		})
	}
}

func TestGetMeasurementsHandlerHandleRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t, &fakeMeasurementsClient{}, &fakeMeasurementsCache{})
	now := time.Now().UTC()

	testCases := []struct {
		name  string
		query GetMeasurements
		want  error
	}{
		{
			name: "empty asset id",
			query: GetMeasurements{
				AssetID: "",
				From:    now,
				To:      now,
			},
			want: ErrAssetIDRequired,
		},
		{
			name: "missing timestamps",
			query: GetMeasurements{
				AssetID: "asset-1",
			},
			want: ErrTimestampZero,
		},
		{
			name: "from after to",
			query: GetMeasurements{
				AssetID: "asset-1",
				From:    now.Add(time.Second),
				To:      now,
			},
			want: ErrInvalidTimeRange,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := handler.Handle(context.Background(), tc.query)
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected error %v, got %v", tc.want, err)
			}
		})
	}
}

func TestGetMeasurementsHandlerHandleReturnsCachedResponse(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	cached := MeasurementSeries{
		AssetID: "asset-1",
		Points: []MeasurementPoint{
			{
				Timestamp:   now,
				Setpoint:    10,
				ActivePower: 9,
			},
		},
	}

	client := &fakeMeasurementsClient{}
	cache := &fakeMeasurementsCache{
		gotValue: cached,
		found:    true,
	}
	handler := newTestHandler(t, client, cache)

	got, err := handler.Handle(context.Background(), GetMeasurements{
		AssetID: "asset-1",
		From:    now,
		To:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client.calls != 0 {
		t.Fatalf("expected client not to be called, got %d calls", client.calls)
	}
	if !cache.getCalled {
		t.Fatal("expected cache get to be called")
	}
	if cache.setCalled {
		t.Fatal("expected cache set not to be called")
	}
	if got.AssetID != cached.AssetID || len(got.Points) != len(cached.Points) {
		t.Fatalf("expected cached response, got %+v", got)
	}
}

func TestGetMeasurementsHandlerHandleFetchesAndCachesOnMiss(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	fresh := MeasurementSeries{
		AssetID: "asset-1",
		Points: []MeasurementPoint{
			{
				Timestamp:   now,
				Setpoint:    20,
				ActivePower: 18,
			},
		},
	}

	client := &fakeMeasurementsClient{series: fresh}
	cache := &fakeMeasurementsCache{}
	handler := newTestHandler(t, client, cache)

	got, err := handler.Handle(context.Background(), GetMeasurements{
		AssetID: " asset-1 ",
		From:    now,
		To:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if client.calls != 1 {
		t.Fatalf("expected client to be called once, got %d", client.calls)
	}
	if client.assetID != "asset-1" {
		t.Fatalf("expected trimmed asset id, got %q", client.assetID)
	}
	if !cache.setCalled {
		t.Fatal("expected cache set to be called")
	}
	if cache.ttl != 5*time.Minute {
		t.Fatalf("expected ttl 5m, got %v", cache.ttl)
	}
	if got.AssetID != fresh.AssetID || len(got.Points) != len(fresh.Points) {
		t.Fatalf("expected fresh response, got %+v", got)
	}
}

func TestGetMeasurementsHandlerHandleContinuesWhenCacheGetFails(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	client := &fakeMeasurementsClient{
		series: MeasurementSeries{AssetID: "asset-1"},
	}
	cache := &fakeMeasurementsCache{
		getErr: errors.New("redis unavailable"),
	}
	handler := newTestHandler(t, client, cache)

	_, err := handler.Handle(context.Background(), GetMeasurements{
		AssetID: "asset-1",
		From:    now,
		To:      now,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("expected client to be called once, got %d", client.calls)
	}
	if !cache.setCalled {
		t.Fatal("expected cache set to still be attempted")
	}
}

func TestGetMeasurementsHandlerHandleContinuesWhenCacheSetFails(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	client := &fakeMeasurementsClient{
		series: MeasurementSeries{AssetID: "asset-1"},
	}
	cache := &fakeMeasurementsCache{
		setErr: errors.New("redis unavailable"),
	}
	handler := newTestHandler(t, client, cache)

	got, err := handler.Handle(context.Background(), GetMeasurements{
		AssetID: "asset-1",
		From:    now,
		To:      now,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.AssetID != "asset-1" {
		t.Fatalf("expected successful response, got %+v", got)
	}
	if client.calls != 1 {
		t.Fatalf("expected client to be called once, got %d", client.calls)
	}
}

func TestGetMeasurementsHandlerHandleReturnsClientError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	wantErr := errors.New("service failed")

	client := &fakeMeasurementsClient{err: wantErr}
	cache := &fakeMeasurementsCache{}
	handler := newTestHandler(t, client, cache)

	_, err := handler.Handle(context.Background(), GetMeasurements{
		AssetID: "asset-1",
		From:    now,
		To:      now,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

func TestGetMeasurementsHandlerHandleCachesOnlySuccessfulResponses(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	client := &fakeMeasurementsClient{err: errors.New("service failed")}
	cache := &fakeMeasurementsCache{}
	handler := newTestHandler(t, client, cache)

	_, _ = handler.Handle(context.Background(), GetMeasurements{
		AssetID: "asset-1",
		From:    now,
		To:      now,
	})

	if cache.setCalled {
		t.Fatal("expected cache set not to be called on client error")
	}
}

func newTestHandler(t *testing.T, client MeasurementsClient, cache MeasurementsCache) GetMeasurementsHandler {
	t.Helper()

	handler, err := NewGetMeasurementsHandler(
		client,
		cache,
		5*time.Minute,
		func(assetID string, from, to time.Time) string {
			return assetID + "|" + from.Format(time.RFC3339Nano) + "|" + to.Format(time.RFC3339Nano)
		},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("expected valid handler, got %v", err)
	}

	return handler
}

type fakeMeasurementsClient struct {
	calls   int
	assetID string
	from    time.Time
	to      time.Time
	series  MeasurementSeries
	err     error
}

func (f *fakeMeasurementsClient) GetMeasurements(_ context.Context, assetID string, from, to time.Time) (MeasurementSeries, error) {
	f.calls++
	f.assetID = assetID
	f.from = from
	f.to = to

	if f.err != nil {
		return MeasurementSeries{}, f.err
	}

	return f.series, nil
}

type fakeMeasurementsCache struct {
	getCalled bool
	setCalled bool
	key       string
	value     MeasurementSeries
	gotValue  MeasurementSeries
	found     bool
	ttl       time.Duration
	getErr    error
	setErr    error
}

func (f *fakeMeasurementsCache) Get(_ context.Context, key string) (MeasurementSeries, bool, error) {
	f.getCalled = true
	f.key = key

	if f.getErr != nil {
		return MeasurementSeries{}, false, f.getErr
	}

	return f.gotValue, f.found, nil
}

func (f *fakeMeasurementsCache) Set(_ context.Context, key string, value MeasurementSeries, ttl time.Duration) error {
	f.setCalled = true
	f.key = key
	f.value = value
	f.ttl = ttl

	if f.setErr != nil {
		return f.setErr
	}

	return nil
}
