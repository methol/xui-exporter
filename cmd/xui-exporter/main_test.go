package main

import (
	"sync"
	"testing"
	"time"

	"github.com/methol/xui-exporter/internal/compute"
	"github.com/methol/xui-exporter/internal/config"
)

func TestBuildVNStatCommand(t *testing.T) {
	t.Parallel()

	cfg := &config.VNStatSSHConfig{Interface: "ens3", LookbackDays: 62}
	want := "LC_ALL=C /usr/bin/vnstat --iface ens3 --json d 62"
	if got := buildVNStatCommand(cfg); got != want {
		t.Fatalf("buildVNStatCommand() = %q, want %q", got, want)
	}
}

func TestValidateUsageMonotonic(t *testing.T) {
	t.Parallel()

	target := config.Target{Name: "vnstat", Type: "vnstat_ssh"}
	previous := compute.SubscriptionMetrics{
		HasData:                true,
		UsedBytes:              1000,
		ExpireTimestampSeconds: 2000,
	}

	tests := []struct {
		name        string
		target      config.Target
		current     compute.SubscriptionMetrics
		hasPrevious bool
		wantError   bool
	}{
		{
			name:        "same cycle increase accepted",
			target:      target,
			current:     compute.SubscriptionMetrics{UsedBytes: 1001, ExpireTimestampSeconds: 2000},
			hasPrevious: true,
		},
		{
			name:        "same cycle decrease rejected",
			target:      target,
			current:     compute.SubscriptionMetrics{UsedBytes: 999, ExpireTimestampSeconds: 2000},
			hasPrevious: true,
			wantError:   true,
		},
		{
			name:        "new cycle decrease accepted",
			target:      target,
			current:     compute.SubscriptionMetrics{UsedBytes: 1, ExpireTimestampSeconds: 3000},
			hasPrevious: true,
		},
		{
			name:        "xui is not checked",
			target:      config.Target{Name: "xui", Type: "xui"},
			current:     compute.SubscriptionMetrics{UsedBytes: 1, ExpireTimestampSeconds: 2000},
			hasPrevious: true,
		},
		{
			name:        "no previous data",
			target:      target,
			current:     compute.SubscriptionMetrics{UsedBytes: 1, ExpireTimestampSeconds: 2000},
			hasPrevious: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateUsageMonotonic(tt.target, tt.current, previous, tt.hasPrevious)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateUsageMonotonic() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestStoreFailedTargetPreservesPreviousData(t *testing.T) {
	t.Parallel()

	target := config.Target{Name: "vnstat", Type: "vnstat_ssh"}
	previous := compute.SubscriptionMetrics{
		SID:                           target.Name,
		Source:                        target.Type,
		Up:                            true,
		HasData:                       true,
		UsedBytes:                     1234,
		LastSuccessTimestampSeconds:   100,
		SourceUpdatedTimestampSeconds: 90,
	}
	previousSnapshot := map[string]compute.SubscriptionMetrics{target.Name: previous}
	newSnapshot := make(map[string]compute.SubscriptionMetrics)
	var mu sync.Mutex

	storeFailedTarget(target, previousSnapshot, time.Now().Add(-time.Second), &newSnapshot, &mu)
	got := newSnapshot[target.Name]
	if got.Up {
		t.Fatal("failed target Up = true, want false")
	}
	if !got.HasData || got.UsedBytes != previous.UsedBytes {
		t.Fatalf("failed target did not preserve data: %+v", got)
	}
	if got.LastSuccessTimestampSeconds != previous.LastSuccessTimestampSeconds {
		t.Errorf("LastSuccessTimestampSeconds = %v, want %v", got.LastSuccessTimestampSeconds, previous.LastSuccessTimestampSeconds)
	}
	if got.SourceUpdatedTimestampSeconds != previous.SourceUpdatedTimestampSeconds {
		t.Errorf("SourceUpdatedTimestampSeconds = %v, want %v", got.SourceUpdatedTimestampSeconds, previous.SourceUpdatedTimestampSeconds)
	}
}

func TestStoreFailedTargetWithoutPreviousData(t *testing.T) {
	t.Parallel()

	target := config.Target{Name: "vnstat", Type: "vnstat_ssh"}
	newSnapshot := make(map[string]compute.SubscriptionMetrics)
	var mu sync.Mutex

	storeFailedTarget(target, nil, time.Now(), &newSnapshot, &mu)
	got := newSnapshot[target.Name]
	if got.Up || got.HasData {
		t.Fatalf("first failure = %+v, want Up=false HasData=false", got)
	}
}
