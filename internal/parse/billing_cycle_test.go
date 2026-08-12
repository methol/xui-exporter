package parse

import (
	"testing"
	"time"
)

func TestCurrentBillingCycle(t *testing.T) {
	t.Parallel()

	shanghai := mustLoadLocation(t, "Asia/Shanghai")
	newYork := mustLoadLocation(t, "America/New_York")

	tests := []struct {
		name       string
		now        time.Time
		billingDay int
		loc        *time.Location
		wantStart  time.Time
		wantEnd    time.Time
	}{
		{
			name:       "first day after boundary",
			now:        time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC),
			billingDay: 1,
			loc:        time.UTC,
			wantStart:  time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "before seventeenth",
			now:        time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC),
			billingDay: 17,
			loc:        time.UTC,
			wantStart:  time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2026, time.August, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "on seventeenth at midnight",
			now:        time.Date(2026, time.August, 17, 0, 0, 0, 0, time.UTC),
			billingDay: 17,
			loc:        time.UTC,
			wantStart:  time.Date(2026, time.August, 17, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2026, time.September, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "twenty eighth in non leap february",
			now:        time.Date(2025, time.February, 28, 12, 0, 0, 0, time.UTC),
			billingDay: 28,
			loc:        time.UTC,
			wantStart:  time.Date(2025, time.February, 28, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2025, time.March, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "thirty first clamps non leap february",
			now:        time.Date(2025, time.February, 15, 12, 0, 0, 0, time.UTC),
			billingDay: 31,
			loc:        time.UTC,
			wantStart:  time.Date(2025, time.January, 31, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2025, time.February, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "thirty first clamps leap february",
			now:        time.Date(2024, time.February, 29, 12, 0, 0, 0, time.UTC),
			billingDay: 31,
			loc:        time.UTC,
			wantStart:  time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC),
			wantEnd:    time.Date(2024, time.March, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			name:       "asia shanghai uses local date",
			now:        time.Date(2026, time.August, 16, 16, 30, 0, 0, time.UTC),
			billingDay: 17,
			loc:        shanghai,
			wantStart:  time.Date(2026, time.August, 17, 0, 0, 0, 0, shanghai),
			wantEnd:    time.Date(2026, time.September, 17, 0, 0, 0, 0, shanghai),
		},
		{
			name:       "dst interval keeps local midnight",
			now:        time.Date(2026, time.March, 20, 12, 0, 0, 0, newYork),
			billingDay: 1,
			loc:        newYork,
			wantStart:  time.Date(2026, time.March, 1, 0, 0, 0, 0, newYork),
			wantEnd:    time.Date(2026, time.April, 1, 0, 0, 0, 0, newYork),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start, end, err := CurrentBillingCycle(tt.now, tt.billingDay, tt.loc)
			if err != nil {
				t.Fatalf("CurrentBillingCycle() error = %v", err)
			}
			if !start.Equal(tt.wantStart) {
				t.Errorf("start = %v, want %v", start, tt.wantStart)
			}
			if !end.Equal(tt.wantEnd) {
				t.Errorf("end = %v, want %v", end, tt.wantEnd)
			}
			if !start.Before(end) {
				t.Errorf("start %v must be before end %v", start, end)
			}
		})
	}
}

func TestCurrentBillingCycleRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	for _, billingDay := range []int{0, 32} {
		if _, _, err := CurrentBillingCycle(time.Now(), billingDay, time.UTC); err == nil {
			t.Errorf("billing day %d: expected error", billingDay)
		}
	}
	if _, _, err := CurrentBillingCycle(time.Now(), 17, nil); err == nil {
		t.Error("nil location: expected error")
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}
