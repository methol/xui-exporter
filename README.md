# xui-exporter

[![Build and Push Docker Image](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml/badge.svg)](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml)

A Prometheus Exporter for monitoring x-ui / 3x-ui subscription traffic usage.

## Features

- **Automatic Discovery**: Monitors multiple subscription URLs via environment variable configuration
- **Rich Metrics**: Exports 15 metrics per subscription including raw data and derived metrics
- **Background Refresh**: Polls subscription pages every 60 seconds with cached results
- **Concurrent Fetching**: Fetches up to 4 subscriptions in parallel
- **Prometheus Native**: Custom collector for efficient metric exposition

## Metrics

### Raw Metrics (from x-ui)
- `xui_subscription_download_bytes{sid}` - Downloaded bytes
- `xui_subscription_upload_bytes{sid}` - Uploaded bytes
- `xui_subscription_quota_bytes{sid}` - Total quota bytes
- `xui_subscription_expire_timestamp_seconds{sid}` - Expiration timestamp (epoch)

### Derived Metrics (computed by exporter)
- `xui_subscription_used_bytes{sid}` - Total used bytes (download + upload)
- `xui_subscription_remaining_bytes{sid}` - Remaining bytes (can be negative)
- `xui_subscription_used_ratio{sid}` - Used ratio (0..1)
- `xui_subscription_remaining_ratio{sid}` - Remaining ratio (0..1)
- `xui_subscription_seconds_until_expire{sid}` - Seconds until expiration
- `xui_subscription_days_until_expire{sid}` - Days until expiration
- `xui_subscription_expired{sid}` - Expiration flag (0=active, 1=expired)
- `xui_subscription_daily_budget_bytes{sid}` - Daily budget from now until expiration

### Health Metrics
- `xui_subscription_up{sid}` - Scrape success indicator (0=failed, 1=success)
- `xui_subscription_last_refresh_timestamp_seconds{sid}` - Last refresh timestamp
- `xui_subscription_refresh_duration_seconds{sid}` - Last refresh duration

## Installation

### Pre-built Docker Image (Recommended)

Pull from GitHub Container Registry:

```bash
docker pull ghcr.io/methol/xui-exporter:latest

# Run container
docker run -d \
  --name xui-exporter \
  -p 9100:9100 \
  -e XUI_EXPORTER_TARGETS="http://example.com/sub/sid1,http://example.com/sub/sid2" \
  ghcr.io/methol/xui-exporter:latest
```

### Docker Compose

```bash
# Edit docker-compose.yml to set your subscription URLs
docker-compose up -d
```

### Build from Source

```bash
go build -o xui-exporter ./cmd/xui-exporter
```

### Build Docker Image

```bash
docker build -t xui-exporter .
docker run -e XUI_EXPORTER_TARGETS="http://example.com/sub/sid1,http://example.com/sub/sid2" -p 9100:9100 xui-exporter
```

## Configuration

### Environment Variables

- **`XUI_EXPORTER_TARGETS`** (required) - Comma-separated list of subscription URLs
  - Example: `http://example.com/sub/uk2jf33cdnzjn2dg,http://example.com/sub/abcd1234`

### Default Settings

- Listen address: `:9100`
- Metrics path: `/metrics`
- Refresh interval: `60 seconds`
- Fetch concurrency: `4`

## Usage

```bash
# Set subscription URLs
export XUI_EXPORTER_TARGETS="http://example.com/sub/sid1,http://example.com/sub/sid2"

# Run exporter
./xui-exporter
```

The exporter will:
1. Parse and validate the target URLs
2. Perform initial refresh
3. Start HTTP server on `:9100`
4. Refresh metrics every 60 seconds in background

Access metrics at `http://localhost:9100/metrics`

## Prometheus Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'xui-exporter'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 60s
```

## Grafana Dashboard

### Recommended Variables

- `sid`: `label_values(xui_subscription_up, sid)`

### Example PromQL Queries

**Daily Usage (MiB/day)**
```promql
increase(xui_subscription_used_bytes{sid=~"$sid"}[1d]) / 1024 / 1024
```

**Remaining Quota (GiB)**
```promql
xui_subscription_remaining_bytes{sid=~"$sid"} / 1024 / 1024 / 1024
```

**Used Percentage**
```promql
xui_subscription_used_ratio{sid=~"$sid"} * 100
```

**Days Until Expiration**
```promql
xui_subscription_days_until_expire{sid=~"$sid"}
```

**Daily Budget (MiB/day)**
```promql
xui_subscription_daily_budget_bytes{sid=~"$sid"} / 1024 / 1024
```

**Estimated Days Remaining (based on 7-day trend)**
```promql
xui_subscription_remaining_bytes{sid=~"$sid"} / ((increase(xui_subscription_used_bytes{sid=~"$sid"}[7d]) / 7))
```

## Architecture

```
┌─────────────┐       ┌──────────────┐       ┌───────────┐
│ Prometheus  │◄──────┤ HTTP Server  │◄──────┤   Store   │
│             │       │  :9100       │       │ (snapshot)│
└─────────────┘       └──────────────┘       └─────▲─────┘
                                                    │
                                              ┌─────┴─────┐
                                              │  Refresh  │
                                              │   Loop    │
                                              │  (60s)    │
                                              └─────┬─────┘
                                                    │
                            ┌───────────────────────┼───────────────────────┐
                            │                       │                       │
                      ┌─────▼─────┐           ┌─────▼─────┐         ┌─────▼─────┐
                      │  Fetcher  │           │  Fetcher  │   ...   │  Fetcher  │
                      │ (worker 1)│           │ (worker 2)│         │ (worker 4)│
                      └─────┬─────┘           └─────┬─────┘         └─────┬─────┘
                            │                       │                       │
                            ▼                       ▼                       ▼
                      ┌──────────┐            ┌──────────┐          ┌──────────┐
                      │  x-ui/   │            │  x-ui/   │          │  x-ui/   │
                      │  3x-ui   │            │  3x-ui   │          │  3x-ui   │
                      └──────────┘            └──────────┘          └──────────┘
```

## Error Handling

- **Missing target URLs**: Exporter exits with error on startup
- **HTTP errors**: Logged, subscription marked as `up=0`
- **Parse errors**: Logged, subscription marked as `up=0` (if SID known)
- **Validation errors**: Logged, subscription marked as `up=0`
- **Quota = 0**: Treated as validation error, `up=0`
- **No retry logic**: Failed subscriptions wait for next refresh cycle

## Development

### Project Structure

```
xui-exporter/
├── cmd/xui-exporter/
│   └── main.go              # Entry point
├── internal/
│   ├── config/              # Configuration parsing
│   ├── fetch/               # HTTP fetching
│   ├── parse/               # HTML parsing
│   ├── compute/             # Metrics computation
│   ├── store/               # Snapshot storage
│   └── metrics/             # Prometheus collector
├── design.md                # Design document
├── requirements.md          # Requirements specification
└── README.md               # This file
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./internal/parse -v
go test ./internal/compute -v
```

### Test Coverage

- Parse package: 7 test cases covering success, missing fields, invalid values, validation
- Compute package: 5 test cases covering normal operation, negative remaining, expiration, edge cases

## License

MIT License
