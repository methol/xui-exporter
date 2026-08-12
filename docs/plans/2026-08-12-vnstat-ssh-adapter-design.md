# xui-exporter：基于 SSH + vnStat 的流量采集适配器设计与实施方案

**状态**：已进入开发
**目标仓库**：`methol-dev/xui-exporter`
**新增数据源类型**：`vnstat_ssh`
**日期**：2026-08-12

## 1. 方案结论

本次不接入 WHMCS 页面，也不使用浏览器自动化、Cookie、代理或供应商
登录态。新增 `vnstat_ssh` 数据源，由 Exporter：

1. 使用独立 SSH 私钥登录远程服务器；
2. 执行固定、只读的 `vnstat --json` 命令；
3. 获取最近若干天的 daily `rx`、`tx`；
4. 根据配置的流量周期起始日计算当前计费周期；
5. 汇总当前周期内的下载、上传和总流量；
6. 转换为现有 `ParsedSubscription`；
7. 复用 compute、store、Prometheus 和 Grafana 链路。

```text
config target
    ↓
SSH 连接远程服务器
    ↓
vnstat --json d 62
    ↓
解析 daily rx / tx
    ↓
计算当前流量周期
    ↓
构造 ParsedSubscription
    ↓
compute.Compute
    ↓
store
    ↓
Prometheus /metrics
```

核心原则：

- 远程服务器只读取，不修改 vnStat 数据库；
- 只支持 SSH 公钥认证，不支持密码、Agent 或 Keyboard Interactive；
- 强制校验 SSH Host Key，禁止 `ssh.InsecureIgnoreHostKey()`；
- 推荐每台服务器使用独立 Key，并由 `authorized_keys` 强制固定命令；
- 每轮采集新建 SSH 连接，执行后立即关闭，不做连接池或 KeepAlive；
- 周期由 Exporter 根据 daily 数据计算，不修改远程 `MonthRotate`；
- 单次 SSH 或解析故障不清空上一份有效数据；
- 保持 `xui`、`flux` 配置、现有指标名和 Grafana 查询兼容；
- 不引入数据库、浏览器或系统 `ssh` 命令。

## 2. 范围

必须完成：

- 新增 `vnstat_ssh` target；
- Go SSH 私钥认证和 `known_hosts` 校验；
- 固定 vnStat daily JSON 查询；
- vnStat 2.x JSON 解析；
- 自定义周期起始日和时区汇总；
- 映射到现有 Prometheus 指标；
- 失败时保留最后成功数据；
- 新增成功时间、源更新时间和数据源信息指标；
- 配置、周期、解析、SSH、缓存和单调性测试；
- 更新 README、示例配置和 Docker Compose。

本期不做：

- SSH 密码、ssh-agent、Keyboard Interactive、多密钥轮询；
- 任意远程命令、代理、跳板机；
- WHMCS、Virtualizor；
- 实时网络速率、逐日 Prometheus label；
- 大型插件框架或持久化数据库；
- 自动修改远程 vnStat 配置、账号或公钥；
- 非零点计费边界的 hourly 精确切割。

## 3. 数据口径

客户端固定构造：

```bash
LC_ALL=C /usr/bin/vnstat --iface ens3 --json d 62
```

其中 `interface` 经严格正则校验，lookback 是范围受限的整数，不允许配置
`remote_command`。vnStat 2.x 的 daily `rx`、`tx` 单位为字节。

当前周期使用量：

```text
download_bytes = Σ daily.rx
upload_bytes   = Σ daily.tx
used_bytes     = download_bytes + upload_bytes
```

映射：

```go
parse.ParsedSubscription{
    SID:          target.Name,
    DownloadByte: int64(cycleRX),
    UploadByte:   int64(cycleTX),
    TotalByte:    target.VNStatSSH.QuotaBytes,
    Expire:       cycleEnd.Unix(),
}
```

配额只接受明确的整数 `quota_bytes`。上线前必须人工确认供应商的单位：

```text
500 GiB = 536870912000 bytes
500 GB  = 500000000000 bytes
```

代码不接受 `"500GB"`，不猜测单位，也不加入隐式修正系数。

## 4. 配置设计

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

