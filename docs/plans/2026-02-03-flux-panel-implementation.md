# Flux-Panel 适配器实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 xui-exporter 添加 flux-panel 采集类型支持，统一配置格式为 JSON 文件。

**Architecture:** 重构 config 包支持 JSON 配置文件，抽象 fetch/parse 层支持多种目标类型（xui/flux），新增 flux-panel API 解析逻辑。

**Tech Stack:** Go 1.24, encoding/json, net/http, time

---

## Task 1: 重构 config 包 - 定义配置结构

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: 写失败测试 - JSON 配置解析**

创建 `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_FromFile(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configContent := `{
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
      "token": "test-token-123"
    }
  ]
}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if len(cfg.Targets) != 2 {
		t.Fatalf("Expected 2 targets, got %d", len(cfg.Targets))
	}

	// 验证第一个 target (xui)
	if cfg.Targets[0].Name != "my-xui-sub1" {
		t.Errorf("Expected name 'my-xui-sub1', got '%s'", cfg.Targets[0].Name)
	}
	if cfg.Targets[0].Type != "xui" {
		t.Errorf("Expected type 'xui', got '%s'", cfg.Targets[0].Type)
	}
	if cfg.Targets[0].URL != "http://example.com/sub/sid1" {
		t.Errorf("Expected URL 'http://example.com/sub/sid1', got '%s'", cfg.Targets[0].URL)
	}

	// 验证第二个 target (flux)
	if cfg.Targets[1].Name != "flux-metholx" {
		t.Errorf("Expected name 'flux-metholx', got '%s'", cfg.Targets[1].Name)
	}
	if cfg.Targets[1].Type != "flux" {
		t.Errorf("Expected type 'flux', got '%s'", cfg.Targets[1].Type)
	}
	if cfg.Targets[1].Token != "test-token-123" {
		t.Errorf("Expected token 'test-token-123', got '%s'", cfg.Targets[1].Token)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.json")
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestLoadConfig_EmptyTargets(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"targets": []}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for empty targets, got nil")
	}
}

func TestLoadConfig_InvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configContent := `{
  "targets": [
    {
      "name": "test",
      "type": "unknown",
      "url": "http://example.com"
    }
  ]
}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for invalid type, got nil")
	}
}

func TestLoadConfig_FluxMissingToken(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configContent := `{
  "targets": [
    {
      "name": "test",
      "type": "flux",
      "url": "http://example.com"
    }
  ]
}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("Expected error for flux without token, got nil")
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test -v ./internal/config -run TestLoadConfig`
Expected: FAIL - LoadConfig 函数不存在

**Step 3: 实现配置加载**

修改 `internal/config/config.go`:

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Target 表示一个采集目标
type Target struct {
	Name  string `json:"name"`
	Type  string `json:"type"`  // "xui" 或 "flux"
	URL   string `json:"url"`
	Token string `json:"token,omitempty"` // 仅 flux 类型需要
}

// Config 表示完整的配置
type Config struct {
	Targets []Target `json:"targets"`
}

// LoadConfig 从指定路径加载 JSON 配置文件
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("config must have at least one target")
	}

	// 验证每个 target
	for i, t := range cfg.Targets {
		if t.Name == "" {
			return nil, fmt.Errorf("target[%d]: name is required", i)
		}
		if t.URL == "" {
			return nil, fmt.Errorf("target[%d]: url is required", i)
		}
		if t.Type != "xui" && t.Type != "flux" {
			return nil, fmt.Errorf("target[%d]: type must be 'xui' or 'flux', got '%s'", i, t.Type)
		}
		if t.Type == "flux" && t.Token == "" {
			return nil, fmt.Errorf("target[%d]: token is required for flux type", i)
		}
	}

	return &cfg, nil
}
```

**Step 4: 运行测试验证通过**

Run: `go test -v ./internal/config -run TestLoadConfig`
Expected: PASS

**Step 5: 提交**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(config): add JSON config file support

Replace environment variable config with JSON file loading.
Supports xui and flux target types with validation.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: 添加流量重置时间计算

**Files:**
- Create: `internal/parse/reset_time.go`
- Test: `internal/parse/reset_time_test.go`

**Step 1: 写失败测试 - 重置时间计算**

创建 `internal/parse/reset_time_test.go`:

