package common

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	RestoreTypeVolume   = "Volume"
	RestoryTypeSnapshot = "Snapshot"
)

// validateSnapshotLockingParameter validates an input lock_expires parameter string and returns
// the computed expire time in Unix Milliseconds or an error if the validation fails
func ValidateSnapshotLockingParameter(nowTime int64, input string) (timeInUnixMilli int64, err error) {
	parts := strings.Split(input, " ")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid format of lock_expires_at parameter, should only have 2 values (int string)")
	}

	// we except the 1st part of the parameter to be an integer
	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid format of lock_expires_at count, should be in the format of an integer")
	}

	if count < 1 {
		return 0, fmt.Errorf("invalid lock_expires_at count, should be greater than 0")
	}

	var futureTime int64

	// input will look like '1 Hours', '1 Days', '1 Weeks', '1 Months', '1 Years'
	// this function converts an input value into a numerical value representing
	// a date in the future from the current time

	const millisPerHour = 3600000
	const millisPerDay = 24 * millisPerHour
	const millisPerWeek = 7 * millisPerDay
	const millisPerMonth = 4 * millisPerWeek
	const millisPerYear = 12 * millisPerMonth

	switch parts[1] {
	case "Hours":
		futureTime = nowTime + int64((count * millisPerHour))
	case "Days":
		futureTime = nowTime + int64((count * millisPerDay))
	case "Weeks":
		futureTime = nowTime + int64((count * millisPerWeek))
	case "Months":
		futureTime = nowTime + int64((count * millisPerMonth))
	case "Years":
		futureTime = nowTime + int64((count * millisPerYear))
	default:
		return 0, fmt.Errorf("invalid format of lock_expires_at frequency, should be either Days, Hours, Weeks, Months, Years")
	}

	return futureTime, nil
}
