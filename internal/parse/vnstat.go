package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

const maxVNStatFutureSkew = 5 * time.Minute

// FlexibleVersion accepts vnStat's JSON version in either string or numeric form.
type FlexibleVersion string

func (v *FlexibleVersion) UnmarshalJSON(data []byte) error {
	if v == nil {
		return fmt.Errorf("cannot unmarshal vnStat JSON version into nil receiver")
	}

	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return fmt.Errorf("vnStat JSON version is empty")
	}
	if raw[0] == '"' {
		var version string
		if err := json.Unmarshal(data, &version); err != nil {
			return fmt.Errorf("decode vnStat JSON version: %w", err)
		}
		*v = FlexibleVersion(version)
		return nil
	}

	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return fmt.Errorf("vnStat JSON version must be a string or number: %w", err)
	}
	if _, err := strconv.ParseFloat(number.String(), 64); err != nil {
		return fmt.Errorf("vnStat JSON version must be a string or number: %w", err)
	}
	*v = FlexibleVersion(number.String())
	return nil
}

func (v FlexibleVersion) major() (int, error) {
	value := strings.TrimSpace(string(v))
	if value == "" {
		return 0, fmt.Errorf("vnStat JSON version is missing")
	}
	majorText := strings.SplitN(value, ".", 2)[0]
	if majorText == "" {
		return 0, fmt.Errorf("invalid vnStat JSON version %q", value)
	}
	for _, char := range majorText {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("invalid vnStat JSON version %q", value)
		}
	}
	major, err := strconv.Atoi(majorText)
	if err != nil {
		return 0, fmt.Errorf("invalid vnStat JSON version %q: %w", value, err)
	}
	return major, nil
}

type VNStatDocument struct {
	VNStatVersion string            `json:"vnstatversion"`
	JSONVersion   FlexibleVersion   `json:"jsonversion"`
	Interfaces    []VNStatInterface `json:"interfaces"`
}

type VNStatInterface struct {
	Name    string         `json:"name"`
	Alias   string         `json:"alias"`
	Created VNStatDateTime `json:"created"`
	Updated VNStatDateTime `json:"updated"`
	Traffic VNStatTraffic  `json:"traffic"`
}

type VNStatTraffic struct {
	Total VNStatCounters `json:"total"`
	Day   []VNStatDay    `json:"day"`
}

type VNStatCounters struct {
	RX uint64 `json:"rx"`
	TX uint64 `json:"tx"`
}

type VNStatDay struct {
	ID        int64      `json:"id"`
	Date      VNStatDate `json:"date"`
	Timestamp int64      `json:"timestamp"`
	RX        uint64     `json:"rx"`
	TX        uint64     `json:"tx"`
}

type VNStatDate struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

type VNStatDateTime struct {
	Date      VNStatDate `json:"date"`
	Timestamp int64      `json:"timestamp"`
}

// VNStatParseConfig contains the values needed to turn vnStat daily data into
// the exporter's existing ParsedSubscription representation.
type VNStatParseConfig struct {
	SID             string
	Interface       string
	QuotaBytes      int64
	BillingCycleDay int
	Location        *time.Location
	MaxDataAge      time.Duration
}

// ParseVNStat strictly parses vnStat 2.x daily JSON and aggregates the current
// billing cycle. The returned time is the source database's updated timestamp.
func ParseVNStat(data []byte, cfg VNStatParseConfig, now time.Time) (ParsedSubscription, time.Time, error) {
	if err := validateVNStatParseConfig(cfg); err != nil {
		return ParsedSubscription{}, time.Time{}, err
	}
	cycleStart, cycleEnd, err := CurrentBillingCycle(now, cfg.BillingCycleDay, cfg.Location)
	if err != nil {
		return ParsedSubscription{}, time.Time{}, err
	}

	document, err := decodeVNStatDocument(data)
	if err != nil {
		return ParsedSubscription{}, time.Time{}, err
	}
	major, err := document.JSONVersion.major()
	if err != nil {
		return ParsedSubscription{}, time.Time{}, err
	}
	if major != 2 {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("unsupported vnStat JSON version: %d", major)
	}

	iface, err := findVNStatInterface(document.Interfaces, cfg.Interface)
	if err != nil {
		return ParsedSubscription{}, time.Time{}, err
	}
	if iface.Created.Timestamp <= 0 {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat interface %q has invalid created timestamp: %d", cfg.Interface, iface.Created.Timestamp)
	}
	if iface.Updated.Timestamp <= 0 {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat interface %q has invalid updated timestamp: %d", cfg.Interface, iface.Updated.Timestamp)
	}

	updatedAt := time.Unix(iface.Updated.Timestamp, 0).UTC()
	if updatedAt.After(now.Add(maxVNStatFutureSkew)) {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat updated timestamp is more than 5 minutes in the future; check NTP, server timezone, and exporter host time")
	}
	if now.Sub(updatedAt) > cfg.MaxDataAge {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat data is stale: last updated %s (maximum age %s)", updatedAt.Format(time.RFC3339), cfg.MaxDataAge)
	}
	createdAt := time.Unix(iface.Created.Timestamp, 0)
	if createdAt.After(cycleStart) {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat database was created after billing cycle start %s; current cycle data is incomplete", cycleStart.Format(time.RFC3339))
	}
	if iface.Traffic.Day == nil {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat interface %q is missing daily traffic data", cfg.Interface)
	}

	rx, tx, err := aggregateVNStatDays(iface.Traffic.Day, cycleStart, cycleEnd, cfg.Location)
	if err != nil {
		return ParsedSubscription{}, time.Time{}, err
	}
	if rx > math.MaxInt64 {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat cycle rx exceeds int64: %d", rx)
	}
	if tx > math.MaxInt64 {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat cycle tx exceeds int64: %d", tx)
	}
	if rx > math.MaxUint64-tx {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat cycle rx + tx overflows uint64")
	}
	if rx+tx > math.MaxInt64 {
		return ParsedSubscription{}, time.Time{}, fmt.Errorf("vnStat cycle rx + tx exceeds int64: %d", rx+tx)
	}

	return ParsedSubscription{
		SID:          cfg.SID,
		DownloadByte: int64(rx),
		UploadByte:   int64(tx),
		TotalByte:    cfg.QuotaBytes,
		Expire:       cycleEnd.Unix(),
	}, updatedAt, nil
}

