package application

import "errors"

var ErrInvalidTelemetry = errors.New("invalid telemetry")

var ErrTelemetrySource = errors.New("telemetry source")

var ErrMeasurementPersistence = errors.New("measurement persistence")

var ErrEmptyAssetID = errors.New("asset id must not be empty")

var ErrNilTelemetrySource = errors.New("telemetry source must not be nil")

var ErrNilMeasurementRepository = errors.New("measurement repository must not be nil")
