package compute

import (
	"testing"
	"time"

	"github.com/methol/xui-exporter/internal/parse"
)

func TestCompute_Success(t *testing.T) {
	// Mock time: 2025-01-01 00:00:00 UTC
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	refreshStart := now.Add(-5 * time.Second)

	parsed := parse.ParsedSubscription{
		SID:          "test123",
		DownloadByte: 100 * 1024 * 1024 * 1024, // 100 GB
		UploadByte:   20 * 1024 * 1024 * 1024,  // 20 GB
		TotalByte:    500 * 1024 * 1024 * 1024, // 500 GB
		Expire:       now.Unix() + 30*86400,    // 30 days from now
	}

	result := Compute(now, parsed, refreshStart)

	// Check basic fields
	if result.SID != "test123" {
		t.Errorf("Expected SID 'test123', got '%s'", result.SID)
	}

	if !result.Up {
		t.Errorf("Expected Up=true, got false")
	}

	if !result.HasData {
		t.Errorf("Expected HasData=true, got false")
	}

	if result.LastSuccessTimestampSeconds != float64(now.Unix()) {
		t.Errorf("Expected LastSuccessTimestampSeconds %d, got %f", now.Unix(), result.LastSuccessTimestampSeconds)
	}

	// Check used bytes (100 GB + 20 GB = 120 GB)
	expectedUsed := int64(120 * 1024 * 1024 * 1024)
	if result.UsedBytes != expectedUsed {
		t.Errorf("Expected UsedBytes %d, got %d", expectedUsed, result.UsedBytes)
	}

	// Check remaining bytes (500 GB - 120 GB = 380 GB)
	expectedRemaining := int64(380 * 1024 * 1024 * 1024)
	if result.RemainingBytes != expectedRemaining {
		t.Errorf("Expected RemainingBytes %d, got %d", expectedRemaining, result.RemainingBytes)
	}

	// Check ratios
	expectedUsedRatio := 120.0 / 500.0
	if result.UsedRatio != expectedUsedRatio {
		t.Errorf("Expected UsedRatio %f, got %f", expectedUsedRatio, result.UsedRatio)
	}

	// Check seconds until expire (30 days = 30 * 86400)
	expectedSeconds := int64(30 * 86400)
	if result.SecondsUntilExpire != expectedSeconds {
		t.Errorf("Expected SecondsUntilExpire %d, got %d", expectedSeconds, result.SecondsUntilExpire)
	}

	// Check days until expire
	expectedDays := 30.0
	if result.DaysUntilExpire != expectedDays {
		t.Errorf("Expected DaysUntilExpire %f, got %f", expectedDays, result.DaysUntilExpire)
	}

	// Check not expired
	if result.Expired != 0 {
		t.Errorf("Expected Expired=0, got %d", result.Expired)
	}

	// Check daily budget (380 GB / 30 days)
	expectedDailyBudget := float64(expectedRemaining) / 30.0
	if result.DailyBudgetBytes != expectedDailyBudget {
		t.Errorf("Expected DailyBudgetBytes %f, got %f", expectedDailyBudget, result.DailyBudgetBytes)
	}
}

func TestComputeWithMetadata(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 5, 0, time.UTC)
	refreshStart := now.Add(-5 * time.Second)
	sourceUpdatedAt := now.Add(-2 * time.Minute)
	parsed := parse.ParsedSubscription{
		SID:       "metadata-test",
		TotalByte: 1000,
		Expire:    now.Add(24 * time.Hour).Unix(),
	}

	result := ComputeWithMetadata(now, parsed, refreshStart, SourceMetadata{
		SourceType:      "vnstat_ssh",
		SourceUpdatedAt: sourceUpdatedAt,
	})

	if result.Source != "vnstat_ssh" {
		t.Errorf("Expected Source vnstat_ssh, got %q", result.Source)
	}
	if !result.Up || !result.HasData {
		t.Errorf("Expected successful result with data, got Up=%t HasData=%t", result.Up, result.HasData)
	}
	if result.LastRefreshTimestampSeconds != float64(now.Unix()) {
		t.Errorf("Expected LastRefreshTimestampSeconds %d, got %f", now.Unix(), result.LastRefreshTimestampSeconds)
	}
	if result.LastSuccessTimestampSeconds != float64(now.Unix()) {
		t.Errorf("Expected LastSuccessTimestampSeconds %d, got %f", now.Unix(), result.LastSuccessTimestampSeconds)
	}
	if result.SourceUpdatedTimestampSeconds != float64(sourceUpdatedAt.Unix()) {
		t.Errorf("Expected SourceUpdatedTimestampSeconds %d, got %f", sourceUpdatedAt.Unix(), result.SourceUpdatedTimestampSeconds)
	}
	if result.RefreshDurationSeconds != 5 {
		t.Errorf("Expected RefreshDurationSeconds 5, got %f", result.RefreshDurationSeconds)
	}
}

