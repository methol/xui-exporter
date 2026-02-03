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
