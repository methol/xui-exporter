package metrics

import (
	"github.com/methol/xui-exporter/internal/store"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector implements prometheus.Collector interface
// It reads metrics from the store snapshot and exposes them to Prometheus
type Collector struct {
	store *store.Store

	// Metric descriptors
	up                         *prometheus.Desc
	downloadBytes              *prometheus.Desc
	uploadBytes                *prometheus.Desc
	quotaBytes                 *prometheus.Desc
	expireTimestampSeconds     *prometheus.Desc
	usedBytes                  *prometheus.Desc
	remainingBytes             *prometheus.Desc
	usedRatio                  *prometheus.Desc
	remainingRatio             *prometheus.Desc
	secondsUntilExpire         *prometheus.Desc
	daysUntilExpire            *prometheus.Desc
	expired                    *prometheus.Desc
	dailyBudgetBytes           *prometheus.Desc
	lastRefreshTimestampSeconds *prometheus.Desc
	refreshDurationSeconds     *prometheus.Desc
}

// NewCollector creates a new Collector
func NewCollector(s *store.Store) *Collector {
	return &Collector{
		store: s,
		up: prometheus.NewDesc(
			"xui_subscription_up",
			"Whether the subscription was successfully scraped and parsed (1=success, 0=failure)",
			[]string{"sid"},
			nil,
		),
		downloadBytes: prometheus.NewDesc(
			"xui_subscription_download_bytes",
			"Downloaded bytes for the subscription",
			[]string{"sid"},
			nil,
		),
		uploadBytes: prometheus.NewDesc(
			"xui_subscription_upload_bytes",
			"Uploaded bytes for the subscription",
			[]string{"sid"},
			nil,
		),
		quotaBytes: prometheus.NewDesc(
			"xui_subscription_quota_bytes",
			"Total quota bytes for the subscription",
			[]string{"sid"},
			nil,
		),
		expireTimestampSeconds: prometheus.NewDesc(
			"xui_subscription_expire_timestamp_seconds",
			"Expiration timestamp in Unix epoch seconds",
			[]string{"sid"},
			nil,
		),
		usedBytes: prometheus.NewDesc(
			"xui_subscription_used_bytes",
			"Total used bytes (download + upload)",
			[]string{"sid"},
			nil,
		),
		remainingBytes: prometheus.NewDesc(
			"xui_subscription_remaining_bytes",
			"Remaining bytes (quota - used, can be negative)",
			[]string{"sid"},
			nil,
		),
		usedRatio: prometheus.NewDesc(
			"xui_subscription_used_ratio",
			"Used bytes ratio (used / quota)",
			[]string{"sid"},
			nil,
		),
		remainingRatio: prometheus.NewDesc(
			"xui_subscription_remaining_ratio",
			"Remaining bytes ratio (remaining / quota)",
			[]string{"sid"},
			nil,
		),
		secondsUntilExpire: prometheus.NewDesc(
			"xui_subscription_seconds_until_expire",
			"Seconds until expiration (can be negative if expired)",
			[]string{"sid"},
			nil,
		),
		daysUntilExpire: prometheus.NewDesc(
			"xui_subscription_days_until_expire",
			"Days until expiration (seconds_until_expire / 86400)",
			[]string{"sid"},
			nil,
		),
		expired: prometheus.NewDesc(
			"xui_subscription_expired",
			"Whether the subscription has expired (1=expired, 0=active)",
			[]string{"sid"},
			nil,
		),
		dailyBudgetBytes: prometheus.NewDesc(
			"xui_subscription_daily_budget_bytes",
			"Average daily budget bytes from now until expiration (remaining / days_until_expire)",
			[]string{"sid"},
			nil,
		),
		lastRefreshTimestampSeconds: prometheus.NewDesc(
			"xui_subscription_last_refresh_timestamp_seconds",
			"Timestamp of the last refresh attempt completion",
			[]string{"sid"},
			nil,
		),
		refreshDurationSeconds: prometheus.NewDesc(
			"xui_subscription_refresh_duration_seconds",
			"Duration of the last refresh attempt in seconds",
			[]string{"sid"},
			nil,
		),
	}
}

// Describe implements prometheus.Collector
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.downloadBytes
	ch <- c.uploadBytes
	ch <- c.quotaBytes
	ch <- c.expireTimestampSeconds
	ch <- c.usedBytes
	ch <- c.remainingBytes
	ch <- c.usedRatio
	ch <- c.remainingRatio
	ch <- c.secondsUntilExpire
	ch <- c.daysUntilExpire
	ch <- c.expired
	ch <- c.dailyBudgetBytes
	ch <- c.lastRefreshTimestampSeconds
	ch <- c.refreshDurationSeconds
}

// Collect implements prometheus.Collector
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	snapshot := c.store.GetSnapshot()

	for sid, metrics := range snapshot {
		labels := []string{sid}

		// Always export up metric
		ch <- prometheus.MustNewConstMetric(
			c.up,
			prometheus.GaugeValue,
			boolToFloat64(metrics.Up),
			labels...,
		)

		// Always export troubleshooting metrics (if available)
		if metrics.LastRefreshTimestampSeconds > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.lastRefreshTimestampSeconds,
				prometheus.GaugeValue,
				metrics.LastRefreshTimestampSeconds,
				labels...,
			)
		}

		if metrics.RefreshDurationSeconds > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.refreshDurationSeconds,
				prometheus.GaugeValue,
				metrics.RefreshDurationSeconds,
				labels...,
			)
		}

		// Only export other metrics if the subscription is up
		if !metrics.Up {
			continue
		}

		// Raw metrics
		ch <- prometheus.MustNewConstMetric(
			c.downloadBytes,
			prometheus.GaugeValue,
			float64(metrics.DownloadBytes),
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.uploadBytes,
			prometheus.GaugeValue,
			float64(metrics.UploadBytes),
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.quotaBytes,
			prometheus.GaugeValue,
			float64(metrics.QuotaBytes),
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.expireTimestampSeconds,
			prometheus.GaugeValue,
			float64(metrics.ExpireTimestampSeconds),
			labels...,
		)

		// Derived metrics
		ch <- prometheus.MustNewConstMetric(
			c.usedBytes,
			prometheus.GaugeValue,
			float64(metrics.UsedBytes),
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.remainingBytes,
			prometheus.GaugeValue,
			float64(metrics.RemainingBytes),
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.usedRatio,
			prometheus.GaugeValue,
			metrics.UsedRatio,
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.remainingRatio,
			prometheus.GaugeValue,
			metrics.RemainingRatio,
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.secondsUntilExpire,
			prometheus.GaugeValue,
			float64(metrics.SecondsUntilExpire),
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.daysUntilExpire,
			prometheus.GaugeValue,
			metrics.DaysUntilExpire,
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.expired,
			prometheus.GaugeValue,
			float64(metrics.Expired),
			labels...,
		)

		ch <- prometheus.MustNewConstMetric(
			c.dailyBudgetBytes,
			prometheus.GaugeValue,
			metrics.DailyBudgetBytes,
			labels...,
		)
	}
}

// boolToFloat64 converts a boolean to float64 (1.0 for true, 0.0 for false)
func boolToFloat64(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}