Go 结构：

```go
type Config struct {
    RefreshIntervalSeconds int      `json:"refresh_interval_seconds,omitempty"`
    Targets                []Target `json:"targets"`
}

type Target struct {
    Name      string           `json:"name"`
    Type      string           `json:"type"`
    URL       string           `json:"url,omitempty"`
    Token     string           `json:"token,omitempty"`
    VNStatSSH *VNStatSSHConfig `json:"vnstat_ssh,omitempty"`
}

type VNStatSSHConfig struct {
    Host                  string `json:"host"`
    Port                  int    `json:"port,omitempty"`
    Username              string `json:"username"`
    PrivateKeyFile        string `json:"private_key_file"`
    KnownHostsFile        string `json:"known_hosts_file"`
    Interface             string `json:"interface"`
    QuotaBytes            int64  `json:"quota_bytes"`
    BillingCycleDay       int    `json:"billing_cycle_day"`
    Timezone              string `json:"timezone"`
    LookbackDays          int    `json:"lookback_days,omitempty"`
    ConnectTimeoutSeconds int    `json:"connect_timeout_seconds,omitempty"`
    CommandTimeoutSeconds int    `json:"command_timeout_seconds,omitempty"`
    MaxDataAgeSeconds     int    `json:"max_data_age_seconds,omitempty"`
}
```

默认值：

```text
refresh_interval_seconds = 60
port                     = 22
lookback_days            = 62
connect_timeout_seconds  = 10
command_timeout_seconds  = 15
max_data_age_seconds     = 900
```

启动时校验：

- targets 非空、name 必填且全局唯一；
- type 仅允许 `xui`、`flux`、`vnstat_ssh`；
- `vnstat_ssh` target 必须有同名配置对象；
- host、username、interface、timezone 必填；
- port 在 `1..65535`；quota > 0；billing day 在 `1..31`；
- lookback 在 `35..400`；timeout 和 max age 为正数；
- 私钥和 `known_hosts` 必须是存在、可读的绝对路径普通文件；
- interface 匹配 `^[A-Za-z0-9_.:+-]{1,64}$`；
- `time.LoadLocation(timezone)` 成功；
- 静态镜像通过 `time/tzdata` 支持 IANA 时区。

## 5. 远程服务器初始化

检查 vnStat：

```bash
vnstat --version
vnstat --dbiflist 1
vnstat --iface ens3 --json d 2
systemctl is-active vnstat
```

创建只读账号并测试无 sudo 读取：

```bash
sudo adduser --disabled-password --gecos "" --shell /bin/sh vnstat-exporter
sudo -u vnstat-exporter /usr/bin/vnstat --iface ens3 --json d 2
```

固定命令包装器：

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

包装器必须使用绝对路径、不接受参数、不读取 `SSH_ORIGINAL_COMMAND`、不调用
sudo、不写文件，stdout 只输出 JSON。

在 Exporter 部署机创建专用 Key：

```bash
mkdir -p ./secrets
chmod 700 ./secrets
ssh-keygen -t ed25519 -f ./secrets/vnstat_ed25519 -N '' \
  -C 'xui-exporter vnstat readonly'
```

远程 `authorized_keys`：

```text
restrict,command="/usr/local/libexec/xui-exporter-vnstat" ssh-ed25519 AAAA... xui-exporter-vnstat-readonly
```

有固定出口 IP 时可加 `from="203.0.113.20/32",`。权限：

```bash
sudo chown -R vnstat-exporter:vnstat-exporter /home/vnstat-exporter/.ssh
sudo chmod 700 /home/vnstat-exporter/.ssh
sudo chmod 600 /home/vnstat-exporter/.ssh/authorized_keys
```

建立并人工核验 `known_hosts`：

```bash
ssh-keyscan -H -p 22 203.0.113.10 > ./secrets/known_hosts
chmod 600 ./secrets/known_hosts
```

`ssh-keyscan` 输出本身不证明身份，Host Key 指纹必须通过可信渠道或首次人工
SSH 登录核对。手工验证：

```bash
ssh -i ./secrets/vnstat_ed25519 \
  -o IdentitiesOnly=yes \
  -o UserKnownHostsFile=./secrets/known_hosts \
  vnstat-exporter@203.0.113.10 anything | jq .
```

