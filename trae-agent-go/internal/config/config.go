// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bytedance/trae-agent-go/internal/llm"
	"gopkg.in/yaml.v3"
)

// Config 是顶层配置结构体。
type Config struct {
	DefaultProvider string                   `yaml:"default_provider"`
	ModelProviders  map[string]ModelProvider `yaml:"model_providers"`
	Agents          map[string]AgentConfig   `yaml:"agents"`
}

// ModelProvider 定义模型提供者的配置。
type ModelProvider struct {
	APIKey            string               `yaml:"api_key"`
	Provider          string               `yaml:"provider"`
	BaseURL           string               `yaml:"base_url"`
	DefaultModel      string               `yaml:"default_model"`
	MaxTokens         int                  `yaml:"max_tokens"`
	Temperature       float64              `yaml:"temperature"`
	TopP              float64              `yaml:"top_p"`
	TopK              int                  `yaml:"top_k"`
	MaxRetries        int                  `yaml:"max_retries"`
	ParallelToolCalls bool                 `yaml:"parallel_tool_calls"`
	Models            map[string]ModelSpec `yaml:"models"`
}

// ModelSpec 定义模型级参数覆盖。
type ModelSpec struct {
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	TopP        float64 `yaml:"top_p"`
	TopK        int     `yaml:"top_k"`
}

// AgentConfig 定义 Agent 的配置参数，与 agent.AgentConfig 对应。
type AgentConfig struct {
	Provider   string   `yaml:"provider"`
	Model      string   `yaml:"model"`
	MaxSteps   int      `yaml:"max_steps"`
	Tools      []string `yaml:"tools"`
	WorkingDir string   `yaml:"working_dir"`
}

// LoadConfig 从指定路径加载配置文件。
// 如果 path 为空，尝试默认路径（./trae_config.yaml, ~/.trae/config.yaml）。
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = findDefaultConfig()
		if path == "" {
			return nil, fmt.Errorf("no config file found in default locations")
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// ResolveModelConfig 从 Config 中查找 ModelProvider 并应用 ModelSpec 覆盖，返回 llm.ModelConfig。
func (c *Config) ResolveModelConfig(providerName string, modelName string) (llm.ModelConfig, error) {
	mp, ok := c.ModelProviders[providerName]
	if !ok {
		return llm.ModelConfig{}, fmt.Errorf("model provider %q not found", providerName)
	}

	model := mp.DefaultModel
	if modelName != "" {
		model = modelName
	}

	mc := llm.ModelConfig{
		Model:             model,
		Provider:          mp.Provider,
		MaxTokens:         mp.MaxTokens,
		Temperature:       mp.Temperature,
		TopP:              mp.TopP,
		TopK:              mp.TopK,
		MaxRetries:        mp.MaxRetries,
		ParallelToolCalls: mp.ParallelToolCalls,
		APIKey:            mp.APIKey,
		BaseURL:           mp.BaseURL,
	}

	// 应用模型级覆盖
	if modelName != "" {
		if spec, ok := mp.Models[modelName]; ok {
			if spec.MaxTokens != 0 {
				mc.MaxTokens = spec.MaxTokens
			}
			if spec.Temperature != 0 {
				mc.Temperature = spec.Temperature
			}
			if spec.TopP != 0 {
				mc.TopP = spec.TopP
			}
			if spec.TopK != 0 {
				mc.TopK = spec.TopK
			}
		}
	}

	return mc, nil
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		DefaultProvider: "anthropic",
		ModelProviders: map[string]ModelProvider{
			"anthropic": {
				Provider:          "anthropic",
				DefaultModel:      "claude-sonnet-4-20250514",
				MaxTokens:         16384,
				Temperature:       0.5,
				MaxRetries:        10,
				ParallelToolCalls: true,
			},
			"openai": {
				Provider:          "openai",
				DefaultModel:      "gpt-4o",
				MaxTokens:         16384,
				Temperature:       0.5,
				MaxRetries:        10,
				ParallelToolCalls: true,
			},
		},
		Agents: map[string]AgentConfig{
			"trae_agent": {
				Provider: "anthropic",
				Model:    "claude-sonnet-4-20250514",
				MaxSteps: 30,
				Tools: []string{
					"bash",
					"read_file",
					"grep_search",
					"glob_search",
					"edit",
					"sequential_thinking",
					"sub_agent",
					"task_done",
				},
			},
		},
	}
}

// findDefaultConfig 在默认路径中查找配置文件。
func findDefaultConfig() string {
	candidates := []string{
		"./trae_config.yaml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".trae", "config.yaml"))
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// applyEnvOverrides 用环境变量覆盖 API Key。
func applyEnvOverrides(cfg *Config) {
	envMapping := map[string]string{
		"anthropic": "ANTHROPIC_API_KEY",
		"openai":    "OPENAI_API_KEY",
		"google":    "GOOGLE_API_KEY",
	}

	for providerName, envKey := range envMapping {
		if mp, ok := cfg.ModelProviders[providerName]; ok {
			if val := os.Getenv(envKey); val != "" {
				mp.APIKey = val
				cfg.ModelProviders[providerName] = mp
			}
		}
	}
}
