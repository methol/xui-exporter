package metrics

import (
	"testing"

	"github.com/methol/xui-exporter/internal/compute"
	"github.com/methol/xui-exporter/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestCollectorExportsCachedDataAfterFailure(t *testing.T) {
	st := store.New()
	st.SetSnapshot(map[string]compute.SubscriptionMetrics{
		"cached": {
			SID:                           "cached",
			Source:                        "vnstat_ssh",
			Up:                            false,
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
			DailyBudgetBytes:              253.33,
			LastRefreshTimestampSeconds:   1_100,
			LastSuccessTimestampSeconds:   1_000,
			SourceUpdatedTimestampSeconds: 990,
			RefreshDurationSeconds:        2,
		},
	})

	families := gather(t, NewCollector(st))

	assertGauge(t, families, "xui_subscription_up", 0)
	assertGauge(t, families, "xui_subscription_last_refresh_timestamp_seconds", 1_100)
	assertGauge(t, families, "xui_subscription_refresh_duration_seconds", 2)
	assertGauge(t, families, "xui_subscription_last_success_timestamp_seconds", 1_000)
	assertGauge(t, families, "xui_subscription_source_updated_timestamp_seconds", 990)
	assertGauge(t, families, "xui_subscription_source_info", 1)
	assertGauge(t, families, "xui_subscription_download_bytes", 100)
	assertGauge(t, families, "xui_subscription_upload_bytes", 20)
	assertGauge(t, families, "xui_subscription_used_bytes", 120)

	sourceInfo := families["xui_subscription_source_info"].Metric[0]
	assertLabel(t, sourceInfo, "sid", "cached")
	assertLabel(t, sourceInfo, "source", "vnstat_ssh")

	used := families["xui_subscription_used_bytes"].Metric[0]
	if len(used.Label) != 1 {
		t.Fatalf("Expected existing data metric to retain only sid label, got %d labels", len(used.Label))
	}
	assertLabel(t, used, "sid", "cached")
}

func TestCollectorOmitsDataWithoutSuccessfulRefresh(t *testing.T) {
	st := store.New()
	st.SetSnapshot(map[string]compute.SubscriptionMetrics{
		"failed": {
			SID:     "failed",
			Source:  "vnstat_ssh",
			Up:      false,
			HasData: false,
		},
	})

	families := gather(t, NewCollector(st))

	for _, name := range []string{
		"xui_subscription_up",
		"xui_subscription_last_refresh_timestamp_seconds",
		"xui_subscription_refresh_duration_seconds",
		"xui_subscription_last_success_timestamp_seconds",
		"xui_subscription_source_updated_timestamp_seconds",
		"xui_subscription_source_info",
	} {
		if _, ok := families[name]; !ok {
			t.Errorf("Expected operational metric %s to be exported", name)
		}
	}

	for _, name := range []string{
		"xui_subscription_download_bytes",
		"xui_subscription_upload_bytes",
		"xui_subscription_used_bytes",
		"xui_subscription_quota_bytes",
	} {
		if _, ok := families[name]; ok {
			t.Errorf("Did not expect data metric %s without valid data", name)
		}
	}
}

func gather(t *testing.T, collector prometheus.Collector) map[string]*dto.MetricFamily {
	t.Helper()

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	result := make(map[string]*dto.MetricFamily, len(metricFamilies))
	for _, family := range metricFamilies {
		result[family.GetName()] = family
	}
	return result
}

func assertGauge(t *testing.T, families map[string]*dto.MetricFamily, name string, want float64) {
	t.Helper()

	family, ok := families[name]
	if !ok {
		t.Fatalf("Metric family %s was not exported", name)
	}
	if len(family.Metric) != 1 {
		t.Fatalf("Metric family %s has %d metrics, want 1", name, len(family.Metric))
	}
	if got := family.Metric[0].GetGauge().GetValue(); got != want {
		t.Errorf("Metric %s = %f, want %f", name, got, want)
	}
}

func assertLabel(t *testing.T, metric *dto.Metric, name, want string) {
	t.Helper()

	for _, label := range metric.Label {
		if label.GetName() == name {
			if got := label.GetValue(); got != want {
				t.Errorf("Label %s = %q, want %q", name, got, want)
			}
			return
		}
	}
	t.Errorf("Label %s was not exported", name)
}
