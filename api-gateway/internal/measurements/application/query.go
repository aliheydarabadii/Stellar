package application

import "time"

type Query struct {
	AssetID string
	From    time.Time
	To      time.Time
}
