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
