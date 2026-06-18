// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs", "trae_config.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "anthropic")
	}

	if len(cfg.ModelProviders) != 2 {
		t.Errorf("len(ModelProviders) = %d, want 2", len(cfg.ModelProviders))
	}

	anthropic, ok := cfg.ModelProviders["anthropic"]
	if !ok {
		t.Fatal("model provider anthropic not found")
	}
	if anthropic.Provider != "anthropic" {
		t.Errorf("anthropic.Provider = %q, want %q", anthropic.Provider, "anthropic")
	}
	if anthropic.DefaultModel != "claude-sonnet-4-20250514" {
		t.Errorf("anthropic.DefaultModel = %q, want %q", anthropic.DefaultModel, "claude-sonnet-4-20250514")
	}
	if anthropic.MaxTokens != 16384 {
		t.Errorf("anthropic.MaxTokens = %d, want 16384", anthropic.MaxTokens)
	}
	if anthropic.Temperature != 0.5 {
		t.Errorf("anthropic.Temperature = %f, want 0.5", anthropic.Temperature)
	}
	if anthropic.MaxRetries != 10 {
		t.Errorf("anthropic.MaxRetries = %d, want 10", anthropic.MaxRetries)
	}
	if !anthropic.ParallelToolCalls {
		t.Errorf("anthropic.ParallelToolCalls = false, want true")
	}

	if len(cfg.Agents) != 1 {
		t.Errorf("len(Agents) = %d, want 1", len(cfg.Agents))
	}

	agent, ok := cfg.Agents["trae_agent"]
	if !ok {
		t.Fatal("agent trae_agent not found")
	}
	if agent.Provider != "anthropic" {
		t.Errorf("agent.Provider = %q, want %q", agent.Provider, "anthropic")
	}
	if agent.MaxSteps != 30 {
		t.Errorf("agent.MaxSteps = %d, want 30", agent.MaxSteps)
	}
	if len(agent.Tools) != 8 {
		t.Errorf("len(agent.Tools) = %d, want 8", len(agent.Tools))
	}
}

func TestResolveModelConfig(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs", "trae_config.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	mc, err := cfg.ResolveModelConfig("anthropic", "")
	if err != nil {
		t.Fatalf("ResolveModelConfig failed: %v", err)
	}

	if mc.Model != "claude-sonnet-4-20250514" {
		t.Errorf("mc.Model = %q, want %q", mc.Model, "claude-sonnet-4-20250514")
	}
	if mc.Provider != "anthropic" {
		t.Errorf("mc.Provider = %q, want %q", mc.Provider, "anthropic")
	}
	if mc.MaxTokens != 16384 {
		t.Errorf("mc.MaxTokens = %d, want 16384", mc.MaxTokens)
	}
	if mc.Temperature != 0.5 {
		t.Errorf("mc.Temperature = %f, want 0.5", mc.Temperature)
	}

	// 测试不存在的 provider
	_, err = cfg.ResolveModelConfig("nonexistent", "")
	if err == nil {
		t.Error("expected error for nonexistent provider, got nil")
	}
}

func TestResolveModelConfigWithModelSpec(t *testing.T) {
	cfg := DefaultConfig()
	// 添加模型级覆盖
	mp := cfg.ModelProviders["anthropic"]
	mp.Models = map[string]ModelSpec{
		"claude-haiku-4-20250414": {
			MaxTokens:   8192,
			Temperature: 0.3,
		},
	}
	cfg.ModelProviders["anthropic"] = mp

	mc, err := cfg.ResolveModelConfig("anthropic", "claude-haiku-4-20250414")
	if err != nil {
		t.Fatalf("ResolveModelConfig failed: %v", err)
	}

	if mc.Model != "claude-haiku-4-20250414" {
		t.Errorf("mc.Model = %q, want %q", mc.Model, "claude-haiku-4-20250414")
	}
	if mc.MaxTokens != 8192 {
		t.Errorf("mc.MaxTokens = %d, want 8192", mc.MaxTokens)
	}
	if mc.Temperature != 0.3 {
		t.Errorf("mc.Temperature = %f, want 0.3", mc.Temperature)
	}
	// 未覆盖的字段应保持 provider 默认值
	if mc.MaxRetries != 10 {
		t.Errorf("mc.MaxRetries = %d, want 10", mc.MaxRetries)
	}
}

func TestEnvOverride(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs", "trae_config.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// 设置环境变量
	os.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	os.Setenv("OPENAI_API_KEY", "test-openai-key")
	defer os.Unsetenv("ANTHROPIC_API_KEY")
	defer os.Unsetenv("OPENAI_API_KEY")

	// 重新加载以触发环境变量覆盖
	cfg, err = LoadConfig(filepath.Join("..", "..", "configs", "trae_config.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ModelProviders["anthropic"].APIKey != "test-anthropic-key" {
		t.Errorf("anthropic.APIKey = %q, want %q", cfg.ModelProviders["anthropic"].APIKey, "test-anthropic-key")
	}
	if cfg.ModelProviders["openai"].APIKey != "test-openai-key" {
		t.Errorf("openai.APIKey = %q, want %q", cfg.ModelProviders["openai"].APIKey, "test-openai-key")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "anthropic")
	}
	if len(cfg.ModelProviders) != 2 {
		t.Errorf("len(ModelProviders) = %d, want 2", len(cfg.ModelProviders))
	}
	if len(cfg.Agents) != 1 {
		t.Errorf("len(Agents) = %d, want 1", len(cfg.Agents))
	}

	agent, ok := cfg.Agents["trae_agent"]
	if !ok {
		t.Fatal("agent trae_agent not found")
	}
	if agent.MaxSteps != 30 {
		t.Errorf("agent.MaxSteps = %d, want 30", agent.MaxSteps)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}