```go
package parse

import (
	"testing"
	"time"
)

func TestCalcNextResetTime_NoReset(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(0, now)
	if result != 0 {
		t.Errorf("Expected 0 for flowResetTime=0, got %d", result)
	}
}

func TestCalcNextResetTime_BeforeResetDay(t *testing.T) {
	// 今天 1月1日，重置日是2号 → 下次重置是 1月2日
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func TestCalcNextResetTime_OnResetDay(t *testing.T) {
	// 今天 1月2日，重置日是2号 → 下次重置是 2月2日
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func TestCalcNextResetTime_AfterResetDay(t *testing.T) {
	// 今天 1月15日，重置日是2号 → 下次重置是 2月2日
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func TestCalcNextResetTime_MonthEndBoundary(t *testing.T) {
	// 2月没有31号，应该回退到2月28日（非闰年）
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(31, now)

	// 1月31日在1月15日之后，所以应该是1月31日
	expected := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (Jan 31), got %d", expected, result)
	}
}

func TestCalcNextResetTime_FebruaryLeapYear(t *testing.T) {
	// 闰年2月，重置日31号 → 回退到2月29日
	now := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC) // 2024是闰年
	result := CalcNextResetTime(31, now)

	expected := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (Feb 29), got %d", expected, result)
	}
}

func TestCalcNextResetTime_FebruaryNonLeapYear(t *testing.T) {
	// 非闰年2月，重置日31号 → 回退到2月28日
	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC) // 2025不是闰年
	result := CalcNextResetTime(31, now)

	expected := time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (Feb 28), got %d", expected, result)
	}
}

func TestCalcNextResetTime_YearCrossover(t *testing.T) {
	// 12月15日，重置日是2号 → 下次重置是次年1月2日
	now := time.Date(2025, 12, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (2026-01-02), got %d", expected, result)
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test -v ./internal/parse -run TestCalcNextResetTime`
Expected: FAIL - CalcNextResetTime 函数不存在

**Step 3: 实现重置时间计算**

创建 `internal/parse/reset_time.go`:

```go
package parse

import "time"

// CalcNextResetTime 计算下一次流量重置时间的 Unix 时间戳
// flowResetTime: 0 表示不重置，1-31 表示每月第 N 天重置
// now: 当前时间
// 返回: Unix 时间戳（秒），flowResetTime=0 时返回 0
func CalcNextResetTime(flowResetTime int, now time.Time) int64 {
	if flowResetTime == 0 {
		return 0
	}

	year, month, day := now.Date()
	loc := now.Location()

	// 本月的重置日
	resetDay := flowResetTime
	lastDayOfMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if resetDay > lastDayOfMonth {
		resetDay = lastDayOfMonth
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

**Step 4: 运行测试验证通过**

Run: `go test -v ./internal/parse -run TestCalcNextResetTime`
Expected: PASS

**Step 5: 提交**

```bash
git add internal/parse/reset_time.go internal/parse/reset_time_test.go
git commit -m "$(cat <<'EOF'
feat(parse): add flow reset time calculation

Calculate next monthly reset timestamp based on flowResetTime field.
Handles month-end boundary cases and year crossover.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: 实现 flux-panel 数据解析

**Files:**
- Create: `internal/parse/flux.go`
- Test: `internal/parse/flux_test.go`

**Step 1: 写失败测试 - flux API 响应解析**

创建 `internal/parse/flux_test.go`:

