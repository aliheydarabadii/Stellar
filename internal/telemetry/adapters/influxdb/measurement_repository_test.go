package influxdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"stellar/internal/telemetry/domain"
)

func TestMeasurementRepositorySave(t *testing.T) {
	t.Parallel()

	collectedAt := time.Date(2026, time.March, 10, 10, 0, 0, 123, time.UTC)
	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, collectedAt)
	if err != nil {
		t.Fatalf("expected valid measurement, got %v", err)
	}

	var (
		gotMethod        string
		gotPath          string
		gotQuery         string
		gotAuthorization string
		gotBody          string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Fatalf("expected request body to be readable, got %v", readErr)
		}

		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuthorization = r.Header.Get("Authorization")
		gotBody = string(body)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	repository, err := NewMeasurementRepositoryWithConfig(Config{
		BaseURL:   server.URL,
		Org:       "demo-org",
		Bucket:    "telemetry",
		Token:     "demo-token",
		Timeout:   time.Second,
		WriteMode: WriteModeBlocking,
	}, NewPointMapper())
	if err != nil {
		t.Fatalf("expected valid repository, got %v", err)
	}

	if err := repository.Save(context.Background(), measurement); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("expected close to succeed, got %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected method %q, got %q", http.MethodPost, gotMethod)
	}

	if gotPath != "/api/v2/write" {
		t.Fatalf("expected path %q, got %q", "/api/v2/write", gotPath)
	}

	if gotQuery != "bucket=telemetry&org=demo-org&precision=ns" {
		t.Fatalf("expected query %q, got %q", "bucket=telemetry&org=demo-org&precision=ns", gotQuery)
	}

	if gotAuthorization != "Token demo-token" {
		t.Fatalf("expected authorization %q, got %q", "Token demo-token", gotAuthorization)
	}

	assertBodyContains(t, gotBody, "asset_measurements")
	assertBodyContains(t, gotBody, "asset_id=871689260010377213")
	assertBodyContains(t, gotBody, "setpoint=100")
	assertBodyContains(t, gotBody, "active_power=55")
	assertBodyContains(t, gotBody, strconv.FormatInt(collectedAt.UnixNano(), 10))
}

func TestNewMeasurementRepositoryWithConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "empty base url rejected",
			config: Config{
				Org:       "demo-org",
				Bucket:    "telemetry",
				Token:     "demo-token",
				Timeout:   time.Second,
				WriteMode: WriteModeBlocking,
			},
			wantErr: ErrEmptyBaseURL,
		},
		{
			name: "empty org rejected",
			config: Config{
				BaseURL:   "http://127.0.0.1:8086",
				Bucket:    "telemetry",
				Token:     "demo-token",
				Timeout:   time.Second,
				WriteMode: WriteModeBlocking,
			},
			wantErr: ErrEmptyOrg,
		},
		{
			name: "empty bucket rejected",
			config: Config{
				BaseURL:   "http://127.0.0.1:8086",
				Org:       "demo-org",
				Token:     "demo-token",
				Timeout:   time.Second,
				WriteMode: WriteModeBlocking,
			},
			wantErr: ErrEmptyBucket,
		},
		{
			name: "empty token rejected",
			config: Config{
				BaseURL:   "http://127.0.0.1:8086",
				Org:       "demo-org",
				Bucket:    "telemetry",
				Timeout:   time.Second,
				WriteMode: WriteModeBlocking,
			},
			wantErr: ErrEmptyToken,
		},
		{
			name: "invalid timeout rejected",
			config: Config{
				BaseURL:   "http://127.0.0.1:8086",
				Org:       "demo-org",
				Bucket:    "telemetry",
				Token:     "demo-token",
				WriteMode: WriteModeBlocking,
			},
			wantErr: ErrInvalidTimeout,
		},
		{
			name: "invalid write mode rejected",
			config: Config{
				BaseURL:   "http://127.0.0.1:8086",
				Org:       "demo-org",
				Bucket:    "telemetry",
				Token:     "demo-token",
				Timeout:   time.Second,
				WriteMode: WriteMode("invalid"),
			},
			wantErr: ErrInvalidWriteMode,
		},
		{
			name: "valid config accepted",
			config: Config{
				BaseURL:   "http://127.0.0.1:8086",
				Org:       "demo-org",
				Bucket:    "telemetry",
				Token:     "demo-token",
				Timeout:   time.Second,
				WriteMode: WriteModeBlocking,
			},
			wantErr: nil,
		},
		{
			name: "valid batch config accepted",
			config: Config{
				BaseURL:       "http://127.0.0.1:8086",
				Org:           "demo-org",
				Bucket:        "telemetry",
				Token:         "demo-token",
				Timeout:       time.Second,
				WriteMode:     WriteModeBatch,
				BatchSize:     100,
				FlushInterval: 250 * time.Millisecond,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repository, err := NewMeasurementRepositoryWithConfig(tt.config, NewPointMapper())

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if repository == nil {
					t.Fatal("expected repository to be created")
				}
				if closeErr := repository.Close(); closeErr != nil {
					t.Fatalf("expected close to succeed, got %v", closeErr)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %v, got nil", tt.wantErr)
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestMeasurementRepositorySaveReturnsWriteFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "write failed", http.StatusBadRequest)
	}))
	defer server.Close()

	repository, err := NewMeasurementRepositoryWithConfig(Config{
		BaseURL:   server.URL,
		Org:       "demo-org",
		Bucket:    "telemetry",
		Token:     "demo-token",
		Timeout:   time.Second,
		WriteMode: WriteModeBlocking,
	}, NewPointMapper())
	if err != nil {
		t.Fatalf("expected valid repository, got %v", err)
	}

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, time.Now().UTC())
	if err != nil {
		t.Fatalf("expected valid measurement, got %v", err)
	}

	err = repository.Save(context.Background(), measurement)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if closeErr := repository.Close(); closeErr != nil {
		t.Fatalf("expected close to succeed, got %v", closeErr)
	}

	if !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write failure in error, got %v", err)
	}
}

