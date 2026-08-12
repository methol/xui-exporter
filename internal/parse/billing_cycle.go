package parse

import (
	"fmt"
	"time"
)

// CurrentBillingCycle returns the current half-open billing interval [start, end).
// A billing day beyond a month's last day is clamped to that last day.
func CurrentBillingCycle(now time.Time, billingDay int, loc *time.Location) (start, end time.Time, err error) {
	if billingDay < 1 || billingDay > 31 {
		return time.Time{}, time.Time{}, fmt.Errorf("billing day must be between 1 and 31 (got %d)", billingDay)
	}
	if loc == nil {
		return time.Time{}, time.Time{}, fmt.Errorf("billing cycle location is required")
	}

	localNow := now.In(loc)
	year, month, _ := localNow.Date()
	candidate := billingBoundary(year, month, billingDay, loc)

	if !localNow.Before(candidate) {
		return candidate, billingBoundary(year, month+1, billingDay, loc), nil
	}

	return billingBoundary(year, month-1, billingDay, loc), candidate, nil
}

func billingBoundary(year int, month time.Month, billingDay int, loc *time.Location) time.Time {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if billingDay > lastDay {
		billingDay = lastDay
	}
	return time.Date(year, month, billingDay, 0, 0, 0, 0, loc)
}
