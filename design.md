# xui-exporter 开发设计文档

> 本文档基于 `requirements.md` 固化为可直接编码的设计说明（Go 实现）。

## 1. 目标与范围

### 1.1 目标

实现一个 Prometheus Exporter：

- 通过环境变量 `XUI_EXPORTER_TARGETS` 配置多个订阅页面 URL（例如 `http://example.com/sub/<sid>`）
- 后台定时抓取订阅页面 HTML
- 从 `template#subscription-data` 的 `data-*` 数值属性中提取流量与到期信息
- 计算派生指标，降低 Grafana/PromQL 复杂度
- 暴露 `/metrics` 供 Prometheus 抓取

### 1.2 非目标

- 不做鉴权（订阅页面无需鉴权）
- 不采集字符串字段（例如 `data-download="5.73GB"`）
- 不在指标中引入 `target`/host label（仅 `sid`）
- 不做失败重试（失败等待下一轮刷新）
- 不做持久化（仅内存缓存）

## 2. 已确认的关键事实与决策

- `data-expire`：秒级 Unix epoch
- `sid`：全局唯一；指标仅使用 `sid` label
- 配额 `data-totalbyte=0`：视为异常（不允许无限额），该 `sid` 的 `xui_subscription_up{sid}=0`
- 采集模型：后台刷新 + 缓存（Prometheus 抓取不触发网络 IO）
- 刷新间隔：60 秒
- HTTP server：监听 `:9100`，metrics 路径 `/metrics`

决策（实现细节）：

- `sid` 冲突：以**最后一次成功**覆盖
- `XUI_EXPORTER_TARGETS` 为空：启动即退出并报错
- 失败重试：不重试
- `used_bytes > quota_bytes`：允许 `remaining_bytes` 为负
- 无法解析到 `sid` 的失败：仅日志记录（无法输出 per-sid 指标）
- 并发度上限：固定 4

## 3. 配置

### 3.1 环境变量

- `XUI_EXPORTER_TARGETS`（必填）
  - 逗号分隔 URL 列表
  - trim 空白、过滤空项
  - 解析后为空：`os.Exit(1)` 并输出错误

### 3.2 默认运行参数

- listen: `:9100`
- metrics path: `/metrics`
- refresh interval: `60s`
- fetch 并发度: `4`

## 4. 数据来源与解析

### 4.1 HTML 结构

从订阅页面 HTML 中定位元素：

- CSS selector：`template#subscription-data`

提取以下属性（必须）：

- `data-sid`：订阅唯一标识（string）
- `data-downloadbyte`：下载字节（非负整数）
- `data-uploadbyte`：上传字节（非负整数）
- `data-totalbyte`：总配额字节（正整数，>0）
- `data-expire`：到期时间（epoch seconds，正整数，>0）

### 4.2 校验规则

- 所有 `*byte` 字段必须能解析为非负整数
- `data-expire` 必须为正整数（epoch seconds）
- `data-totalbyte` 必须为正整数（>0）
  - 若 `data-totalbyte==0`：视为异常，若已获得 sid，则 `xui_subscription_up{sid}=0`

失败处理：

- HTTP 失败 / 非 200 / HTML 缺少目标节点 / 缺字段 / 解析失败 / 校验失败
  - 若 sid 已知：该 sid `up=0`，并记录错误原因
  - 若 sid 未知：仅记录日志

## 5. Prometheus 指标设计

### 5.1 Labels

- 所有 per-subscription 指标仅包含 `sid` label

### 5.2 Raw metrics（Gauge）

- `xui_subscription_download_bytes{sid}`
- `xui_subscription_upload_bytes{sid}`
- `xui_subscription_quota_bytes{sid}`
- `xui_subscription_expire_timestamp_seconds{sid}`

### 5.3 Derived metrics（Gauge）

- `xui_subscription_used_bytes{sid}` = download + upload
- `xui_subscription_remaining_bytes{sid}` = quota - used（允许为负）
- `xui_subscription_used_ratio{sid}` = used / quota
- `xui_subscription_remaining_ratio{sid}` = remaining / quota
- `xui_subscription_seconds_until_expire{sid}` = expire - now
- `xui_subscription_days_until_expire{sid}` = seconds_until_expire / 86400
- `xui_subscription_expired{sid}` = seconds_until_expire <= 0 ? 1 : 0
- `xui_subscription_daily_budget_bytes{sid}`
  - days_until_expire > 0：remaining / days_until_expire
  - 否则：0

### 5.4 Health metrics（Gauge）

- `xui_subscription_up{sid}`
  - 1：抓取 + 解析 + 校验成功
  - 0：失败（含 quota=0）

### 5.5 可选排障指标（建议实现）

- `xui_subscription_last_refresh_timestamp_seconds{sid}`
- `xui_subscription_refresh_duration_seconds{sid}`

语义建议：

- `last_refresh_timestamp_seconds`：本次 refresh attempt 完成时刻（成功/失败均可，前提是 sid 已知）
- `refresh_duration_seconds`：本次 attempt 耗时（成功/失败均可，前提是 sid 已知）

## 6. 架构与数据流（Mermaid）

### 6.1 流程图

