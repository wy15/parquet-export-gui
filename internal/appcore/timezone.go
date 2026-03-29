package appcore

import (
	odpscommon "github.com/aliyun/aliyun-odps-go-sdk/odps/common"
	"time"
	_ "time/tzdata"
)

func init() {
	ensureLocalTimezone()
}

func ensureLocalTimezone() {
	if location, err := time.LoadLocation("Asia/Shanghai"); err == nil && location != nil {
		time.Local = location
	} else if location, err := time.LoadLocation("Local"); err == nil && location != nil {
		time.Local = location
	} else if time.Local == nil {
		time.Local = time.UTC
	}

	if odpscommon.GMT == nil {
		odpscommon.GMT = time.UTC
	}
}