func TestCompute_NegativeRemaining(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	refreshStart := now

	parsed := parse.ParsedSubscription{
		SID:          "test123",
		DownloadByte: 600 * 1024 * 1024 * 1024, // 600 GB (over quota)
		UploadByte:   0,
		TotalByte:    500 * 1024 * 1024 * 1024, // 500 GB quota
		Expire:       now.Unix() + 10*86400,
	}

	result := Compute(now, parsed, refreshStart)

	// Remaining should be negative
	expectedRemaining := int64(-100 * 1024 * 1024 * 1024)
	if result.RemainingBytes != expectedRemaining {
		t.Errorf("Expected RemainingBytes %d (negative), got %d", expectedRemaining, result.RemainingBytes)
	}

	// Daily budget should be 0 (negative remaining)
	if result.DailyBudgetBytes != 0 {
		t.Errorf("Expected DailyBudgetBytes 0 for negative remaining, got %f", result.DailyBudgetBytes)
	}
}

func TestCompute_Expired(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	refreshStart := now

	parsed := parse.ParsedSubscription{
		SID:          "test123",
		DownloadByte: 100 * 1024 * 1024 * 1024,
		UploadByte:   20 * 1024 * 1024 * 1024,
		TotalByte:    500 * 1024 * 1024 * 1024,
		Expire:       now.Unix() - 86400, // Expired 1 day ago
	}

	result := Compute(now, parsed, refreshStart)

	// Check expired flag
	if result.Expired != 1 {
		t.Errorf("Expected Expired=1, got %d", result.Expired)
	}

	// Check negative seconds until expire
	if result.SecondsUntilExpire >= 0 {
		t.Errorf("Expected negative SecondsUntilExpire, got %d", result.SecondsUntilExpire)
	}

	// Check daily budget is 0 (expired)
	if result.DailyBudgetBytes != 0 {
		t.Errorf("Expected DailyBudgetBytes 0 for expired subscription, got %f", result.DailyBudgetBytes)
	}
}

func TestCompute_ZeroQuotaDailyBudget(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	refreshStart := now

	parsed := parse.ParsedSubscription{
		SID:          "test123",
		DownloadByte: 100 * 1024 * 1024 * 1024,
		UploadByte:   0,
		TotalByte:    100 * 1024 * 1024 * 1024, // Exactly used = quota
		Expire:       now.Unix() + 10*86400,
	}

	result := Compute(now, parsed, refreshStart)

	// Remaining is 0
	if result.RemainingBytes != 0 {
		t.Errorf("Expected RemainingBytes 0, got %d", result.RemainingBytes)
	}

	// Daily budget should be 0 (no remaining)
	if result.DailyBudgetBytes != 0 {
		t.Errorf("Expected DailyBudgetBytes 0 for zero remaining, got %f", result.DailyBudgetBytes)
	}
}

