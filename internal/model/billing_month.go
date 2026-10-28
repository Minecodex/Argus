package model

import "time"

// The date is a billing-period label, not midnight UTC as an event timestamp.
func billingMonth(at time.Time, timezone string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	local := at.In(location)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}
