package parse

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseVNStatSuccess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 12, 12, 10, 0, 0, time.UTC)
	got, updatedAt, err := ParseVNStat(loadVNStatFixture(t), validVNStatParseConfig(), now)
	if err != nil {
		t.Fatalf("ParseVNStat() error = %v", err)
	}

	want := ParsedSubscription{
		SID:          "akkocloud-us",
		DownloadByte: 900,
		UploadByte:   1200,
		TotalByte:    536870912000,
		Expire:       time.Date(2026, time.August, 17, 0, 0, 0, 0, time.UTC).Unix(),
	}
	if got != want {
		t.Errorf("ParseVNStat() = %+v, want %+v", got, want)
	}
	wantUpdated := time.Date(2026, time.August, 12, 12, 5, 0, 0, time.UTC)
	if !updatedAt.Equal(wantUpdated) {
		t.Errorf("updatedAt = %v, want %v", updatedAt, wantUpdated)
	}
}

func TestParseVNStatFlexibleJSONVersion(t *testing.T) {
	t.Parallel()

	fixture := loadVNStatFixture(t)
	tests := []struct {
		name string
		data []byte
	}{
		{name: "string", data: fixture},
		{name: "number", data: []byte(strings.Replace(string(fixture), `"jsonversion": "2"`, `"jsonversion": 2`, 1))},
		{name: "string minor version", data: []byte(strings.Replace(string(fixture), `"jsonversion": "2"`, `"jsonversion": "2.1"`, 1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := ParseVNStat(tt.data, validVNStatParseConfig(), validVNStatNow()); err != nil {
				t.Fatalf("ParseVNStat() error = %v", err)
			}
		})
	}
}

func TestParseVNStatRejectsUnsupportedJSONVersion(t *testing.T) {
	t.Parallel()

	fixture := loadVNStatFixture(t)
	data := []byte(strings.Replace(string(fixture), `"jsonversion": "2"`, `"jsonversion": 3`, 1))
	_, _, err := ParseVNStat(data, validVNStatParseConfig(), validVNStatNow())
	assertErrorContains(t, err, "unsupported vnStat JSON version: 3")
}

func TestParseVNStatMatchesConfiguredInterfaceExactly(t *testing.T) {
	t.Parallel()

	result, _, err := ParseVNStat(loadVNStatFixture(t), validVNStatParseConfig(), validVNStatNow())
	if err != nil {
		t.Fatalf("ParseVNStat() error = %v", err)
	}
	if result.DownloadByte == 999999 || result.UploadByte == 999999 {
		t.Fatalf("ParseVNStat() selected the first interface: %+v", result)
	}

	cfg := validVNStatParseConfig()
	cfg.Interface = "ens"
	_, _, err = ParseVNStat(loadVNStatFixture(t), cfg, validVNStatNow())
	assertErrorContains(t, err, `vnStat interface "ens" not found`)
}

func TestParseVNStatRejectsMissingDailyData(t *testing.T) {
	t.Parallel()

	document := loadVNStatDocument(t)
	document.Interfaces[1].Traffic.Day = nil
	_, _, err := ParseVNStat(marshalVNStatDocument(t, document), validVNStatParseConfig(), validVNStatNow())
	assertErrorContains(t, err, "missing daily traffic data")
}

func TestParseVNStatRejectsInvalidJSONFraming(t *testing.T) {
	t.Parallel()

	fixture := loadVNStatFixture(t)
	tests := []struct {
		name      string
		data      []byte
		wantError string
	}{
		{name: "empty", data: []byte(" \n\t"), wantError: "vnStat JSON is empty"},
		{name: "malformed", data: []byte(`{"jsonversion":"2"`), wantError: "parse vnStat JSON"},
		{name: "garbage prefix", data: append([]byte("notice: "), fixture...), wantError: "must start with a JSON object"},
		{name: "garbage suffix", data: append(append([]byte{}, fixture...), []byte("warning")...), wantError: "invalid trailing data"},
		{name: "second object", data: append(append([]byte{}, fixture...), []byte("{}")...), wantError: "contains data after the JSON object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := ParseVNStat(tt.data, validVNStatParseConfig(), validVNStatNow())
			assertErrorContains(t, err, tt.wantError)
		})
	}
}

func TestParseVNStatTimestampValidation(t *testing.T) {
	t.Parallel()

	t.Run("stale updated timestamp", func(t *testing.T) {
		t.Parallel()
		cfg := validVNStatParseConfig()
		now := time.Date(2026, time.August, 12, 12, 20, 1, 0, time.UTC)
		_, _, err := ParseVNStat(loadVNStatFixture(t), cfg, now)
		assertErrorContains(t, err, "vnStat data is stale")
	})

	t.Run("updated timestamp in future", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, time.August, 12, 11, 59, 59, 0, time.UTC)
		_, _, err := ParseVNStat(loadVNStatFixture(t), validVNStatParseConfig(), now)
		assertErrorContains(t, err, "more than 5 minutes in the future")
	})

	t.Run("database created after cycle start", func(t *testing.T) {
		t.Parallel()
		document := loadVNStatDocument(t)
		document.Interfaces[1].Created.Timestamp = time.Date(2026, time.July, 17, 0, 0, 1, 0, time.UTC).Unix()
		_, _, err := ParseVNStat(marshalVNStatDocument(t, document), validVNStatParseConfig(), validVNStatNow())
		assertErrorContains(t, err, "current cycle data is incomplete")
	})

	for _, field := range []string{"created", "updated"} {
		field := field
		t.Run("invalid "+field+" timestamp", func(t *testing.T) {
			t.Parallel()
			document := loadVNStatDocument(t)
			if field == "created" {
				document.Interfaces[1].Created.Timestamp = 0
			} else {
				document.Interfaces[1].Updated.Timestamp = 0
			}
			_, _, err := ParseVNStat(marshalVNStatDocument(t, document), validVNStatParseConfig(), validVNStatNow())
			assertErrorContains(t, err, "invalid "+field+" timestamp")
		})
	}
}