预期输出 JSON、无法获得交互 shell、客户端参数被强制命令忽略，完成即断开。

## 6. SSH 客户端实现

依赖：

```text
golang.org/x/crypto/ssh
golang.org/x/crypto/ssh/knownhosts
```

接口：

```go
type SSHCommandConfig struct {
    Host           string
    Port           int
    Username       string
    PrivateKeyFile string
    KnownHostsFile string
    ConnectTimeout time.Duration
    CommandTimeout time.Duration
    MaxOutputBytes int64
}

func RunSSHCommand(ctx context.Context, cfg SSHCommandConfig, command string) (
    stdout []byte,
    stderr []byte,
    err error,
)
```

约束：

- 使用 `os.ReadFile` + `ssh.ParsePrivateKey`；
- 加密私钥返回 `encrypted private keys are not supported; use a dedicated restricted key`；
- `knownhosts.New` 生成唯一 HostKeyCallback；未知或不匹配即失败；
- 使用 `net.Dialer.DialContext`、`ssh.NewClientConn`、`ssh.NewClient`；
- context/握手/命令超时关闭整个 Client；
- 不申请 PTY，每轮只建立一次连接；
- stdout 最大 1 MiB，stderr 最大 64 KiB，使用有界 writer；
- 远程非零退出返回状态码和受限 stderr；
- 不调用 `exec.Command("ssh", ...)`。

## 7. vnStat JSON 解析

目标结构为 vnStat 2.x 的：

```text
vnstatversion
jsonversion
interfaces[]
  name
  created.timestamp
  updated.timestamp
  traffic.total.rx / tx
  traffic.day[].date / timestamp / rx / tx
```

`FlexibleVersion` 同时兼容字符串 `"2"` 和数字 `2`，只接受主版本 2。

必须校验：

- JSON 非空，第一个非空白字符为 `{`；
- 单个完整 JSON object，前后垃圾或第二个值均拒绝；
- jsonversion 主版本为 2；
- 精确匹配配置 interface，不能默认取 `interfaces[0]`；
- created/updated timestamp 为正数；
- updated 不晚于本机时间 5 分钟以上；
- `now - updated <= max_data_age`；
- daily 日期在配置时区中合法；
- rx、tx 分项求和、合计和 int64 转换均无溢出；
- stderr 不拼入 JSON。

解析接口：

```go
type VNStatParseConfig struct {
    SID             string
    Interface       string
    QuotaBytes      int64
    BillingCycleDay int
    Location        *time.Location
    MaxDataAge      time.Duration
}

func ParseVNStat(data []byte, cfg VNStatParseConfig, now time.Time) (
    ParsedSubscription,
    time.Time,
    error,
)
```

第二返回值是远端数据库的 `updated.timestamp`。

## 8. 当前计费周期

```go
func CurrentBillingCycle(now time.Time, billingDay int, loc *time.Location) (
    start time.Time,
    end time.Time,
    err error,
)
```

算法：

1. `now.In(loc)`；
2. 构造当月 billing day 00:00，超过月末时截断到最后一天；
3. 若 now 不早于该边界，start 为该边界，end 为下月对应边界；
4. 否则 end 为该边界，start 为上月对应边界；
5. 返回半开区间 `[start, end)`。

日记录不依赖 JSON timestamp 判断归属，而是使用 `date.year/month/day` 在配置
时区构造本地零点，再判断 `dayStart >= start && dayStart < end`。

覆盖 day 1、17、28、31、2 月、闰年、月末截断、UTC、Asia/Shanghai、DST。
Daily 方案仅适合当地 00:00 重置；13:27 等边界本期不支持。

## 9. 数据完整性保护

### 9.1 当前周期覆盖

若 `created.timestamp > cycle_start`，说明当前周期开始时数据库尚不存在，本轮
失败、`up=0`，保留上一份有效数据，禁止发布偏小值。

### 9.2 新鲜度和时间漂移

若 `now - updated.timestamp > max_data_age_seconds`，本轮失败并保留旧数据。
若 `updated.timestamp > now + 5m`，失败并提示检查 NTP、服务器时区和 Exporter
主机时间。