```go
package parse

import (
	"testing"
	"time"
)

func TestParseFluxResponse_Success(t *testing.T) {
	jsonData := `{
  "code": 0,
  "message": "操作成功",
  "data": {
    "userInfo": {
      "flow": 100,
      "inFlow": 53687091200,
      "outFlow": 10737418240,
      "flowResetTime": 2
    }
  }
}`

	// 使用固定时间进行测试：2025年1月1日
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	result, err := ParseFluxResponse([]byte(jsonData), "test-sid", now)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if result.SID != "test-sid" {
		t.Errorf("Expected SID 'test-sid', got '%s'", result.SID)
	}

	// inFlow 对应 DownloadByte
	if result.DownloadByte != 53687091200 {
		t.Errorf("Expected DownloadByte 53687091200, got %d", result.DownloadByte)
	}

	// outFlow 对应 UploadByte
	if result.UploadByte != 10737418240 {
		t.Errorf("Expected UploadByte 10737418240, got %d", result.UploadByte)
	}

	// flow (GB) * 1024^3 = TotalByte
	expectedTotal := int64(100 * 1024 * 1024 * 1024)
	if result.TotalByte != expectedTotal {
		t.Errorf("Expected TotalByte %d, got %d", expectedTotal, result.TotalByte)
	}

	// flowResetTime=2，今天1月1日 → 下次重置1月2日
	expectedExpire := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result.Expire != expectedExpire {
		t.Errorf("Expected Expire %d, got %d", expectedExpire, result.Expire)
	}
}

func TestParseFluxResponse_NoReset(t *testing.T) {
	jsonData := `{
  "code": 0,
  "message": "操作成功",
  "data": {
    "userInfo": {
      "flow": 100,
      "inFlow": 0,
      "outFlow": 0,
      "flowResetTime": 0
    }
  }
}`

	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	result, err := ParseFluxResponse([]byte(jsonData), "test-sid", now)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	// flowResetTime=0 表示不重置，Expire 应该为 0
	if result.Expire != 0 {
		t.Errorf("Expected Expire 0 for no reset, got %d", result.Expire)
	}
}

func TestParseFluxResponse_APIError(t *testing.T) {
	jsonData := `{
  "code": 1,
  "message": "token无效"
}`

	now := time.Now()
	_, err := ParseFluxResponse([]byte(jsonData), "test-sid", now)
	if err == nil {
		t.Fatal("Expected error for API error response, got nil")
	}
}

func TestParseFluxResponse_InvalidJSON(t *testing.T) {
	now := time.Now()
	_, err := ParseFluxResponse([]byte("invalid json"), "test-sid", now)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestParseFluxResponse_MissingUserInfo(t *testing.T) {
	jsonData := `{
  "code": 0,
  "message": "操作成功",
  "data": {}
}`

	now := time.Now()
	_, err := ParseFluxResponse([]byte(jsonData), "test-sid", now)
	if err == nil {
		t.Fatal("Expected error for missing userInfo, got nil")
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test -v ./internal/parse -run TestParseFluxResponse`
Expected: FAIL - ParseFluxResponse 函数不存在

**Step 3: 实现 flux 响应解析**

创建 `internal/parse/flux.go`:

```go
package parse

import (
	"encoding/json"
	"fmt"
	"time"
)

// FluxAPIResponse 表示 flux-panel API 响应结构
type FluxAPIResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		UserInfo *FluxUserInfo `json:"userInfo"`
	} `json:"data"`
}

// FluxUserInfo 表示用户信息
type FluxUserInfo struct {
	Flow          float64 `json:"flow"`          // 总流量 (GB)
	InFlow        int64   `json:"inFlow"`        // 下载流量 (Bytes)
	OutFlow       int64   `json:"outFlow"`       // 上传流量 (Bytes)
	FlowResetTime int     `json:"flowResetTime"` // 重置日 (0=不重置, 1-31=每月第N天)
}

// ParseFluxResponse 解析 flux-panel API 响应并转换为 ParsedSubscription
// jsonData: API 响应的 JSON 数据
// sid: 配置文件中指定的 name，用作 SID
// now: 当前时间，用于计算下次重置时间
func ParseFluxResponse(jsonData []byte, sid string, now time.Time) (ParsedSubscription, error) {
	var resp FluxAPIResponse
	if err := json.Unmarshal(jsonData, &resp); err != nil {
		return ParsedSubscription{}, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if resp.Code != 0 {
		return ParsedSubscription{}, fmt.Errorf("API error: code=%d, message=%s", resp.Code, resp.Message)
	}

	if resp.Data.UserInfo == nil {
		return ParsedSubscription{}, fmt.Errorf("userInfo is missing in response")
	}

	userInfo := resp.Data.UserInfo

	// flow 单位是 GB，转换为 Bytes
	totalBytes := int64(userInfo.Flow * 1024 * 1024 * 1024)

	// 计算下次重置时间
	expire := CalcNextResetTime(userInfo.FlowResetTime, now)

	return ParsedSubscription{
		SID:          sid,
		DownloadByte: userInfo.InFlow,
		UploadByte:   userInfo.OutFlow,
		TotalByte:    totalBytes,
		Expire:       expire,
	}, nil
}
```

**Step 4: 运行测试验证通过**

Run: `go test -v ./internal/parse -run TestParseFluxResponse`
Expected: PASS

**Step 5: 提交**

