# xui-exporter

[![Build and Push Docker Image](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml/badge.svg)](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml)

A Prometheus Exporter for monitoring x-ui / 3x-ui subscription traffic usage.

## Usage

### 1. Run Exporter

```bash
docker run -d \
  --name xui-exporter \
  -p 9100:9100 \
  -e XUI_EXPORTER_TARGETS="http://example.com/sub/sid1,http://example.com/sub/sid2" \
  ghcr.io/methol/xui-exporter:latest
```

### 2. Configure Prometheus

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'xui-exporter'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 60s
```

### 3. Import Grafana Dashboard

Import `grafana-dashboard.json` from this repository:

1. Open Grafana -> Dashboards -> Import
2. Upload `grafana-dashboard.json` or paste its content
3. Select your Prometheus data source
4. Click Import
