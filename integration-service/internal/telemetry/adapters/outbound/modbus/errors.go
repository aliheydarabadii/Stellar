package modbus

import "errors"

var (
	ErrEmptyHost               = errors.New("empty host")
	ErrZeroPort                = errors.New("zero port")
	ErrZeroUnitID              = errors.New("zero unit id")
	ErrEmptyRegisterType       = errors.New("empty register type")
	ErrZeroSetpointAddress     = errors.New("zero setpoint address")
	ErrZeroActivePowerAddress  = errors.New("zero active power address")
	ErrUnsupportedRegisterType = errors.New("unsupported register type")
)
