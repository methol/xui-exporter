# xui-exporter

[![Build and Push Docker Image](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml/badge.svg)](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml)

A Prometheus Exporter for monitoring x-ui / 3x-ui subscription traffic usage and flux-panel traffic data.

## Features

- Support for x-ui / 3x-ui subscription page scraping
- Support for flux-panel API data collection
- Multiple targets in a single exporter instance
- JSON configuration file for easy management

## Configuration

Create a JSON configuration file (e.g., `config.json`):

```json
{
  "targets": [
    {
      "name": "my-xui-sub1",
      "type": "xui",
      "url": "http://example.com/sub/sid1"
    },
    {
      "name": "my-xui-sub2",
      "type": "xui",
      "url": "http://example.com/sub/sid2"
    },
    {
      "name": "my-flux-panel",
      "type": "flux",
      "url": "https://example.com/api/v1/user/package",
      "token": "your-jwt-token-here"
    }
  ]
}
```

### Target Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique identifier, used as `sid` label in Prometheus metrics |
| `type` | Yes | Target type: `xui` or `flux` |
| `url` | Yes | Target URL |
| `token` | Only for `flux` | Authorization header value (JWT token) |

## Usage

### Docker (Recommended)

```bash
docker run -d \
  --name xui-exporter \
  -p 9100:9100 \
  -v /path/to/config.json:/etc/xui-exporter/config.json \
  ghcr.io/methol/xui-exporter:latest
```

Or with a custom config path:

```bash
docker run -d \
  --name xui-exporter \
  -p 9100:9100 \
  -v /path/to/config.json:/app/config.json \
  ghcr.io/methol/xui-exporter:latest \
  -config /app/config.json
```

### Binary

```bash
# Default config path: /etc/xui-exporter/config.json
./xui-exporter

# Custom config path
./xui-exporter -config /path/to/config.json
```

## Prometheus Configuration

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'xui-exporter'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 60s
```

## Grafana Dashboard

Import `grafana-dashboard.json` from this repository:

1. Open Grafana -> Dashboards -> Import
2. Upload `grafana-dashboard.json` or paste its content
3. Select your Prometheus data source
4. Click Import

## Metrics

All metrics have label `sid` (subscription ID from config `name` field).

| Metric | Type | Description |
|--------|------|-------------|
| `xui_subscription_up` | Gauge | 1 if target is reachable, 0 otherwise |
| `xui_subscription_used_bytes` | Gauge | Total bytes used (upload + download) |
| `xui_subscription_remaining_bytes` | Gauge | Remaining bytes in quota |
| `xui_subscription_quota_bytes` | Gauge | Total quota in bytes |
| `xui_subscription_used_ratio` | Gauge | Usage ratio (0.0 - 1.0) |
| `xui_subscription_days_until_expire` | Gauge | Days until subscription expires/resets |
| `xui_subscription_expired` | Gauge | 1 if expired, 0 otherwise |
| `xui_subscription_daily_budget_bytes` | Gauge | Recommended daily usage to stay within quota |

## Development

```bash
# Build
go build -o xui-exporter ./cmd/xui-exporter

# Run tests
go test ./...

# Docker build
docker build -t xui-exporter .
```