func TestMeasurementRepositoryBatchModeSaveFlushesOnInterval(t *testing.T) {
	t.Parallel()

	bodyCh := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("expected request body to be readable, got %v", err)
		}

		bodyCh <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	repository, err := NewMeasurementRepositoryWithConfig(Config{
		BaseURL:       server.URL,
		Org:           "demo-org",
		Bucket:        "telemetry",
		Token:         "demo-token",
		Timeout:       time.Second,
		WriteMode:     WriteModeBatch,
		BatchSize:     10,
		FlushInterval: 10 * time.Millisecond,
	}, NewPointMapper())
	if err != nil {
		t.Fatalf("expected valid repository, got %v", err)
	}
	defer func() {
		if closeErr := repository.Close(); closeErr != nil {
			t.Fatalf("expected close to succeed, got %v", closeErr)
		}
	}()

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, time.Now().UTC())
	if err != nil {
		t.Fatalf("expected valid measurement, got %v", err)
	}

	if err := repository.Save(context.Background(), measurement); err != nil {
		t.Fatalf("expected save to flush successfully, got %v", err)
	}

	select {
	case body := <-bodyCh:
		assertBodyContains(t, body, "asset_measurements")
		assertBodyContains(t, body, "asset_id=871689260010377213")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for batched write flush")
	}
}

func TestMeasurementRepositoryBatchModeCloseReturnsFlushFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "write failed", http.StatusBadRequest)
	}))
	defer server.Close()

	repository, err := NewMeasurementRepositoryWithConfig(Config{
		BaseURL:       server.URL,
		Org:           "demo-org",
		Bucket:        "telemetry",
		Token:         "demo-token",
		Timeout:       time.Second,
		WriteMode:     WriteModeBatch,
		BatchSize:     10,
		FlushInterval: time.Hour,
	}, NewPointMapper())
	if err != nil {
		t.Fatalf("expected valid repository, got %v", err)
	}

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, time.Now().UTC())
	if err != nil {
		t.Fatalf("expected valid measurement, got %v", err)
	}

	saveErrCh := make(chan error, 1)
	go func() {
		saveErrCh <- repository.Save(context.Background(), measurement)
	}()

	time.Sleep(20 * time.Millisecond)

	closeErr := repository.Close()
	if closeErr == nil {
		t.Fatal("expected close to return flush failure, got nil")
	}
	if !strings.Contains(closeErr.Error(), "write failed") {
		t.Fatalf("expected close error to contain write failure, got %v", closeErr)
	}

	select {
	case saveErr := <-saveErrCh:
		if saveErr == nil {
			t.Fatal("expected save to return flush failure, got nil")
		}
		if !strings.Contains(saveErr.Error(), "write failed") {
			t.Fatalf("expected save error to contain write failure, got %v", saveErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for save to return")
	}
}

func assertBodyContains(t *testing.T, body, want string) {
	t.Helper()

	if !strings.Contains(body, want) {
		t.Fatalf("expected body to contain %q, got %q", want, body)
	}
}
