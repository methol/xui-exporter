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