func TestNewFailedMetrics(t *testing.T) {
	now := time.Now()
	refreshStart := now.Add(-3 * time.Second)

	result := NewFailedMetrics("failed123", refreshStart)

	if result.SID != "failed123" {
		t.Errorf("Expected SID 'failed123', got '%s'", result.SID)
	}

	if result.Up {
		t.Errorf("Expected Up=false, got true")
	}

	if result.HasData {
		t.Errorf("Expected HasData=false, got true")
	}

	if result.LastRefreshTimestampSeconds <= 0 {
		t.Errorf("Expected positive LastRefreshTimestampSeconds, got %f", result.LastRefreshTimestampSeconds)
	}

	if result.RefreshDurationSeconds <= 0 {
		t.Errorf("Expected positive RefreshDurationSeconds, got %f", result.RefreshDurationSeconds)
	}
}

func TestMarkFailedWithoutPreviousData(t *testing.T) {
	refreshStart := time.Now().Add(-3 * time.Second)

	result := MarkFailed("failed123", "vnstat_ssh", nil, refreshStart)

	if result.SID != "failed123" || result.Source != "vnstat_ssh" {
		t.Errorf("Expected failed123/vnstat_ssh identity, got %s/%s", result.SID, result.Source)
	}
	if result.Up || result.HasData {
		t.Errorf("Expected failed result without data, got Up=%t HasData=%t", result.Up, result.HasData)
	}
	if result.LastSuccessTimestampSeconds != 0 {
		t.Errorf("Expected no last success timestamp, got %f", result.LastSuccessTimestampSeconds)
	}
	if result.SourceUpdatedTimestampSeconds != 0 {
		t.Errorf("Expected no source updated timestamp, got %f", result.SourceUpdatedTimestampSeconds)
	}
}

func TestMarkFailedPreservesPreviousData(t *testing.T) {
	previous := SubscriptionMetrics{
		SID:                           "cached123",
		Source:                        "vnstat_ssh",
		Up:                            true,
		HasData:                       true,
		DownloadBytes:                 100,
		UploadBytes:                   20,
		QuotaBytes:                    500,
		ExpireTimestampSeconds:        2_000,
		UsedBytes:                     120,
		RemainingBytes:                380,
		UsedRatio:                     0.24,
		RemainingRatio:                0.76,
		SecondsUntilExpire:            1_000,
		DaysUntilExpire:               1.5,
		Expired:                       0,
		DailyBudgetBytes:              253.33,
		LastRefreshTimestampSeconds:   1_000,
		LastSuccessTimestampSeconds:   1_000,
		SourceUpdatedTimestampSeconds: 990,
		RefreshDurationSeconds:        1,
	}
	refreshStart := time.Now().Add(-2 * time.Second)

	result := MarkFailed("cached123", "vnstat_ssh", &previous, refreshStart)

	if result.Up || !result.HasData {
		t.Errorf("Expected failed result with cached data, got Up=%t HasData=%t", result.Up, result.HasData)
	}
	if result.DownloadBytes != previous.DownloadBytes ||
		result.UploadBytes != previous.UploadBytes ||
		result.UsedBytes != previous.UsedBytes ||
		result.RemainingBytes != previous.RemainingBytes ||
		result.DailyBudgetBytes != previous.DailyBudgetBytes {
		t.Errorf("Expected raw and derived values to be preserved, got %+v", result)
	}
	if result.LastSuccessTimestampSeconds != previous.LastSuccessTimestampSeconds {
		t.Errorf("Expected LastSuccessTimestampSeconds to remain %f, got %f", previous.LastSuccessTimestampSeconds, result.LastSuccessTimestampSeconds)
	}
	if result.SourceUpdatedTimestampSeconds != previous.SourceUpdatedTimestampSeconds {
		t.Errorf("Expected SourceUpdatedTimestampSeconds to remain %f, got %f", previous.SourceUpdatedTimestampSeconds, result.SourceUpdatedTimestampSeconds)
	}
	if result.LastRefreshTimestampSeconds <= previous.LastRefreshTimestampSeconds {
		t.Errorf("Expected LastRefreshTimestampSeconds to advance, got %f", result.LastRefreshTimestampSeconds)
	}
	if result.RefreshDurationSeconds < 2 {
		t.Errorf("Expected RefreshDurationSeconds at least 2, got %f", result.RefreshDurationSeconds)
	}
}
