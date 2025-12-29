package compute

import (
	"time"

	"github.com/methol/xui-exporter/internal/parse"
)

// SubscriptionMetrics contains all computed metrics for a subscription
type SubscriptionMetrics struct {
	// Metadata
	SID string

	// Health
	Up bool

	// Raw metrics (from x-ui)
	DownloadBytes         int64
	UploadBytes           int64
	QuotaBytes            int64
	ExpireTimestampSeconds int64

	// Derived metrics
	UsedBytes                 int64
	RemainingBytes            int64
	UsedRatio                 float64
	RemainingRatio            float64
	SecondsUntilExpire        int64
	DaysUntilExpire           float64
	Expired                   int64 // 0 or 1
	DailyBudgetBytes          float64

	// Troubleshooting metrics
	LastRefreshTimestampSeconds float64
	RefreshDurationSeconds      float64
}

// Compute calculates all derived metrics from parsed subscription data
// now is the current time used for time-based calculations
func Compute(now time.Time, parsed parse.ParsedSubscription, refreshStart time.Time) SubscriptionMetrics {
	nowUnix := now.Unix()

	// Calculate used bytes
	usedBytes := parsed.DownloadByte + parsed.UploadByte

	// Calculate remaining bytes (can be negative if over quota)
	remainingBytes := parsed.TotalByte - usedBytes

	// Calculate ratios (use float64 for precision)
	var usedRatio, remainingRatio float64
	if parsed.TotalByte > 0 {
		usedRatio = float64(usedBytes) / float64(parsed.TotalByte)
		remainingRatio = float64(remainingBytes) / float64(parsed.TotalByte)
	}

	// Calculate seconds until expire
	secondsUntilExpire := parsed.Expire - nowUnix

	// Calculate days until expire
	daysUntilExpire := float64(secondsUntilExpire) / 86400.0

	// Calculate expired flag
	var expired int64
	if secondsUntilExpire <= 0 {
		expired = 1
	}

	// Calculate daily budget bytes
	var dailyBudgetBytes float64
	if daysUntilExpire > 0 && remainingBytes > 0 {
		dailyBudgetBytes = float64(remainingBytes) / daysUntilExpire
	}
	// If expired or no remaining bytes, daily budget is 0

	// Calculate troubleshooting metrics
	refreshDuration := time.Since(refreshStart).Seconds()

	return SubscriptionMetrics{
		SID:                    parsed.SID,
		Up:                     true,
		DownloadBytes:          parsed.DownloadByte,
		UploadBytes:            parsed.UploadByte,
		QuotaBytes:             parsed.TotalByte,
		ExpireTimestampSeconds: parsed.Expire,
		UsedBytes:              usedBytes,
		RemainingBytes:         remainingBytes,
		UsedRatio:              usedRatio,
		RemainingRatio:         remainingRatio,
		SecondsUntilExpire:     secondsUntilExpire,
		DaysUntilExpire:        daysUntilExpire,
		Expired:                expired,
		DailyBudgetBytes:       dailyBudgetBytes,
		LastRefreshTimestampSeconds: float64(now.Unix()),
		RefreshDurationSeconds:      refreshDuration,
	}
}

// NewFailedMetrics creates a SubscriptionMetrics with up=0 for a failed subscription
// This is used when we know the SID but parsing/validation failed
func NewFailedMetrics(sid string, refreshStart time.Time) SubscriptionMetrics {
	now := time.Now()
	refreshDuration := now.Sub(refreshStart).Seconds()

	return SubscriptionMetrics{
		SID:                         sid,
		Up:                          false,
		LastRefreshTimestampSeconds: float64(now.Unix()),
		RefreshDurationSeconds:      refreshDuration,
	}
}
