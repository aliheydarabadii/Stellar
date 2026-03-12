package influxdb

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"stellar/internal/telemetry/domain"
)

type MeasurementRepositoryTestSuite struct {
	suite.Suite
}

func TestMeasurementRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(MeasurementRepositoryTestSuite))
}

func (s *MeasurementRepositoryTestSuite) TestMeasurementRepositorySave() {
	collectedAt := time.Date(2026, time.March, 10, 10, 0, 0, 123, time.UTC)
	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, collectedAt)
	s.Require().NoError(err)

	var (
		gotMethod        string
		gotPath          string
		gotQuery         string
		gotAuthorization string
		gotBody          string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		s.Require().NoError(readErr)

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
	s.Require().NoError(err)

	s.Require().NoError(repository.Save(context.Background(), measurement))
	s.Require().NoError(repository.Close())

	s.Equal(http.MethodPost, gotMethod)
	s.Equal("/api/v2/write", gotPath)
	s.Equal("bucket=telemetry&org=demo-org&precision=ns", gotQuery)
	s.Equal("Token demo-token", gotAuthorization)
	s.assertBodyContains(gotBody, "asset_measurements")
	s.assertBodyContains(gotBody, "asset_id=871689260010377213")
	s.assertBodyContains(gotBody, "setpoint=100")
	s.assertBodyContains(gotBody, "active_power=55")
	s.assertBodyContains(gotBody, strconv.FormatInt(collectedAt.UnixNano(), 10))
}

func (s *MeasurementRepositoryTestSuite) TestNewMeasurementRepositoryWithConfigValidation() {
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
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			repository, err := NewMeasurementRepositoryWithConfig(tt.config, NewPointMapper())

			if tt.wantErr == nil {
				s.Require().NoError(err)
				s.NotNil(repository)
				s.Require().NoError(repository.Close())
				return
			}

			s.Require().Error(err)
			s.ErrorIs(err, tt.wantErr)
		})
	}
}

func (s *MeasurementRepositoryTestSuite) TestMeasurementRepositorySaveReturnsWriteFailure() {
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
	s.Require().NoError(err)

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, time.Now().UTC())
	s.Require().NoError(err)

	err = repository.Save(context.Background(), measurement)
	s.Require().Error(err)
	s.Require().NoError(repository.Close())
	s.Contains(err.Error(), "write failed")
}

func (s *MeasurementRepositoryTestSuite) TestMeasurementRepositoryBatchModeSaveFlushesOnInterval() {
	bodyCh := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		s.Require().NoError(err)

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
	s.Require().NoError(err)
	defer func() {
		s.Require().NoError(repository.Close())
	}()

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, time.Now().UTC())
	s.Require().NoError(err)

	s.Require().NoError(repository.Save(context.Background(), measurement))

	select {
	case body := <-bodyCh:
		s.assertBodyContains(body, "asset_measurements")
		s.assertBodyContains(body, "asset_id=871689260010377213")
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for batched write flush")
	}
}

func (s *MeasurementRepositoryTestSuite) TestMeasurementRepositoryBatchModeCloseReturnsFlushFailure() {
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
	s.Require().NoError(err)

	measurement, err := domain.NewMeasurement(domain.DefaultAssetID, 100, 55, time.Now().UTC())
	s.Require().NoError(err)

	saveErrCh := make(chan error, 1)
	go func() {
		saveErrCh <- repository.Save(context.Background(), measurement)
	}()

	time.Sleep(20 * time.Millisecond)

	closeErr := repository.Close()
	s.Require().Error(closeErr)
	s.Contains(closeErr.Error(), "write failed")

	select {
	case saveErr := <-saveErrCh:
		s.Require().Error(saveErr)
		s.Contains(saveErr.Error(), "write failed")
	case <-time.After(2 * time.Second):
		s.T().Fatal("timed out waiting for save to return")
	}
}

func (s *MeasurementRepositoryTestSuite) assertBodyContains(body, want string) {
	s.T().Helper()
	s.Contains(body, want)
	s.True(strings.Contains(body, want))
}
