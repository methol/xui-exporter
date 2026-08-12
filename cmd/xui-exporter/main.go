package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/methol/xui-exporter/internal/compute"
	"github.com/methol/xui-exporter/internal/config"
	"github.com/methol/xui-exporter/internal/fetch"
	"github.com/methol/xui-exporter/internal/metrics"
	"github.com/methol/xui-exporter/internal/parse"
	"github.com/methol/xui-exporter/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	listenAddr        = ":9100"
	metricsPath       = "/metrics"
	fetchConcurrency  = 4
	defaultConfigPath = "/etc/xui-exporter/config.json"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "", "Path to config file (default: /etc/xui-exporter/config.json)")
	flag.Parse()

	// 确定配置文件路径
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = defaultConfigPath
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		log.Fatalf("Config file not found: %s", cfgPath)
	}

	// 加载配置
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Loaded %d target(s) from %s", len(cfg.Targets), cfgPath)

	// Initialize store
	st := store.New()

	// Create and register custom collector
	collector := metrics.NewCollector(st)
	prometheus.MustRegister(collector)

	log.Printf("Registered Prometheus collector")

	// Perform initial refresh before starting server
	log.Printf("Performing initial refresh...")
	refresh(cfg.Targets, st)

	// Start refresh loop in background
	refreshInterval := time.Duration(cfg.RefreshIntervalSeconds) * time.Second
	go refreshLoop(cfg.Targets, st, refreshInterval)

	// Start HTTP server
	http.Handle(metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html>
<head><title>XUI Exporter</title></head>
<body>
<h1>XUI Exporter</h1>
<p><a href="%s">Metrics</a></p>
</body>
</html>`, metricsPath)
	})

	log.Printf("Starting HTTP server on %s", listenAddr)
	log.Printf("Metrics available at %s%s", listenAddr, metricsPath)

	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

// refreshLoop runs the refresh process on a ticker
func refreshLoop(targets []config.Target, st *store.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		refresh(targets, st)
	}
}

// refresh fetches all targets concurrently and updates the store
func refresh(targets []config.Target, st *store.Store) {
	refreshStart := time.Now()
	log.Printf("Starting refresh cycle for %d target(s)", len(targets))

	// Read the previous snapshot once so failed targets can retain valid data.
	previousSnapshot := st.GetSnapshot()

	// Create new snapshot map
	newSnapshot := make(map[string]compute.SubscriptionMetrics)
	var mu sync.Mutex

	// Semaphore for concurrency control
	sem := make(chan struct{}, fetchConcurrency)
	var wg sync.WaitGroup

	for _, target := range targets {
		wg.Add(1)
		go func(t config.Target) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			fetchAndProcess(t, previousSnapshot, refreshStart, &newSnapshot, &mu)
		}(target)
	}

	// Wait for all fetches to complete
	wg.Wait()

	// Atomically swap snapshot
	st.SetSnapshot(newSnapshot)

	duration := time.Since(refreshStart)
	log.Printf("Refresh cycle completed in %v, collected %d subscription(s)", duration, len(newSnapshot))
}

// fetchAndProcess fetches a single target, parses it, and adds to snapshot
func fetchAndProcess(
	target config.Target,
	previousSnapshot map[string]compute.SubscriptionMetrics,
	refreshStart time.Time,
	snapshot *map[string]compute.SubscriptionMetrics,
	mu *sync.Mutex,
) {
	ctx, cancel := context.WithTimeout(context.Background(), targetOperationTimeout(target))
	defer cancel()

	var parsed parse.ParsedSubscription
	metadata := compute.SourceMetadata{SourceType: target.Type}
	var err error

	switch target.Type {
	case "xui":
		parsed, err = fetchXUI(ctx, target)
	case "flux":
		parsed, err = fetchFlux(ctx, target)
	case "vnstat_ssh":
		parsed, metadata.SourceUpdatedAt, err = fetchVNStatSSH(ctx, target, time.Now())
	default:
		err = fmt.Errorf("unknown target type %q", target.Type)
	}
	if err == nil && metadata.SourceUpdatedAt.IsZero() {
		// HTTP sources do not expose an upstream update timestamp, so use the
		// time the response was successfully received.
		metadata.SourceUpdatedAt = time.Now()
	}

	if err != nil {
		storeFailedTarget(target, previousSnapshot, refreshStart, snapshot, mu)
		log.Printf("Failed to fetch %s (%s): %v", target.Name, target.Type, err)
		return
	}

	// Validate quota (quota=0 is treated as failure)
	if parsed.TotalByte == 0 {
		storeFailedTarget(target, previousSnapshot, refreshStart, snapshot, mu)
		log.Printf("Validation failed for %s: quota is 0 (not allowed)", target.Name)
		return
	}

	// Compute metrics
	now := time.Now()
	metricsData := compute.ComputeWithMetadata(now, parsed, refreshStart, metadata)
	previous, hasPrevious := previousSnapshot[target.Name]
	if err := validateUsageMonotonic(target, metricsData, previous, hasPrevious); err != nil {
		storeFailedTarget(target, previousSnapshot, refreshStart, snapshot, mu)
		log.Printf("Validation failed for %s (%s): %v", target.Name, target.Type, err)
		return
	}

	// Add to snapshot
	mu.Lock()
	(*snapshot)[target.Name] = metricsData
	mu.Unlock()

	log.Printf("Successfully processed %s (%s)", target.Name, target.Type)
}

func targetOperationTimeout(target config.Target) time.Duration {
	if target.Type == "vnstat_ssh" && target.VNStatSSH != nil {
		connectTimeout := time.Duration(target.VNStatSSH.ConnectTimeoutSeconds) * time.Second
		commandTimeout := time.Duration(target.VNStatSSH.CommandTimeoutSeconds) * time.Second
		return connectTimeout + commandTimeout + time.Second
	}
	return 15 * time.Second
}

func storeFailedTarget(
	target config.Target,
	previousSnapshot map[string]compute.SubscriptionMetrics,
	refreshStart time.Time,
	snapshot *map[string]compute.SubscriptionMetrics,
	mu *sync.Mutex,
) {
	var previous *compute.SubscriptionMetrics
	if value, ok := previousSnapshot[target.Name]; ok {
		previous = &value
	}
	failed := compute.MarkFailed(target.Name, target.Type, previous, refreshStart)

	mu.Lock()
	(*snapshot)[target.Name] = failed
	mu.Unlock()
}

func validateUsageMonotonic(
	target config.Target,
	current compute.SubscriptionMetrics,
	previous compute.SubscriptionMetrics,
	hasPrevious bool,
) error {
	if target.Type != "vnstat_ssh" || !hasPrevious || !previous.HasData {
		return nil
	}
	if current.ExpireTimestampSeconds != previous.ExpireTimestampSeconds {
		return nil
	}
	if current.UsedBytes < previous.UsedBytes {
		return fmt.Errorf(
			"same-cycle usage decreased from %d to %d bytes; possible vnStat database rebuild, rollback, or corruption",
			previous.UsedBytes,
			current.UsedBytes,
		)
	}
	return nil
}

// fetchXUI fetches and parses xui subscription HTML
func fetchXUI(ctx context.Context, target config.Target) (parse.ParsedSubscription, error) {
	htmlBytes, err := fetch.GetHTML(ctx, target.URL)
	if err != nil {
		return parse.ParsedSubscription{}, err
	}

	parsed, err := parse.ParseSubscription(htmlBytes)
	if err != nil {
		return parse.ParsedSubscription{}, err
	}

	// 使用配置文件中的 name 覆盖 SID
	parsed.SID = target.Name
	return parsed, nil
}

// fetchFlux fetches and parses flux-panel API response
func fetchFlux(ctx context.Context, target config.Target) (parse.ParsedSubscription, error) {
	jsonBytes, err := fetch.GetFluxAPI(ctx, target.URL, target.Token)
	if err != nil {
		return parse.ParsedSubscription{}, err
	}

	return parse.ParseFluxResponse(jsonBytes, target.Name, time.Now())
}

// fetchVNStatSSH queries one fixed, read-only vnStat command over SSH and
// converts its daily data into the existing ParsedSubscription model.
func fetchVNStatSSH(
	ctx context.Context,
	target config.Target,
	now time.Time,
) (parse.ParsedSubscription, time.Time, error) {
	cfg := target.VNStatSSH
	if cfg == nil {
		return parse.ParsedSubscription{}, time.Time{}, fmt.Errorf("vnstat_ssh configuration is missing")
	}

	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return parse.ParsedSubscription{}, time.Time{}, fmt.Errorf("load vnStat timezone: %w", err)
	}
	command := buildVNStatCommand(cfg)
	stdout, _, err := fetch.RunSSHCommand(ctx, fetch.SSHCommandConfig{
		Host:           cfg.Host,
		Port:           cfg.Port,
		Username:       cfg.Username,
		PrivateKeyFile: cfg.PrivateKeyFile,
		KnownHostsFile: cfg.KnownHostsFile,
		ConnectTimeout: time.Duration(cfg.ConnectTimeoutSeconds) * time.Second,
		CommandTimeout: time.Duration(cfg.CommandTimeoutSeconds) * time.Second,
		MaxOutputBytes: fetch.DefaultSSHMaxOutputBytes,
	}, command)
	if err != nil {
		return parse.ParsedSubscription{}, time.Time{}, err
	}

	parsed, updatedAt, err := parse.ParseVNStat(stdout, parse.VNStatParseConfig{
		SID:             target.Name,
		Interface:       cfg.Interface,
		QuotaBytes:      cfg.QuotaBytes,
		BillingCycleDay: cfg.BillingCycleDay,
		Location:        location,
		MaxDataAge:      time.Duration(cfg.MaxDataAgeSeconds) * time.Second,
	}, now)
	if err != nil {
		return parse.ParsedSubscription{}, time.Time{}, err
	}
	return parsed, updatedAt, nil
}

func buildVNStatCommand(cfg *config.VNStatSSHConfig) string {
	return fmt.Sprintf(
		"LC_ALL=C /usr/bin/vnstat --iface %s --json d %d",
		cfg.Interface,
		cfg.LookbackDays,
	)
}
