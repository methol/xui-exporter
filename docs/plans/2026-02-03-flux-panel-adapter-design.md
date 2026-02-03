# Flux-Panel 适配器设计

## 概述

为 xui-exporter 添加 flux-panel 采集类型支持，统一配置格式为 JSON 文件。

## 配置文件格式

**路径加载顺序**：
1. 命令行 `-config /path/to/config.json`
2. 默认 `/etc/xui-exporter/config.json`

**配置结构**：

```json
{
  "targets": [
    {
      "name": "my-xui-sub1",
      "type": "xui",
      "url": "http://example.com/sub/sid1"
    },
    {
      "name": "flux-metholx",
      "type": "flux",
      "url": "https://ix.321cmo.com/api/v1/user/package",
      "token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIbWFjU0hBMjU2In0..."
    }
  ]
}
```

**字段说明**：
- `name`: 唯一标识符，用于 Prometheus 指标的 `sid` label
- `type`: 采集类型，`xui` 或 `flux`
- `url`: 目标 URL
- `token`: （仅 flux 类型）Authorization header 的值

## 数据映射

**flux-panel API 响应到 SubscriptionMetrics 的映射**：

| flux-panel 字段 | 计算方式 | 映射到 |
|----------------|---------|--------|
| 配置中的 `name` | 直接使用 | `SID` |
| `userInfo.inFlow` | 直接使用（单位 B） | `DownloadByte` |
| `userInfo.outFlow` | 直接使用（单位 B） | `UploadByte` |
| `userInfo.flow` | `flow * 1024^3`（GB → B） | `TotalByte` |
| `userInfo.flowResetTime` + 当前时间 | 计算下次重置日期的时间戳（秒） | `Expire` |

## 流量重置时间计算

**`flowResetTime` 的含义**：
- 值为 `0`：不重置，返回 0
- 值为 `1-31`：每月第 N 天重置

**计算逻辑**：

```go
func calcNextResetTime(flowResetTime int, now time.Time) int64 {
    if flowResetTime == 0 {
        return 0  // 不重置，返回 0
    }

    year, month, day := now.Date()
    loc := now.Location()

    // 本月的重置日
    resetDay := flowResetTime
    lastDayOfMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
    if resetDay > lastDayOfMonth {
        resetDay = lastDayOfMonth  // 月末边界处理
    }

    if day < resetDay {
        // 今天在重置日之前，下次重置是本月
        return time.Date(year, month, resetDay, 0, 0, 0, 0, loc).Unix()
    }

    // 今天 >= 重置日，下次重置是下个月
    nextMonth := month + 1
    nextYear := year
    if nextMonth > 12 {
        nextMonth = 1
        nextYear++
    }

    lastDayOfNextMonth := time.Date(nextYear, nextMonth+1, 0, 0, 0, 0, 0, loc).Day()
    resetDay = flowResetTime
    if resetDay > lastDayOfNextMonth {
        resetDay = lastDayOfNextMonth
    }

    return time.Date(nextYear, nextMonth, resetDay, 0, 0, 0, 0, loc).Unix()
}
```

**示例**（假设 `flowResetTime = 2`）：
- 今天 1月1日 → 下次重置 1月2日 00:00
- 今天 1月2日 → 下次重置 2月2日 00:00
- 今天 1月15日 → 下次重置 2月2日 00:00

## 架构变更

**需要修改的模块**：

| 模块 | 变更内容 |
|-----|---------|
| `internal/config` | 重写配置加载逻辑：读取 JSON 文件，解析 targets 数组 |
| `internal/fetch` | 抽象为接口，支持两种采集方式：HTML 抓取（xui）和 API 调用（flux） |
| `internal/parse` | 拆分为 `parse/xui.go` 和 `parse/flux.go`，各自解析对应格式 |
| `cmd/xui-exporter/main.go` | 添加 `-config` 命令行参数解析，根据 target type 调用不同的采集逻辑 |

**新增文件**：
- `internal/parse/flux.go` - flux-panel API 响应解析
- `internal/fetch/flux.go` - flux-panel API 请求（带 Authorization header）

**保持不变的模块**：
- `internal/compute` - 计算逻辑通用
- `internal/store` - 存储逻辑通用
- `internal/metrics` - Prometheus 指标暴露通用

## 使用方式

**命令行**：
```bash
./xui-exporter -config /path/to/config.json
```

**Docker**：
```bash
docker run -p 9100:9100 \
  -v /path/to/config.json:/etc/xui-exporter/config.json \
  xui-exporter
```

## 错误处理

- 配置文件不存在或格式错误：启动时报错退出
- 单个 target 采集失败：记录日志，该 target 的 `Up` 指标为 0，不影响其他 target
- flux API 返回 `code != 0`：视为采集失败

## 向后兼容

- 移除 `XUI_EXPORTER_TARGETS` 环境变量支持
- README 需要更新使用说明