```mermaid
flowchart TD
  A[Start] --> B[Parse XUI_EXPORTER_TARGETS]
  B -->|empty| X[Exit(1) + log]
  B --> C[Init Store Snapshot]
  C --> D[Register Prometheus Collector]
  D --> E[Start HTTP :9100 (/metrics)]
  D --> F[Start refresh loop (ticker 60s)]

  F --> G[Refresh cycle]
  G --> H[Fetch targets with concurrency=4]
  H --> I[For each target: GET HTML]
  I -->|error/non-200| J[Log error (sid unknown)]
  I -->|200| K[Parse template#subscription-data]
  K -->|fail| J
  K --> L[Validate fields]
  L -->|fail| M[Build sid snapshot with up=0 + log]
  L -->|ok| N[Compute derived metrics]
  N --> O[Build sid snapshot with up=1]
  M --> P[Merge into new snapshot map]
  O --> P
  P --> Q[Atomic swap store snapshot]
```

### 6.2 时序图

```mermaid
sequenceDiagram
  participant Prom as Prometheus
  participant HTTP as Exporter HTTP
  participant Loop as Refresh Loop
  participant Fetch as Fetcher/Parser
  participant XUI as x-ui/3x-ui
  participant Store as Store

  Loop->>Fetch: refresh(targets)
  par for each target (limit 4)
    Fetch->>XUI: GET /sub/<sid>
    XUI-->>Fetch: 200 + HTML
    Fetch->>Fetch: parse + validate + compute
    Fetch->>Store: write into new snapshot
  end
  Fetch->>Store: atomic swap snapshot

  Prom->>HTTP: GET /metrics
  HTTP->>Store: read current snapshot
  Store-->>HTTP: snapshot
  HTTP-->>Prom: exposition
```

## 7. Go 模块划分与接口（建议）

> 目录结构可调整，但边界建议保留，便于单测与维护。

- `cmd/xui-exporter/main.go`
  - 解析配置（targets）
  - 初始化 store
  - 注册 collector
  - 启动 refresh loop
  - 启动 HTTP server

- `internal/config`
  - `ParseTargetsFromEnv() ([]string, error)`

- `internal/fetch`
  - `GetHTML(ctx context.Context, url string) ([]byte, error)`

- `internal/parse`
  - `ParseSubscription(html []byte) (ParsedSubscription, error)`
  - 推荐实现：使用 `golang.org/x/net/html`（低依赖）或 `github.com/PuerkitoBio/goquery`（更易写）。二选一即可。

- `internal/compute`
  - `Compute(now time.Time, parsed ParsedSubscription) SubscriptionMetrics`

- `internal/store`
  - 保存 `map[string]SubscriptionMetrics` 的快照
  - 快照替换：一次 refresh 构建新 map，完成后 atomic swap

- `internal/metrics`
  - 自定义 Prometheus `Collector`：每次 Collect 时从 store 取快照并输出

## 8. 并发、缓存与一致性

### 8.1 并发度

- refresh 内部抓取并发度固定为 4
- 建议实现：semaphore（容量 4）或 worker pool

### 8.2 缓存策略

- Store 持有“当前快照”
- refresh：构建 `newSnapshot := map[string]SubscriptionMetrics{}`，全部完成后一次性替换
- `/metrics`：只读当前快照

### 8.3 sid 冲突

- 同一轮 refresh 内出现重复 sid 的多次成功结果：按抓取完成顺序写入 `newSnapshot[sid] = ...`，后写入覆盖先写入
- 建议记录 warn log（不影响指标）

## 9. 错误处理与日志

- 对每个 target 的失败记录原因：
  - 网络错误 / 超时
  - HTTP 非 200
  - 解析失败（缺 template、缺字段、格式错误）
  - 校验失败（expire<=0、quota==0 等）

- sid 不可得：仅日志记录（不会产生 per-sid `up` 指标）
- sid 可得但失败：输出 `xui_subscription_up{sid}=0`，其余指标按实现策略：
  - 建议：仅输出 `up` + optional refresh timestamp/duration；raw/derived 不输出，避免误导

## 10. 测试计划（最小集）

- `internal/parse`：
  - 正常 HTML 可解析
  - 缺 template / 缺字段 / 非数字字段 → error

- `internal/validate`（若拆出）：
  - quota==0 → fail
  - expire<=0 → fail

- `internal/compute`：
  - remaining 允许负数
  - expired / daily_budget 分支覆盖

## 11. 实现 Checklist（按编码顺序）

1. 初始化 Go module（`go mod init`）
2. 引入 Prometheus client_golang（`prometheus` + `promhttp`）
3. 实现 `ParseTargetsFromEnv()`：空则退出
4. 实现 HTTP fetch（超时 + 非 200 处理）
5. 实现 HTML 解析 `template#subscription-data` 属性
6. 实现校验与派生指标计算
7. 实现 store（快照替换）
8. 实现自定义 Prometheus Collector（从 store 输出所有 sid 指标）
9. 实现 refresh loop：启动时先跑一次 refresh，然后 ticker 每 60s 跑一次
10. 补齐单测（parser/compute 至少覆盖异常与边界）