```bash
git add internal/parse/flux.go internal/parse/flux_test.go
git commit -m "$(cat <<'EOF'
feat(parse): add flux-panel API response parser

Parse flux-panel JSON response and map to ParsedSubscription.
Converts flow (GB) to bytes, uses flowResetTime for Expire.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: 实现 flux-panel HTTP 请求

**Files:**
- Create: `internal/fetch/flux.go`
- Test: `internal/fetch/flux_test.go`

**Step 1: 写失败测试 - flux API 请求**

创建 `internal/fetch/flux_test.go`:

```go
package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetFluxAPI_Success(t *testing.T) {
	responseJSON := `{"code": 0, "message": "success", "data": {}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// 验证 Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "test-token" {
			t.Errorf("Expected Authorization 'test-token', got '%s'", auth)
		}

		// 验证 Content-Type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	ctx := context.Background()
	body, err := GetFluxAPI(ctx, server.URL, "test-token")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if string(body) != responseJSON {
		t.Errorf("Expected response '%s', got '%s'", responseJSON, string(body))
	}
}

func TestGetFluxAPI_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := GetFluxAPI(ctx, server.URL, "bad-token")
	if err == nil {
		t.Fatal("Expected error for 401 response, got nil")
	}
}

func TestGetFluxAPI_NetworkError(t *testing.T) {
	ctx := context.Background()
	_, err := GetFluxAPI(ctx, "http://localhost:1", "token")
	if err == nil {
		t.Fatal("Expected error for network failure, got nil")
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test -v ./internal/fetch -run TestGetFluxAPI`
Expected: FAIL - GetFluxAPI 函数不存在

**Step 3: 实现 flux API 请求**

创建 `internal/fetch/flux.go`:

```go
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// GetFluxAPI 发送 POST 请求到 flux-panel API 并返回响应体
// url: API endpoint URL
// token: Authorization header 的值
func GetFluxAPI(ctx context.Context, url string, token string) ([]byte, error) {
	client := &http.Client{
		Timeout: DefaultTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d (expected 200)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}
```

**Step 4: 运行测试验证通过**

Run: `go test -v ./internal/fetch -run TestGetFluxAPI`
Expected: PASS

**Step 5: 提交**

```bash
git add internal/fetch/flux.go internal/fetch/flux_test.go
git commit -m "$(cat <<'EOF'
feat(fetch): add flux-panel API client

POST request with Authorization header for flux-panel API.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: 重构 main.go 支持多目标类型

**Files:**
- Modify: `cmd/xui-exporter/main.go`

**Step 1: 阅读现有 main.go 结构**

已在前面阅读。需要修改：
1. 添加 `-config` 命令行参数
2. 使用新的 `config.LoadConfig()` 加载配置
3. 根据 target.Type 调用不同的 fetch/parse 逻辑
4. 使用 target.Name 作为 SID

**Step 2: 修改 main.go**

修改 `cmd/xui-exporter/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
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
	listenAddr         = ":9100"
	metricsPath        = "/metrics"
	refreshInterval    = 60 * time.Second
	fetchConcurrency   = 4
	defaultConfigPath  = "/etc/xui-exporter/config.json"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "", "Path to config file (default: /etc/xui-exporter/config.json)")
	flag.Parse()

	// 确定配置文件路径
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = defaultConfigPath
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		log.Fatalf("Config file not found: %s", cfgPath)
	}

	// 加载配置
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Loaded %d target(s) from %s", len(cfg.Targets), cfgPath)

	// Initialize store
	st := store.New()

	// Create and register custom collector
	collector := metrics.NewCollector(st)
	prometheus.MustRegister(collector)

	log.Printf("Registered Prometheus collector")

	// Perform initial refresh before starting server
	log.Printf("Performing initial refresh...")
	refresh(cfg.Targets, st)

	// Start refresh loop in background
	go refreshLoop(cfg.Targets, st, refreshInterval)

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
func refreshLoop(targets []config.Target, st *store.Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		refresh(targets, st)
	}
}

