package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_defaultOnly(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.DefaultProvider != "" {
		t.Errorf("DefaultProvider = %q, want empty", cfg.DefaultProvider)
	}
}

func TestLoad_userConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
default_provider: anthropic
log_level: debug
`)
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider = %q, want anthropic", cfg.DefaultProvider)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoad_projectOverridesUser(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
default_provider: anthropic
log_level: debug
`)
	projCfg := filepath.Join(tmp, "project", ".trae", "config.yaml")
	writeFile(t, projCfg, `
default_provider: openai
`)
	cfg, err := Load(LoadOptions{ProjectDir: filepath.Join(tmp, "project")})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai (project override)", cfg.DefaultProvider)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug (from user)", cfg.LogLevel)
	}
}

func TestLoad_envOverridesAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
default_provider: anthropic
`)
	t.Setenv("TRAE_DEFAULT_PROVIDER", "openai")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai (env override)", cfg.DefaultProvider)
	}
}

func TestLoad_flagOverridesAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("TRAE_DEFAULT_PROVIDER", "openai")
	cfg, err := Load(LoadOptions{DefaultProvider: "gemini"})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "gemini" {
		t.Errorf("DefaultProvider = %q, want gemini (flag override)", cfg.DefaultProvider)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
