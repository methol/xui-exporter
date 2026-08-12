package config

import (
	"encoding/json"
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

func TestLoadConfig_VNStatSSHValidAndDefaults(t *testing.T) {
	cfg := validVNStatConfig(t)

	loaded, err := writeAndLoadConfig(t, cfg)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if loaded.RefreshIntervalSeconds != DefaultRefreshIntervalSeconds {
		t.Errorf("RefreshIntervalSeconds = %d, want %d", loaded.RefreshIntervalSeconds, DefaultRefreshIntervalSeconds)
	}
	vnstat := loaded.Targets[0].VNStatSSH
	if vnstat.Port != DefaultSSHPort {
		t.Errorf("Port = %d, want %d", vnstat.Port, DefaultSSHPort)
	}
	if vnstat.LookbackDays != DefaultLookbackDays {
		t.Errorf("LookbackDays = %d, want %d", vnstat.LookbackDays, DefaultLookbackDays)
	}
	if vnstat.ConnectTimeoutSeconds != DefaultConnectTimeoutSeconds {
		t.Errorf("ConnectTimeoutSeconds = %d, want %d", vnstat.ConnectTimeoutSeconds, DefaultConnectTimeoutSeconds)
	}
	if vnstat.CommandTimeoutSeconds != DefaultCommandTimeoutSeconds {
		t.Errorf("CommandTimeoutSeconds = %d, want %d", vnstat.CommandTimeoutSeconds, DefaultCommandTimeoutSeconds)
	}
	if vnstat.MaxDataAgeSeconds != DefaultMaxDataAgeSeconds {
		t.Errorf("MaxDataAgeSeconds = %d, want %d", vnstat.MaxDataAgeSeconds, DefaultMaxDataAgeSeconds)
	}
}

func TestLoadConfig_VNStatSSHObjectRequired(t *testing.T) {
	cfg := validVNStatConfig(t)
	cfg.Targets[0].VNStatSSH = nil
	if _, err := writeAndLoadConfig(t, cfg); err == nil {
		t.Fatal("LoadConfig() error = nil, want missing vnstat_ssh object error")
	}
}

func TestLoadConfig_VNStatSSHValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*VNStatSSHConfig)
	}{
		{name: "missing host", mutate: func(c *VNStatSSHConfig) { c.Host = "" }},
		{name: "missing username", mutate: func(c *VNStatSSHConfig) { c.Username = "" }},
		{name: "missing interface", mutate: func(c *VNStatSSHConfig) { c.Interface = "" }},
		{name: "invalid interface", mutate: func(c *VNStatSSHConfig) { c.Interface = "ens3;id" }},
		{name: "missing timezone", mutate: func(c *VNStatSSHConfig) { c.Timezone = "" }},
		{name: "invalid timezone", mutate: func(c *VNStatSSHConfig) { c.Timezone = "Mars/Olympus" }},
		{name: "invalid port below range", mutate: func(c *VNStatSSHConfig) { c.Port = -1 }},
		{name: "invalid port above range", mutate: func(c *VNStatSSHConfig) { c.Port = 65536 }},
		{name: "zero quota", mutate: func(c *VNStatSSHConfig) { c.QuotaBytes = 0 }},
		{name: "negative quota", mutate: func(c *VNStatSSHConfig) { c.QuotaBytes = -1 }},
		{name: "billing day zero", mutate: func(c *VNStatSSHConfig) { c.BillingCycleDay = 0 }},
		{name: "billing day 32", mutate: func(c *VNStatSSHConfig) { c.BillingCycleDay = 32 }},
		{name: "lookback too short", mutate: func(c *VNStatSSHConfig) { c.LookbackDays = 34 }},
		{name: "lookback too long", mutate: func(c *VNStatSSHConfig) { c.LookbackDays = MaxLookbackDays + 1 }},
		{name: "negative connect timeout", mutate: func(c *VNStatSSHConfig) { c.ConnectTimeoutSeconds = -1 }},
		{name: "negative command timeout", mutate: func(c *VNStatSSHConfig) { c.CommandTimeoutSeconds = -1 }},
		{name: "negative max data age", mutate: func(c *VNStatSSHConfig) { c.MaxDataAgeSeconds = -1 }},
		{name: "missing private key", mutate: func(c *VNStatSSHConfig) { c.PrivateKeyFile = "" }},
		{name: "relative private key", mutate: func(c *VNStatSSHConfig) { c.PrivateKeyFile = "key" }},
		{name: "nonexistent private key", mutate: func(c *VNStatSSHConfig) { c.PrivateKeyFile = "/nonexistent/vnstat-key" }},
		{name: "missing known hosts", mutate: func(c *VNStatSSHConfig) { c.KnownHostsFile = "" }},
		{name: "relative known hosts", mutate: func(c *VNStatSSHConfig) { c.KnownHostsFile = "known_hosts" }},
		{name: "nonexistent known hosts", mutate: func(c *VNStatSSHConfig) { c.KnownHostsFile = "/nonexistent/known_hosts" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validVNStatConfig(t)
			tt.mutate(cfg.Targets[0].VNStatSSH)
			if _, err := writeAndLoadConfig(t, cfg); err == nil {
				t.Fatal("LoadConfig() error = nil, want validation error")
			}
		})
	}
}

func TestLoadConfig_DuplicateTargetName(t *testing.T) {
	cfg := validVNStatConfig(t)
	cfg.Targets = append(cfg.Targets, Target{
		Name: "vnstat-test",
		Type: "xui",
		URL:  "https://example.com/sub/test",
	})
	if _, err := writeAndLoadConfig(t, cfg); err == nil {
		t.Fatal("LoadConfig() error = nil, want duplicate target name error")
	}
}

func TestLoadConfig_RefreshIntervalValidation(t *testing.T) {
	cfg := validVNStatConfig(t)
	cfg.RefreshIntervalSeconds = -1
	if _, err := writeAndLoadConfig(t, cfg); err == nil {
		t.Fatal("LoadConfig() error = nil, want refresh interval error")
	}
}

func validVNStatConfig(t *testing.T) Config {
	t.Helper()
	tmpDir := t.TempDir()
	privateKey := filepath.Join(tmpDir, "vnstat_ed25519")
	knownHosts := filepath.Join(tmpDir, "known_hosts")
	if err := os.WriteFile(privateKey, []byte("test private key"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownHosts, []byte("test known host"), 0600); err != nil {
		t.Fatal(err)
	}

	return Config{Targets: []Target{{
		Name: "vnstat-test",
		Type: "vnstat_ssh",
		VNStatSSH: &VNStatSSHConfig{
			Host:            "192.0.2.10",
			Username:        "vnstat-exporter",
			PrivateKeyFile:  privateKey,
			KnownHostsFile:  knownHosts,
			Interface:       "ens3",
			QuotaBytes:      536870912000,
			BillingCycleDay: 17,
			Timezone:        "UTC",
		},
	}}}
}

func writeAndLoadConfig(t *testing.T, cfg Config) (*Config, error) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(path)
}
