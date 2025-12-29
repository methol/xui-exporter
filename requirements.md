# xui-exporter Requirements

## 1. Goal / 背景

开发一个 Prometheus Exporter，用于监控 x-ui / 3x-ui 面板生成的「订阅地址」对应账号的流量使用情况，并在 Grafana 中制作 Dashboard 展示（按天用量、剩余、到期、每日可用额度/趋势估算等）。

数据来源：订阅页面 HTML 中的如下节点属性（示例来自 `/sub/<sid>` 页面）：

```html
<template id="subscription-data"
  data-sid="uk2jf33cdnzjn2dg"
  data-sub-url="http://example.com/sub/uk2jf33cdnzjn2dg"
  data-downloadbyte="6150124543"
  data-uploadbyte="267143927"
  data-totalbyte="536870912000"
  data-expire="1769184000"
></template>
```

## 2. Inputs / Data Extraction

### 2.1 Subscription endpoints

- 订阅链接可能有多个
- 通过环境变量 `XUI_EXPORTER_TARGETS` 配置（详见 §4）
- 订阅页面无需鉴权

Exporter 会对每个订阅链接发起 HTTP(S) GET 请求，获取 HTML 内容。

### 2.2 Parsing rules

从 HTML 中定位 `template#subscription-data` 元素，提取以下字段：

- `data-sid`：订阅唯一标识（全局唯一）（必须）
- `data-expire`：过期时间（Unix epoch，**秒级**）（必须）
- `data-downloadbyte`：下载流量（字节）（必须）
- `data-uploadbyte`：上传流量（字节）（必须）
- `data-totalbyte`：总配额（字节）（必须）

解析时优先使用 `data-xxxbyte` 的数值字段，不使用 `data-download="5.73GB"` 等字符串字段。

### 2.3 Data validation / 异常处理

- 所有 `*byte` 字段必须能解析为非负整数
- `data-expire` 必须能解析为正整数（epoch seconds）
- `data-totalbyte` 必须为正整数（>0）
  - 如果出现 `data-totalbyte=0`：按异常处理（该 sid `up=0`，并记录日志），因为本项目不允许“无限制”配额

若 `template#subscription-data` 不存在或关键字段缺失/不可解析/不符合上述校验：

- 该订阅抓取失败
- Exporter 在自身日志中记录错误
- Prometheus 指标中体现为该订阅的 `xui_subscription_up{sid}=0`

## 3. Prometheus Metrics

### 3.1 Labels

- 所有指标仅使用 `sid` label（不包含 `target` 等维度）

### 3.2 Raw per-subscription metrics (from x-ui)

每个订阅（由 `sid` 识别）暴露以下指标（Gauge）：

- `xui_subscription_download_bytes{sid}`：已下载字节数（来自 `data-downloadbyte`）
- `xui_subscription_upload_bytes{sid}`：已上传字节数（来自 `data-uploadbyte`）
- `xui_subscription_quota_bytes{sid}`：总配额字节数（来自 `data-totalbyte`）
- `xui_subscription_expire_timestamp_seconds{sid}`：到期时间（epoch seconds，来自 `data-expire`）

### 3.3 Derived metrics (exporter-computed, for simpler Grafana)

为了减少 Grafana/PromQL 表达式复杂度，Exporter 额外暴露以下派生指标（Gauge）：

- `xui_subscription_used_bytes{sid}`
  - `download_bytes + upload_bytes`
- `xui_subscription_remaining_bytes{sid}`
  - `quota_bytes - used_bytes`
- `xui_subscription_used_ratio{sid}`
  - `used_bytes / quota_bytes`，范围建议 0..1
- `xui_subscription_remaining_ratio{sid}`
  - `remaining_bytes / quota_bytes`，范围建议 0..1
- `xui_subscription_seconds_until_expire{sid}`
  - `expire_timestamp_seconds - now`（now 为 Exporter 计算时的 Unix time 秒）
- `xui_subscription_days_until_expire{sid}`
  - `seconds_until_expire / 86400`
- `xui_subscription_expired{sid}`
  - 1 表示已过期（`seconds_until_expire <= 0`），否则 0
