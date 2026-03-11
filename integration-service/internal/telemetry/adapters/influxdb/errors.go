package influxdb

import "errors"

var (
	ErrEmptyBaseURL     = errors.New("empty base url")
	ErrEmptyOrg         = errors.New("empty org")
	ErrEmptyBucket      = errors.New("empty bucket")
	ErrEmptyToken       = errors.New("empty token")
	ErrInvalidTimeout   = errors.New("invalid timeout")
	ErrInvalidWriteMode = errors.New("invalid write mode")
	ErrRepositoryClosed = errors.New("repository closed")
)
