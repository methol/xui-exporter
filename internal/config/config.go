package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
	_ "time/tzdata"
)

const (
	DefaultRefreshIntervalSeconds = 60
	DefaultSSHPort                = 22
	DefaultLookbackDays           = 62
	DefaultConnectTimeoutSeconds  = 10
	DefaultCommandTimeoutSeconds  = 15
	DefaultMaxDataAgeSeconds      = 900
	MaxLookbackDays               = 400
)

var interfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.:+-]{1,64}$`)

// Target 表示一个采集目标。
type Target struct {
	Name string `json:"name"`
	Type string `json:"type"`

	URL   string `json:"url,omitempty"`
	Token string `json:"token,omitempty"`

	VNStatSSH *VNStatSSHConfig `json:"vnstat_ssh,omitempty"`
}

// VNStatSSHConfig 表示通过 SSH 查询远程 vnStat 的配置。
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

// Config 表示完整的配置。
type Config struct {
	RefreshIntervalSeconds int      `json:"refresh_interval_seconds,omitempty"`
	Targets                []Target `json:"targets"`
}

// LoadConfig 从指定路径加载并验证 JSON 配置文件。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if cfg.RefreshIntervalSeconds == 0 {
		cfg.RefreshIntervalSeconds = DefaultRefreshIntervalSeconds
	}
	if cfg.RefreshIntervalSeconds < 0 {
		return nil, fmt.Errorf("refresh_interval_seconds must be positive")
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("config must have at least one target")
	}

	seenNames := make(map[string]struct{}, len(cfg.Targets))
	for i := range cfg.Targets {
		target := &cfg.Targets[i]
		if target.Name == "" {
			return nil, fmt.Errorf("target[%d]: name is required", i)
		}
		if _, exists := seenNames[target.Name]; exists {
			return nil, fmt.Errorf("target[%d]: duplicate name %q", i, target.Name)
		}
		seenNames[target.Name] = struct{}{}

		switch target.Type {
		case "xui":
			if target.URL == "" {
				return nil, fmt.Errorf("target[%d]: url is required for xui type", i)
			}
		case "flux":
			if target.URL == "" {
				return nil, fmt.Errorf("target[%d]: url is required for flux type", i)
			}
			if target.Token == "" {
				return nil, fmt.Errorf("target[%d]: token is required for flux type", i)
			}
		case "vnstat_ssh":
			if err := validateVNStatSSH(i, target); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("target[%d]: type must be 'xui', 'flux', or 'vnstat_ssh', got %q", i, target.Type)
		}
	}

	return &cfg, nil
}

func validateVNStatSSH(index int, target *Target) error {
	cfg := target.VNStatSSH
	if cfg == nil {
		return fmt.Errorf("target[%d]: vnstat_ssh object is required for vnstat_ssh type", index)
	}

	if cfg.Port == 0 {
		cfg.Port = DefaultSSHPort
	}
	if cfg.LookbackDays == 0 {
		cfg.LookbackDays = DefaultLookbackDays
	}
	if cfg.ConnectTimeoutSeconds == 0 {
		cfg.ConnectTimeoutSeconds = DefaultConnectTimeoutSeconds
	}
	if cfg.CommandTimeoutSeconds == 0 {
		cfg.CommandTimeoutSeconds = DefaultCommandTimeoutSeconds
	}
	if cfg.MaxDataAgeSeconds == 0 {
		cfg.MaxDataAgeSeconds = DefaultMaxDataAgeSeconds
	}

	if cfg.Host == "" {
		return fmt.Errorf("target[%d].vnstat_ssh: host is required", index)
	}
	if cfg.Username == "" {
		return fmt.Errorf("target[%d].vnstat_ssh: username is required", index)
	}
	if cfg.Interface == "" {
		return fmt.Errorf("target[%d].vnstat_ssh: interface is required", index)
	}
	if !interfacePattern.MatchString(cfg.Interface) {
		return fmt.Errorf("target[%d].vnstat_ssh: invalid interface %q", index, cfg.Interface)
	}
	if cfg.Timezone == "" {
		return fmt.Errorf("target[%d].vnstat_ssh: timezone is required", index)
	}
	if _, err := time.LoadLocation(cfg.Timezone); err != nil {
		return fmt.Errorf("target[%d].vnstat_ssh: invalid timezone %q: %w", index, cfg.Timezone, err)
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("target[%d].vnstat_ssh: port must be between 1 and 65535", index)
	}
	if cfg.QuotaBytes <= 0 {
		return fmt.Errorf("target[%d].vnstat_ssh: quota_bytes must be positive", index)
	}
	if cfg.BillingCycleDay < 1 || cfg.BillingCycleDay > 31 {
		return fmt.Errorf("target[%d].vnstat_ssh: billing_cycle_day must be between 1 and 31", index)
	}
	if cfg.LookbackDays < 35 || cfg.LookbackDays > MaxLookbackDays {
		return fmt.Errorf("target[%d].vnstat_ssh: lookback_days must be between 35 and %d", index, MaxLookbackDays)
	}
	if cfg.ConnectTimeoutSeconds <= 0 {
		return fmt.Errorf("target[%d].vnstat_ssh: connect_timeout_seconds must be positive", index)
	}
	if cfg.CommandTimeoutSeconds <= 0 {
		return fmt.Errorf("target[%d].vnstat_ssh: command_timeout_seconds must be positive", index)
	}
	if cfg.MaxDataAgeSeconds <= 0 {
		return fmt.Errorf("target[%d].vnstat_ssh: max_data_age_seconds must be positive", index)
	}

	if err := validateReadableAbsoluteFile(cfg.PrivateKeyFile, "private_key_file"); err != nil {
		return fmt.Errorf("target[%d].vnstat_ssh: %w", index, err)
	}
	if err := validateReadableAbsoluteFile(cfg.KnownHostsFile, "known_hosts_file"); err != nil {
		return fmt.Errorf("target[%d].vnstat_ssh: %w", index, err)
	}

	return nil
}

func validateReadableAbsoluteFile(path, field string) error {
	if path == "" {
		return fmt.Errorf("%s is required", field)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be an absolute path", field)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s is not readable: %w", field, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", field, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must reference a regular file", field)
	}
	return nil
}
