# xui-exporter

[![Build and Push Docker Image](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml/badge.svg)](https://github.com/methol/xui-exporter/actions/workflows/docker-build.yml)

A cached Prometheus exporter for x-ui / 3x-ui subscriptions, flux-panel, and
vnStat daily traffic collected over restricted SSH.

## Features

- Existing `xui` and `flux` target formats remain compatible.
- `vnstat_ssh` uses one private key and mandatory OpenSSH `known_hosts`
  verification. Passwords, ssh-agent, keyboard-interactive authentication,
  arbitrary commands, and the system `ssh` binary are not supported.
- vnStat 2.x daily `rx` and `tx` are aggregated into a configurable monthly
  billing cycle and exposed through the existing `xui_subscription_*` metrics.
- A failed refresh sets `up=0` while retaining the last valid traffic values.
- Multiple targets refresh concurrently; Prometheus scrapes only the in-memory
  snapshot.

## Configuration

Copy [`config.example.json`](config.example.json) to `config.json` and edit it.
The default config path is `/etc/xui-exporter/config.json`; override it with
`-config`.

```json
{
  "refresh_interval_seconds": 300,
  "targets": [
    {
      "name": "my-xui-sub1",
      "type": "xui",
      "url": "https://example.com/sub/sid1"
    },
    {
      "name": "my-flux-panel",
      "type": "flux",
      "url": "https://example.com/api/v1/user/package",
      "token": "replace-with-your-jwt-token"
    },
    {
      "name": "my-vnstat-server",
      "type": "vnstat_ssh",
      "vnstat_ssh": {
        "host": "203.0.113.10",
        "port": 22,
        "username": "vnstat-exporter",
        "private_key_file": "/run/secrets/vnstat_ed25519",
        "known_hosts_file": "/etc/xui-exporter/known_hosts",
        "interface": "ens3",
        "quota_bytes": 536870912000,
        "billing_cycle_day": 17,
        "timezone": "UTC",
        "lookback_days": 62,
        "connect_timeout_seconds": 10,
        "command_timeout_seconds": 15,
        "max_data_age_seconds": 900
      }
    }
  ]
}
```

Before production, manually confirm both `billing_cycle_day` and
`quota_bytes`. The exporter deliberately does not infer whether a provider's
“500 GB” means 500,000,000,000 bytes or 536,870,912,000 bytes.

`vnstat_ssh` defaults are: port `22`, lookback `62` days, connect timeout `10s`,
command timeout `15s`, and maximum data age `900s`. The lookback must be between
35 and 400 days. Both key paths must be absolute, readable files. The interface
is restricted to `[A-Za-z0-9_.:+-]`, and the timezone must be a valid IANA name.

## Remote vnStat setup

First verify the interface name and vnStat access on the VPS:

```bash
vnstat --version
vnstat --dbiflist 1
vnstat --iface ens3 --json d 2
systemctl is-active vnstat
```

Create an unprivileged account and verify that it can read vnStat without
`sudo`:

```bash
sudo adduser --disabled-password --gecos "" --shell /bin/sh vnstat-exporter
sudo -u vnstat-exporter /usr/bin/vnstat --iface ens3 --json d 2
```

Install a fixed command wrapper. Adjust `ens3` once here if the VPS uses a
different interface:

```bash
sudo install -d -m 0755 /usr/local/libexec
sudo tee /usr/local/libexec/xui-exporter-vnstat >/dev/null <<'EOF'
#!/bin/sh
set -eu
export LC_ALL=C
exec /usr/bin/vnstat --iface ens3 --json d 62
EOF
sudo chown root:root /usr/local/libexec/xui-exporter-vnstat
sudo chmod 0755 /usr/local/libexec/xui-exporter-vnstat
```

Create a dedicated key on the exporter host:

```bash
mkdir -p ./secrets
chmod 700 ./secrets
ssh-keygen -t ed25519 -f ./secrets/vnstat_ed25519 -N '' \
  -C 'xui-exporter vnstat readonly'
```

Put the public key in
`/home/vnstat-exporter/.ssh/authorized_keys` on the VPS using this restriction:

```text
restrict,command="/usr/local/libexec/xui-exporter-vnstat" ssh-ed25519 AAAA... xui-exporter-vnstat-readonly
```

Optionally prepend `from="EXPORTER_IP/32",`. Then set ownership and modes:

```bash
sudo chown -R vnstat-exporter:vnstat-exporter /home/vnstat-exporter/.ssh
sudo chmod 700 /home/vnstat-exporter/.ssh
sudo chmod 600 /home/vnstat-exporter/.ssh/authorized_keys
```

Create `known_hosts` and verify the host-key fingerprint through a trusted
channel or a first manual SSH login. `ssh-keyscan` output alone is not proof of
server identity.

```bash
ssh-keyscan -H -p 22 203.0.113.10 > ./secrets/known_hosts
chmod 600 ./secrets/known_hosts
ssh -i ./secrets/vnstat_ed25519 \
  -o IdentitiesOnly=yes \
  -o UserKnownHostsFile=./secrets/known_hosts \
  vnstat-exporter@203.0.113.10 ignored | jq .
```

The remote key must return JSON and must not provide an interactive shell. Keep
`vnstat` enabled under systemd; the exporter cannot reconstruct traffic missed
while `vnstatd` was stopped.

## Run

With Docker Compose:

```bash
cp config.example.json config.json
docker compose up -d
```

Or run the binary:

```bash
go build -o xui-exporter ./cmd/xui-exporter
./xui-exporter -config ./config.json
```

The exporter listens on `:9100` and exposes `/metrics`.

## Metrics

Existing metrics retain their names and `sid`-only labels:

- `xui_subscription_up`
- `xui_subscription_download_bytes`
- `xui_subscription_upload_bytes`
- `xui_subscription_used_bytes`
- `xui_subscription_quota_bytes`
- `xui_subscription_remaining_bytes`
- `xui_subscription_used_ratio`
- `xui_subscription_remaining_ratio`
- `xui_subscription_expire_timestamp_seconds`
- `xui_subscription_seconds_until_expire`
- `xui_subscription_days_until_expire`
- `xui_subscription_expired`
- `xui_subscription_daily_budget_bytes`
- `xui_subscription_last_refresh_timestamp_seconds`
- `xui_subscription_refresh_duration_seconds`

New freshness metadata:

- `xui_subscription_last_success_timestamp_seconds{sid}`
- `xui_subscription_source_updated_timestamp_seconds{sid}`
- `xui_subscription_source_info{sid,source} 1`

When a refresh fails, `up` becomes `0`. Traffic metrics remain present only if
a previous successful value exists. For vnStat, stale data, future timestamps,
an incomplete current cycle, or a same-cycle usage decrease are rejected.

Example alerts:

```promql
xui_subscription_up{sid="my-vnstat-server"} == 0
time() - xui_subscription_last_success_timestamp_seconds{sid="my-vnstat-server"} > 900
time() - xui_subscription_source_updated_timestamp_seconds{sid="my-vnstat-server"} > 900
xui_subscription_used_ratio{sid="my-vnstat-server"} > 0.80
```

## Development

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/xui-exporter
docker build -t xui-exporter:vnstat-ssh .
```

Tests use an in-process SSH server and do not connect to a public host.

## Known limitations

- Daily records only support billing boundaries at local midnight.
- vnStat measures traffic inside the guest, which may differ from provider
  billing because of accounting layer, timezone, or provider-specific rules.
- A rebuilt vnStat database makes the current cycle incomplete; the exporter
  fails closed instead of publishing a lower value.
- Current freshness detects a database that has stopped updating, but not a
  historical gap after `vnstatd` later recovers.
