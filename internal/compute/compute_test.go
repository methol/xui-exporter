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
		Expire:       now.Unix() + 30*86400,     // 30 days from now
	}

	result := Compute(now, parsed, refreshStart)

	// Check basic fields
	if result.SID != "test123" {
		t.Errorf("Expected SID 'test123', got '%s'", result.SID)
	}

	if !result.Up {
		t.Errorf("Expected Up=true, got false")
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

	if result.LastRefreshTimestampSeconds <= 0 {
		t.Errorf("Expected positive LastRefreshTimestampSeconds, got %f", result.LastRefreshTimestampSeconds)
	}

	if result.RefreshDurationSeconds <= 0 {
		t.Errorf("Expected positive RefreshDurationSeconds, got %f", result.RefreshDurationSeconds)
	}
}
