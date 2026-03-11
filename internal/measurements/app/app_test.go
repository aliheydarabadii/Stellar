package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"stellar/internal/measurements/app/query"
)

type ApplicationSuite struct {
	suite.Suite
}

func TestApplicationSuite(t *testing.T) {
	suite.Run(t, new(ApplicationSuite))
}

func (s *ApplicationSuite) TestNewRejectsNilReadModel() {
	_, err := New(nil)

	s.ErrorIs(err, query.ErrReadModelUnavailable)
}

func (s *ApplicationSuite) TestNewBuildsApplicationWithValidReadModel() {
	application, err := New(appReadModelStub{})
	s.Require().NoError(err)

	series, err := application.Queries.GetMeasurements.Handle(context.Background(), query.GetMeasurements{
		AssetID: "asset-1",
		From:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 3, 10, 12, 0, 1, 0, time.UTC),
	})
	s.Require().NoError(err)
	s.Equal("asset-1", series.AssetID)
}

type appReadModelStub struct{}

func (appReadModelStub) GetMeasurements(context.Context, string, time.Time, time.Time) ([]query.MeasurementPoint, error) {
	return nil, nil
}
