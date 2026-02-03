# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

xui-exporter 是一个 Prometheus exporter，用于抓取 x-ui/3x-ui 订阅页面的流量使用和过期数据。运行端口 9100，每 60 秒刷新一次指标。

## 构建与运行

```bash
# 构建
go build -o xui-exporter ./cmd/xui-exporter

# 运行（需要设置 XUI_EXPORTER_TARGETS 环境变量）
XUI_EXPORTER_TARGETS="http://example.com/sub/sid1,http://example.com/sub/sid2" ./xui-exporter

# Docker 构建
docker build -t xui-exporter .

# Docker 运行
docker run -p 9100:9100 -e XUI_EXPORTER_TARGETS="..." xui-exporter
```

## 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test -v ./internal/parse

# 运行单个测试
go test -v ./internal/parse -run TestParseSubscription_Success

# 带覆盖率
go test -cover ./...
```

## 架构

数据按顺序流经以下内部包：

1. **config** - 解析 `XUI_EXPORTER_TARGETS` 环境变量（逗号分隔的 URL）
2. **fetch** - HTTP 客户端，从订阅 URL 获取 HTML
3. **parse** - 使用 `golang.org/x/net/html` 从 `<template id="subscription-data">` 提取数据属性
4. **compute** - 计算派生指标（剩余字节、使用率、每日预算）
5. **store** - 使用 `sync.RWMutex` 的线程安全内存快照
6. **metrics** - 自定义 Prometheus `Collector`，从 store 读取数据

主循环（`cmd/xui-exporter/main.go`）并发抓取所有目标（最多 4 个并行），然后原子替换快照。

## 关键约定

- 使用 `golang.org/x/net/html` 解析 HTML（禁用正则）
- Prometheus 指标命名规范：`xui_subscription_<name>_<unit>`
- 错误包装：`fmt.Errorf("上下文: %w", err)`