func TestParseVNStatRejectsInvalidDailyDate(t *testing.T) {
	t.Parallel()

	tests := []VNStatDate{
		{Year: 2026, Month: 2, Day: 30},
		{Year: 2026, Month: 13, Day: 1},
		{Year: 0, Month: 1, Day: 1},
	}
	for _, date := range tests {
		date := date
		t.Run(time.Date(max(date.Year, 1), time.Month(max(date.Month, 1)), max(date.Day, 1), 0, 0, 0, 0, time.UTC).String(), func(t *testing.T) {
			t.Parallel()
			document := loadVNStatDocument(t)
			document.Interfaces[1].Traffic.Day[0].Date = date
			_, _, err := ParseVNStat(marshalVNStatDocument(t, document), validVNStatParseConfig(), validVNStatNow())
			assertErrorContains(t, err, "invalid daily date")
		})
	}
}

func TestParseVNStatRejectsCounterOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func([]VNStatDay) []VNStatDay
		wantError string
	}{
		{
			name: "rx uint64 sum",
			mutate: func(days []VNStatDay) []VNStatDay {
				days[1].RX = math.MaxUint64
				days[2].RX = 1
				return days
			},
			wantError: "cycle rx overflows uint64",
		},
		{
			name: "tx uint64 sum",
			mutate: func(days []VNStatDay) []VNStatDay {
				days[1].TX = math.MaxUint64
				days[2].TX = 1
				return days
			},
			wantError: "cycle tx overflows uint64",
		},
		{
			name: "rx int64 conversion",
			mutate: func(days []VNStatDay) []VNStatDay {
				days[1].RX = math.MaxInt64 + 1
				days[2].RX = 0
				days[3].RX = 0
				return days
			},
			wantError: "cycle rx exceeds int64",
		},
		{
			name: "combined int64 used bytes",
			mutate: func(days []VNStatDay) []VNStatDay {
				days[1].RX = math.MaxInt64
				days[1].TX = 1
				days[2].RX, days[2].TX = 0, 0
				days[3].RX, days[3].TX = 0, 0
				return days
			},
			wantError: "cycle rx + tx exceeds int64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			document := loadVNStatDocument(t)
			document.Interfaces[1].Traffic.Day = tt.mutate(document.Interfaces[1].Traffic.Day)
			_, _, err := ParseVNStat(marshalVNStatDocument(t, document), validVNStatParseConfig(), validVNStatNow())
			assertErrorContains(t, err, tt.wantError)
		})
	}
}

func TestParseVNStatValidatesConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*VNStatParseConfig)
		wantError string
	}{
		{name: "missing SID", mutate: func(cfg *VNStatParseConfig) { cfg.SID = "" }, wantError: "SID is required"},
		{name: "missing interface", mutate: func(cfg *VNStatParseConfig) { cfg.Interface = "" }, wantError: "interface is required"},
		{name: "invalid quota", mutate: func(cfg *VNStatParseConfig) { cfg.QuotaBytes = 0 }, wantError: "quota bytes must be positive"},
		{name: "missing location", mutate: func(cfg *VNStatParseConfig) { cfg.Location = nil }, wantError: "timezone location is required"},
		{name: "invalid maximum age", mutate: func(cfg *VNStatParseConfig) { cfg.MaxDataAge = 0 }, wantError: "maximum data age must be positive"},
		{name: "invalid billing day", mutate: func(cfg *VNStatParseConfig) { cfg.BillingCycleDay = 32 }, wantError: "billing day must be between"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := validVNStatParseConfig()
			tt.mutate(&cfg)
			_, _, err := ParseVNStat(loadVNStatFixture(t), cfg, validVNStatNow())
			assertErrorContains(t, err, tt.wantError)
		})
	}
}

func validVNStatParseConfig() VNStatParseConfig {
	return VNStatParseConfig{
		SID:             "akkocloud-us",
		Interface:       "ens3",
		QuotaBytes:      536870912000,
		BillingCycleDay: 17,
		Location:        time.UTC,
		MaxDataAge:      15 * time.Minute,
	}
}

func validVNStatNow() time.Time {
	return time.Date(2026, time.August, 12, 12, 10, 0, 0, time.UTC)
}

func loadVNStatFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/vnstat-2.10-days.json")
	if err != nil {
		t.Fatalf("read vnStat fixture: %v", err)
	}
	return data
}

func loadVNStatDocument(t *testing.T) VNStatDocument {
	t.Helper()
	var document VNStatDocument
	if err := json.Unmarshal(loadVNStatFixture(t), &document); err != nil {
		t.Fatalf("unmarshal vnStat fixture: %v", err)
	}
	return document
}

func marshalVNStatDocument(t *testing.T, document VNStatDocument) []byte {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal vnStat document: %v", err)
	}
	return data
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err, want)
	}
}