// refresh fetches all targets concurrently and updates the store
func refresh(targets []config.Target, st *store.Store) {
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
		go func(t config.Target) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			fetchAndProcess(t, refreshStart, &newSnapshot, &mu)
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
func fetchAndProcess(target config.Target, refreshStart time.Time, snapshot *map[string]compute.SubscriptionMetrics, mu *sync.Mutex) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var parsed parse.ParsedSubscription
	var err error

	switch target.Type {
	case "xui":
		parsed, err = fetchXUI(ctx, target)
	case "flux":
		parsed, err = fetchFlux(ctx, target)
	default:
		log.Printf("Unknown target type '%s' for %s", target.Type, target.Name)
		return
	}

	if err != nil {
		log.Printf("Failed to fetch %s (%s): %v", target.Name, target.Type, err)
		mu.Lock()
		(*snapshot)[target.Name] = compute.NewFailedMetrics(target.Name, refreshStart)
		mu.Unlock()
		return
	}

	// Validate quota (quota=0 is treated as failure)
	if parsed.TotalByte == 0 {
		log.Printf("Validation failed for %s: quota is 0 (not allowed)", target.Name)
		mu.Lock()
		(*snapshot)[target.Name] = compute.NewFailedMetrics(target.Name, refreshStart)
		mu.Unlock()
		return
	}

	// Compute metrics
	now := time.Now()
	metricsData := compute.Compute(now, parsed, refreshStart)

	// Add to snapshot
	mu.Lock()
	(*snapshot)[target.Name] = metricsData
	mu.Unlock()

	log.Printf("Successfully processed %s (%s)", target.Name, target.Type)
}

// fetchXUI fetches and parses xui subscription HTML
func fetchXUI(ctx context.Context, target config.Target) (parse.ParsedSubscription, error) {
	htmlBytes, err := fetch.GetHTML(ctx, target.URL)
	if err != nil {
		return parse.ParsedSubscription{}, err
	}

	parsed, err := parse.ParseSubscription(htmlBytes)
	if err != nil {
		return parse.ParsedSubscription{}, err
	}

	// 使用配置文件中的 name 覆盖 SID
	parsed.SID = target.Name
	return parsed, nil
}

// fetchFlux fetches and parses flux-panel API response
func fetchFlux(ctx context.Context, target config.Target) (parse.ParsedSubscription, error) {
	jsonBytes, err := fetch.GetFluxAPI(ctx, target.URL, target.Token)
	if err != nil {
		return parse.ParsedSubscription{}, err
	}

	return parse.ParseFluxResponse(jsonBytes, target.Name, time.Now())
}
```

**Step 3: 运行所有测试**

Run: `go build ./... && go test ./...`
Expected: PASS

**Step 4: 提交**

```bash
git add cmd/xui-exporter/main.go
git commit -m "$(cat <<'EOF'
refactor(main): support multi-target types with JSON config

- Add -config CLI flag for config file path
- Load targets from JSON config file
- Route to xui or flux fetch/parse based on target type
- Use target.Name as SID for all targets

BREAKING CHANGE: XUI_EXPORTER_TARGETS env var no longer supported.
Use JSON config file instead.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: 删除旧的环境变量配置逻辑

**Files:**
- Delete: `internal/config/config.go` 中的 `ParseTargetsFromEnv` 函数（如果还需要保留文件的话）

**Step 1: 确认旧函数不再被使用**

检查是否还有其他地方引用 `ParseTargetsFromEnv`。

Run: `grep -r "ParseTargetsFromEnv" --include="*.go" .`
Expected: 只在 config.go 中定义，main.go 不再使用

**Step 2: 删除旧函数**

由于 Task 1 已经完全重写了 config.go，旧的 `ParseTargetsFromEnv` 函数已被移除。此任务可跳过。

---

## Task 7: 更新 README

**Files:**
- Modify: `README.md`

**Step 1: 更新使用说明**

更新 README.md 中的配置和运行说明，移除环境变量相关内容，添加 JSON 配置文件说明。

**Step 2: 提交**

```bash
git add README.md
git commit -m "$(cat <<'EOF'
docs: update README for JSON config and flux-panel support

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: 端到端测试

**Step 1: 创建测试配置文件**

```bash
cat > /tmp/test-config.json << 'EOF'
{
  "targets": [
    {
      "name": "test-xui",
      "type": "xui",
      "url": "http://example.com/sub/test"
    }
  ]
}
EOF
```

**Step 2: 构建并运行**

```bash
go build -o xui-exporter ./cmd/xui-exporter
./xui-exporter -config /tmp/test-config.json
```

Expected: 启动成功，显示加载了 1 个 target

**Step 3: 验证 metrics 端点**

```bash
curl http://localhost:9100/metrics
```

Expected: 返回 Prometheus 指标格式数据

---

## 验证清单

- [ ] `go test ./...` 全部通过
- [ ] `go build ./...` 无错误
- [ ] `-config` 参数正确工作
- [ ] 默认配置路径 `/etc/xui-exporter/config.json` 正确
- [ ] xui 类型 target 正常采集
- [ ] flux 类型 target 正常采集
- [ ] 流量重置时间计算正确
- [ ] Prometheus 指标正确暴露
