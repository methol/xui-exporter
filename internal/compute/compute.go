package compute

import (
	"time"

	"github.com/methol/xui-exporter/internal/parse"
)

// SubscriptionMetrics contains all computed metrics for a subscription
type SubscriptionMetrics struct {
	// Metadata
	SID    string
	Source string

	// Health
	Up      bool
	HasData bool

	// Raw metrics (from the source)
	DownloadBytes          int64
	UploadBytes            int64
	QuotaBytes             int64
	ExpireTimestampSeconds int64

	// Derived metrics
	UsedBytes          int64
	RemainingBytes     int64
	UsedRatio          float64
	RemainingRatio     float64
	SecondsUntilExpire int64
	DaysUntilExpire    float64
	Expired            int64 // 0 or 1
	DailyBudgetBytes   float64

	// Troubleshooting metrics
	LastRefreshTimestampSeconds   float64
	LastSuccessTimestampSeconds   float64
	SourceUpdatedTimestampSeconds float64
	RefreshDurationSeconds        float64
}

// SourceMetadata describes the source of a successful refresh.
type SourceMetadata struct {
	SourceType      string
	SourceUpdatedAt time.Time
}

// Compute calculates all derived metrics from parsed subscription data
// now is the current time used for time-based calculations
func Compute(now time.Time, parsed parse.ParsedSubscription, refreshStart time.Time) SubscriptionMetrics {
	return ComputeWithMetadata(now, parsed, refreshStart, SourceMetadata{
		SourceUpdatedAt: now,
	})
}

// ComputeWithMetadata calculates all derived metrics and records source metadata.
func ComputeWithMetadata(
	now time.Time,
	parsed parse.ParsedSubscription,
	refreshStart time.Time,
	metadata SourceMetadata,
) SubscriptionMetrics {
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
	refreshDuration := now.Sub(refreshStart).Seconds()
	sourceUpdatedAt := metadata.SourceUpdatedAt
	if sourceUpdatedAt.IsZero() {
		sourceUpdatedAt = now
	}

	return SubscriptionMetrics{
		SID:                           parsed.SID,
		Source:                        metadata.SourceType,
		Up:                            true,
		HasData:                       true,
		DownloadBytes:                 parsed.DownloadByte,
		UploadBytes:                   parsed.UploadByte,
		QuotaBytes:                    parsed.TotalByte,
		ExpireTimestampSeconds:        parsed.Expire,
		UsedBytes:                     usedBytes,
		RemainingBytes:                remainingBytes,
		UsedRatio:                     usedRatio,
		RemainingRatio:                remainingRatio,
		SecondsUntilExpire:            secondsUntilExpire,
		DaysUntilExpire:               daysUntilExpire,
		Expired:                       expired,
		DailyBudgetBytes:              dailyBudgetBytes,
		LastRefreshTimestampSeconds:   float64(nowUnix),
		LastSuccessTimestampSeconds:   float64(nowUnix),
		SourceUpdatedTimestampSeconds: float64(sourceUpdatedAt.Unix()),
		RefreshDurationSeconds:        refreshDuration,
	}
}

// NewFailedMetrics creates a SubscriptionMetrics with up=0 for a failed subscription
// This is used when we know the SID but parsing/validation failed
func NewFailedMetrics(sid string, refreshStart time.Time) SubscriptionMetrics {
	return MarkFailed(sid, "", nil, refreshStart)
}

// MarkFailed records a failed refresh while preserving the last valid data.
func MarkFailed(
	sid string,
	source string,
	previous *SubscriptionMetrics,
	refreshStart time.Time,
) SubscriptionMetrics {
	now := time.Now()
	refreshDuration := now.Sub(refreshStart).Seconds()

	var result SubscriptionMetrics
	if previous != nil {
		result = *previous
	}

	result.SID = sid
	result.Source = source
	result.Up = false
	result.LastRefreshTimestampSeconds = float64(now.Unix())
	result.RefreshDurationSeconds = refreshDuration

	return result
}
