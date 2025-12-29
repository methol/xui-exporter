package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
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
	listenAddr       = ":9100"
	metricsPath      = "/metrics"
	refreshInterval  = 60 * time.Second
	fetchConcurrency = 4
)

func main() {
	// Parse targets from environment variable
	targets, err := config.ParseTargetsFromEnv()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Loaded %d target(s) from XUI_EXPORTER_TARGETS", len(targets))

	// Initialize store
	st := store.New()

	// Create and register custom collector
	collector := metrics.NewCollector(st)
	prometheus.MustRegister(collector)

	log.Printf("Registered Prometheus collector")

	// Perform initial refresh before starting server
	log.Printf("Performing initial refresh...")
	refresh(targets, st)

	// Start refresh loop in background
	go refreshLoop(targets, st, refreshInterval)

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
func refreshLoop(targets []string, st *store.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		refresh(targets, st)
	}
}

// refresh fetches all targets concurrently and updates the store
func refresh(targets []string, st *store.Store) {
	refreshStart := time.Now()
	log.Printf("Starting refresh cycle for %d target(s)", len(targets))

	// Create new snapshot map
	newSnapshot := make(map[string]compute.SubscriptionMetrics)
	var mu sync.Mutex

	// Semaphore for concurrency control
	sem := make(chan struct{}, fetchConcurrency)
	var wg sync.WaitGroup

	for _, target := range targets {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			fetchAndProcess(url, refreshStart, &newSnapshot, &mu)
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
func fetchAndProcess(url string, refreshStart time.Time, snapshot *map[string]compute.SubscriptionMetrics, mu *sync.Mutex) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch HTML
	htmlBytes, err := fetch.GetHTML(ctx, url)
	if err != nil {
		log.Printf("Failed to fetch %s: %v", url, err)
		return
	}

	// Parse subscription data
	parsed, err := parse.ParseSubscription(htmlBytes)
	if err != nil {
		// Log error with HTML preview for debugging
		preview := string(htmlBytes)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		log.Printf("Failed to parse %s: %v\nHTML preview (first 500 chars): %s", url, err, preview)
		return
	}

	sid := parsed.SID

	// Validate quota (quota=0 is treated as failure)
	if parsed.TotalByte == 0 {
		log.Printf("Validation failed for %s (sid=%s): quota is 0 (not allowed)", url, sid)
		mu.Lock()
		(*snapshot)[sid] = compute.NewFailedMetrics(sid, refreshStart)
		mu.Unlock()
		return
	}

	// Compute metrics
	now := time.Now()
	metricsData := compute.Compute(now, parsed, refreshStart)

	// Add to snapshot (last write wins on sid collision)
	mu.Lock()
	if existing, exists := (*snapshot)[sid]; exists {
		log.Printf("Warning: SID %s appears in multiple targets, last write wins (previous from %s)", sid, url)
		// Check if we're overwriting - just for logging
		_ = existing
	}
	(*snapshot)[sid] = metricsData
	mu.Unlock()

	log.Printf("Successfully processed %s (sid=%s)", url, sid)
}