### 9.3 同周期单调性

若新旧 `expire_timestamp` 相同，则 `new.used_bytes` 必须不小于旧值。下降视为
vnStat 数据库重建、回滚或损坏：拒绝新值、`up=0`、保留旧值并记录明确日志。
若周期结束时间改变，则允许流量归零或下降。

### 9.4 已知检测边界

`updated.timestamp` 可以发现当前数据库停止更新，但无法可靠发现 vnstatd 曾经
停机数小时、后来恢复的历史缺口。远程必须执行：

```bash
systemctl enable --now vnstat
```

## 10. 失败时保留最后成功数据

公共数据模型增加：

```go
type SubscriptionMetrics struct {
    SID     string
    Source  string
    Up      bool
    HasData bool

    // 原始和派生字段保持不变

    LastRefreshTimestampSeconds   float64
    LastSuccessTimestampSeconds   float64
    SourceUpdatedTimestampSeconds float64
    RefreshDurationSeconds        float64
}
```

成功：`Up=true`、`HasData=true`、LastRefresh/LastSuccess 为本次完成时间。vnStat
的 SourceUpdated 是远端 updated；HTTP 源为成功收到响应的时间。

失败：

```go
func MarkFailed(
    sid string,
    source string,
    previous *SubscriptionMetrics,
    refreshStart time.Time,
) SubscriptionMetrics
```

首次失败为 `Up=false, HasData=false`。有历史时复制原始和派生数据，设置
`Up=false, HasData=true`，保留 LastSuccess/SourceUpdated，更新 LastRefresh 和
RefreshDuration。

Collector 无条件输出健康和排障指标，仅在 `HasData=true` 时输出流量指标，不能
再以 `!Up` 作为跳过流量的唯一条件。

## 11. 指标映射

| Prometheus 指标 | 数据来源 |
| --- | --- |
| `xui_subscription_download_bytes` | 当前周期 `Σrx` |
| `xui_subscription_upload_bytes` | 当前周期 `Σtx` |
| `xui_subscription_used_bytes` | `Σrx + Σtx` |
| `xui_subscription_quota_bytes` | `quota_bytes` |
| `xui_subscription_remaining_bytes` | quota - used |
| `xui_subscription_used_ratio` | used / quota |
| `xui_subscription_remaining_ratio` | remaining / quota |
| `xui_subscription_expire_timestamp_seconds` | 下一周期开始 |
| `xui_subscription_seconds_until_expire` | 周期结束 - now |
| `xui_subscription_days_until_expire` | 剩余秒数 / 86400 |
| `xui_subscription_daily_budget_bytes` | 剩余量 / 剩余天数 |

新增且无条件输出：

```text
xui_subscription_last_success_timestamp_seconds{sid}
xui_subscription_source_updated_timestamp_seconds{sid}
xui_subscription_source_info{sid,source} 1
```

不向既有指标添加 `source` label，避免破坏 PromQL。

## 12. 实施文件

修改：

```text
.dockerignore
.gitignore
go.mod
go.sum
cmd/xui-exporter/main.go
internal/config/config.go
internal/config/config_test.go
internal/compute/compute.go
internal/compute/compute_test.go
internal/metrics/collector.go
README.md
docker-compose.yml
```

新增：

```text
config.example.json
cmd/xui-exporter/main_test.go
internal/fetch/ssh.go
internal/fetch/ssh_test.go
internal/metrics/collector_test.go
internal/parse/vnstat.go
internal/parse/vnstat_test.go
internal/parse/billing_cycle.go
internal/parse/billing_cycle_test.go
internal/parse/testdata/vnstat-2.10-days.json
docs/plans/2026-08-12-vnstat-ssh-adapter-design.md
```

继续保留 `switch target.Type`，不将本功能与 Adapter 框架重构绑定。

## 13. 自动化测试计划

配置：旧 xui/flux、合法 vnstat、必填字段、路径、端口、interface、quota、周期
日、时区、重复 name、默认值和各范围边界。

周期：day 1/17/28/31、周期日前/当天、2 月、闰年、月末截断、UTC、上海、DST、
start < end。