- `xui_subscription_daily_budget_bytes{sid}`
  - “从现在到到期，平均每天可用额度”
  - 当 `days_until_expire > 0` 时：`remaining_bytes / days_until_expire`
  - 当 `days_until_expire <= 0` 时：输出 0（并可结合 `expired` 判断）

> 说明：下载/上传/已用来自面板累计值，常见用 Gauge 暴露；Grafana 侧做“每日使用量”推荐对 `xui_subscription_used_bytes` 使用 `increase([1d])`。

### 3.4 Scrape health / refresh metrics

- `xui_subscription_up{sid}`
  - 1：成功抓取并解析且通过校验
  - 0：抓取失败/解析失败/校验失败（例如 quota=0）

可选（便于排障与缓存观测）：

- `xui_subscription_last_refresh_timestamp_seconds{sid}`
- `xui_subscription_refresh_duration_seconds{sid}`

## 4. Configuration

仅保留一个环境变量：

- `XUI_EXPORTER_TARGETS`（必填）
  - 订阅链接列表（多个）
  - 格式建议：逗号分隔（并 trim 空白）
  - 示例：
    - `XUI_EXPORTER_TARGETS="http://example.com/sub/uk2jf33cdnzjn2dg,http://example.com/sub/abcd"`

默认行为（不提供额外配置项）：

- Exporter HTTP server 监听 `:9100`
- 暴露指标路径 `/metrics`
- 后台刷新间隔 60 秒

## 5. Runtime behavior

### 5.1 Collection model

采用 **Background refresh + cached metrics**：

- 后台定时抓取所有订阅链接并缓存最新数据
- Prometheus 抓取 `/metrics` 时只读取缓存，避免抓取超时

### 5.2 Concurrency

- 多个订阅链接并发抓取
- 单个订阅失败不影响其他订阅

### 5.3 Logging

- 记录每个 sid 的抓取失败原因（网络错误、HTTP 非 200、解析失败、字段缺失、quota=0 等）

## 6. Grafana Dashboard expectations

目标：做出“漂亮且有用”的可视化，包括：

- 按天展示每天使用情况
- 当前总使用/剩余/到期
- 从现在到到期的“每日可用额度”（daily budget）
- 基于近期趋势的“预计还能用多少天/每天使用趋势”

### 6.1 推荐的 Grafana 变量

- `sid` 变量：
  - Query：`label_values(xui_subscription_up, sid)`

### 6.2 PromQL 示例（尽量使用派生指标，表达式更短）

- **已用总量（MiB）**
  - `xui_subscription_used_bytes{sid=~"$sid"} / 1024 / 1024`

- **每日使用量（MiB/day，柱状图，按天 step=1d）**
  - `increase(xui_subscription_used_bytes{sid=~"$sid"}[1d]) / 1024 / 1024`

- **近 7 天平均每日使用（MiB/day）**
  - `(increase(xui_subscription_used_bytes{sid=~"$sid"}[7d]) / 7) / 1024 / 1024`

- **剩余配额（MiB）**
  - `xui_subscription_remaining_bytes{sid=~"$sid"} / 1024 / 1024`

- **已用百分比（%）**
  - `xui_subscription_used_ratio{sid=~"$sid"} * 100`

- **距离到期剩余天数（days）**
  - `xui_subscription_days_until_expire{sid=~"$sid"}`

- **从现在到到期的“每日可用额度”（MiB/day）**
  - `xui_subscription_daily_budget_bytes{sid=~"$sid"} / 1024 / 1024`

- **按近期趋势估算“还能用多少天”（days，7d）**
  - `xui_subscription_remaining_bytes{sid=~"$sid"} / ((increase(xui_subscription_used_bytes{sid=~"$sid"}[7d]) / 7))`

> Grafana 单位建议直接设置为 `bytes (IEC)` / `mebibytes (IEC)`，避免在 PromQL 中频繁手写换算。

## 7. Confirmed facts (from user)

- `data-expire` 始终为秒级 epoch
- `sid` 全局唯一，不会跨面板/host 重复
- 订阅页面不需要鉴权
- Prometheus 抓取频率：1 分钟 1 次；Exporter 采用后台刷新+缓存模式
- 不采集字符串字段（只用 byte/expire 等数值字段）
- `quota=0` 视为异常：本项目不允许无限制配额
