package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Target 表示一个采集目标
type Target struct {
	Name  string `json:"name"`
	Type  string `json:"type"`            // "xui" 或 "flux"
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
