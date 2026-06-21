package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_defaultOnly(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// 隔离 TRAE_* env，避免外部环境干扰默认值断言。
	t.Setenv("TRAE_DEFAULT_PROVIDER", "")
	t.Setenv("TRAE_LOG_LEVEL", "")
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
	t.Setenv("TRAE_DEFAULT_PROVIDER", "")
	t.Setenv("TRAE_LOG_LEVEL", "")
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
	t.Setenv("TRAE_DEFAULT_PROVIDER", "")
	t.Setenv("TRAE_LOG_LEVEL", "")
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
	t.Setenv("TRAE_LOG_LEVEL", "")
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
	t.Setenv("TRAE_LOG_LEVEL", "")
	cfg, err := Load(LoadOptions{DefaultProvider: "gemini"})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "gemini" {
		t.Errorf("DefaultProvider = %q, want gemini (flag override)", cfg.DefaultProvider)
	}
}

// TestLoad_mergeMaxStepsAndSystemPrompt 验证 merge 不会漏掉 MaxSteps 和 SystemPrompt。
// 回归测试：旧实现 merge 只处理 DefaultProvider/LogLevel/Providers，
// 用户配置的 max_steps 和 system_prompt 会被静默丢弃。
func TestLoad_mergeMaxStepsAndSystemPrompt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("TRAE_DEFAULT_PROVIDER", "")
	t.Setenv("TRAE_LOG_LEVEL", "")
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
max_steps: 50
system_prompt: "You are a test agent"
`)
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxSteps != 50 {
		t.Errorf("MaxSteps = %d, want 50 (must not be dropped by merge)", cfg.MaxSteps)
	}
	if cfg.SystemPrompt != "You are a test agent" {
		t.Errorf("SystemPrompt = %q, want 'You are a test agent'", cfg.SystemPrompt)
	}
}

// TestLoad_mergeProviderFieldLevel 验证同名 provider 做字段级合并，
// 而非整体替换。回归测试：用户配 api_key、项目配 base_url，合并后应同时存在。
func TestLoad_mergeProviderFieldLevel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("TRAE_DEFAULT_PROVIDER", "")
	t.Setenv("TRAE_LOG_LEVEL", "")
	// 隔离约定 env，避免外部 TRAE_PROVIDER_ANTHROPIC_API_KEY 覆盖 user config 的 key。
	t.Setenv("TRAE_PROVIDER_ANTHROPIC_API_KEY", "")
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
model_providers:
  anthropic:
    api_key: sk-user-key
    provider: anthropic
`)
	projCfg := filepath.Join(tmp, "project", ".trae", "config.yaml")
	writeFile(t, projCfg, `
model_providers:
  anthropic:
    base_url: https://custom.anthropic.com
`)
	cfg, err := Load(LoadOptions{ProjectDir: filepath.Join(tmp, "project")})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	p, ok := cfg.Providers["anthropic"]
	if !ok {
		t.Fatal("anthropic provider missing")
	}
	if p.APIKey != "sk-user-key" {
		t.Errorf("APIKey = %q, want sk-user-key (field-level merge must preserve)", p.APIKey)
	}
	if p.BaseURL != "https://custom.anthropic.com" {
		t.Errorf("BaseURL = %q, want https://custom.anthropic.com", p.BaseURL)
	}
}

// TestRedactKey 验证 API key 脱敏逻辑。
func TestRedactKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"short", "***"},
		{"12345678", "***"}, // 长度 == 8，脱敏
		{"sk-ant-abc123-xyz789", "sk-a***z789"},
	}
	for _, c := range cases {
		got := redactKey(c.in)
		if got != c.want {
			t.Errorf("redactKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestConfig_Redacted 验证 Redacted 输出不包含原始 API key。
func TestConfig_Redacted(t *testing.T) {
	cfg := Config{
		DefaultProvider: "anthropic",
		Providers: map[string]ProviderConfig{
			"anthropic": {APIKey: "sk-ant-supersecretkey123", Provider: "anthropic"},
		},
	}
	r := cfg.Redacted()
	got := r.Providers["anthropic"].APIKey
	if got == "sk-ant-supersecretkey123" {
		t.Errorf("Redacted() leaked full API key: %q", got)
	}
	if !strings.Contains(got, "***") {
		t.Errorf("Redacted() APIKey = %q, must contain ***", got)
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
