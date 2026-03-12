package measurements

import "errors"

var ErrMeasurementsReaderUnavailable = errors.New("measurement service unavailable")

var ErrMeasurementsReaderInvalidRequest = errors.New("measurement service rejected request")