解析：2.10 fixture、字符串/数字 version、多接口、缺接口、缺 day、畸形或夹杂
垃圾、stale/future/partial、周期筛选、rx/tx、非法日期和各类溢出。

SSH：进程内 Go SSH server 覆盖正常、坏/加密/缺失私钥、缺失或不匹配
known_hosts、connect/handshake/command timeout、cancel、stdout/stderr 超限、非零
退出。CI 不连接公网服务器。

可靠性：首次失败无数据；成功后失败保留；再次成功更新；同周期下降拒绝；新周期
下降接受；Collector 在 `up=0, HasData=true` 时仍输出流量。

必须执行：

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/xui-exporter
docker build -t xui-exporter:vnstat-ssh .
```

## 14. Docker 部署

Compose 改为 JSON 配置和只读 secret 挂载：

```yaml
services:
  xui-exporter:
    image: ghcr.io/methol-dev/xui-exporter:latest
    container_name: xui-exporter
    restart: unless-stopped
    ports:
      - "9100:9100"
    command:
      - -config
      - /etc/xui-exporter/config.json
    volumes:
      - ./config.json:/etc/xui-exporter/config.json:ro
      - ./secrets/vnstat_ed25519:/run/secrets/vnstat_ed25519:ro
      - ./secrets/known_hosts:/etc/xui-exporter/known_hosts:ro
    read_only: true
    tmpfs:
      - /tmp
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
```

私钥和 known_hosts 不提交 Git、不 COPY 入镜像、不写入 config、不写日志、不作为
Prometheus label、不出现在错误堆栈。

## 15. Prometheus 告警

```promql
xui_subscription_up{sid="my-vnstat-server"} == 0
```

建议 `for: 10m`。长时间未成功：

```promql
time() - xui_subscription_last_success_timestamp_seconds{sid="my-vnstat-server"} > 900
```

源数据陈旧：

```promql
time() - xui_subscription_source_updated_timestamp_seconds{sid="my-vnstat-server"} > 900
```

流量阈值建议分别为 80%、90%、95%，为客体 vnStat 与供应商计费差异保留余量。

## 16. 手工验收

1. 用受限 Key 手工执行命令并通过 `jq` 验证 JSON；
2. `curl /metrics | grep 'sid="my-vnstat-server"'`，确认 up、流量、新鲜度和
   source-info 指标；
3. 成功一次后阻断 SSH，确认 `up=0` 且 used 保留；恢复后确认 `up=1` 且
   LastSuccess 更新；
4. 至少观察一个完整周期，对比 `vnStat Σrx+Σtx` 与供应商页面；
5. 若长期存在差异，前移告警阈值，不在代码中乘修正系数。

## 17. 完成验收标准

- 原有 xui/flux 配置无需修改；
- 新增 vnstat_ssh，仅私钥认证且强制 known_hosts；
- 无 InsecureIgnoreHostKey、无系统 ssh、无任意远程命令；
- 解析 Debian 12 vnStat 2.10 daily JSON；
- billing day/timezone 汇总、rx/tx/used 与手工求和一致；
- stale/future/partial/overflow/同周期下降均失败闭锁；
- 新周期允许用量下降；
- SSH 故障不清空上一份流量；
- 新增 last-success/source-updated/source-info；
- Compose 使用 JSON 和只读 secret；
- README 含远程初始化；
- test、race、vet、Go build、Docker build 全部通过；
- Git 中无真实私钥、Host Key、密码或 Token。

## 18. 上线与回滚

上线顺序：

1. 人工确认 `billing_cycle_day`；
2. 人工确认 `quota_bytes`（GB 或 GiB）；
3. 远程创建只读用户和固定 wrapper；
4. 安装受限公钥并核验 Host Key；
5. 人工验证 SSH JSON；
6. 先用临时 target name 与供应商页面对比；
7. 确认后切换正式 target name；
8. 建立 80%、90%、95% 告警。

代码回滚只需从配置删除 `vnstat_ssh` target，xui/flux 和 Prometheus 历史数据不受
影响。远程可删除对应 authorized_keys 公钥，或在确认无其他用途后删除 wrapper
与专用账号。