func validateVNStatParseConfig(cfg VNStatParseConfig) error {
	if cfg.SID == "" {
		return fmt.Errorf("vnStat SID is required")
	}
	if cfg.Interface == "" {
		return fmt.Errorf("vnStat interface is required")
	}
	if cfg.QuotaBytes <= 0 {
		return fmt.Errorf("vnStat quota bytes must be positive (got %d)", cfg.QuotaBytes)
	}
	if cfg.Location == nil {
		return fmt.Errorf("vnStat timezone location is required")
	}
	if cfg.MaxDataAge <= 0 {
		return fmt.Errorf("vnStat maximum data age must be positive (got %s)", cfg.MaxDataAge)
	}
	if cfg.BillingCycleDay < 1 || cfg.BillingCycleDay > 31 {
		return fmt.Errorf("billing day must be between 1 and 31 (got %d)", cfg.BillingCycleDay)
	}
	return nil
}

func decodeVNStatDocument(data []byte) (VNStatDocument, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return VNStatDocument{}, fmt.Errorf("vnStat JSON is empty")
	}
	if trimmed[0] != '{' {
		return VNStatDocument{}, fmt.Errorf("vnStat output must start with a JSON object")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var document VNStatDocument
	if err := decoder.Decode(&document); err != nil {
		return VNStatDocument{}, fmt.Errorf("parse vnStat JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return VNStatDocument{}, fmt.Errorf("vnStat output contains data after the JSON object")
		}
		return VNStatDocument{}, fmt.Errorf("vnStat output contains invalid trailing data: %w", err)
	}
	return document, nil
}

func findVNStatInterface(interfaces []VNStatInterface, name string) (VNStatInterface, error) {
	var matched *VNStatInterface
	for i := range interfaces {
		if interfaces[i].Name != name {
			continue
		}
		if matched != nil {
			return VNStatInterface{}, fmt.Errorf("vnStat output contains duplicate interface %q", name)
		}
		matched = &interfaces[i]
	}
	if matched == nil {
		return VNStatInterface{}, fmt.Errorf("vnStat interface %q not found", name)
	}
	return *matched, nil
}

func aggregateVNStatDays(days []VNStatDay, cycleStart, cycleEnd time.Time, loc *time.Location) (uint64, uint64, error) {
	var totalRX uint64
	var totalTX uint64
	for index, item := range days {
		dayStart, err := vnStatDayStart(item.Date, loc)
		if err != nil {
			return 0, 0, fmt.Errorf("vnStat daily record %d: %w", index, err)
		}
		if dayStart.Before(cycleStart) || !dayStart.Before(cycleEnd) {
			continue
		}
		if totalRX > math.MaxUint64-item.RX {
			return 0, 0, fmt.Errorf("vnStat cycle rx overflows uint64 at daily record %d", index)
		}
		if totalTX > math.MaxUint64-item.TX {
			return 0, 0, fmt.Errorf("vnStat cycle tx overflows uint64 at daily record %d", index)
		}
		totalRX += item.RX
		totalTX += item.TX
	}
	return totalRX, totalTX, nil
}

func vnStatDayStart(date VNStatDate, loc *time.Location) (time.Time, error) {
	if date.Year < 1 || date.Year > 9999 || date.Month < 1 || date.Month > 12 || date.Day < 1 || date.Day > 31 {
		return time.Time{}, fmt.Errorf("invalid daily date %04d-%02d-%02d", date.Year, date.Month, date.Day)
	}
	dayStart := time.Date(date.Year, time.Month(date.Month), date.Day, 0, 0, 0, 0, loc)
	year, month, day := dayStart.Date()
	if year != date.Year || int(month) != date.Month || day != date.Day {
		return time.Time{}, fmt.Errorf("invalid daily date %04d-%02d-%02d", date.Year, date.Month, date.Day)
	}
	return dayStart, nil
}
