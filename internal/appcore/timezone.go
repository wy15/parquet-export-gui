package appcore

import (
	"time"
	_ "time/tzdata"
)

func init() {
	ensureLocalTimezone()
}

func ensureLocalTimezone() {
	if location, err := time.LoadLocation("Local"); err == nil && location != nil {
		time.Local = location
		return
	}

	if time.Local == nil {
		time.Local = time.UTC
	}
}
