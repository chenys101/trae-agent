package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadOptions 控制配置加载行为。
type LoadOptions struct {
	ProjectDir      string // 项目根目录
	DefaultProvider string // flag 覆盖
	LogLevel        string // flag 覆盖
}

// Load 按优先级合并配置：默认 < 用户 < 项目 < env < flag
func Load(opts LoadOptions) (Config, error) {
	cfg := Default()

	if opts.ProjectDir == "" {
		opts.ProjectDir = os.Getenv("TRAE_PROJECT_DIR")
	}

	// 1. 用户级 ~/.trae/config.yaml
	userPath, err := userConfigPath()
	if err != nil {
		return cfg, fmt.Errorf("resolve user config path: %w", err)
	}
	if userCfg, err := loadYaml(userPath); err == nil {
		merge(&cfg, userCfg)
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("load user config: %w", err)
	}

	// 2. 项目级
	if opts.ProjectDir != "" {
		projPath := filepath.Join(opts.ProjectDir, ".trae", "config.yaml")
		if projCfg, err := loadYaml(projPath); err == nil {
			merge(&cfg, projCfg)
		} else if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("load project config: %w", err)
		}
	}

	// 3. 环境变量
	if v := os.Getenv("TRAE_DEFAULT_PROVIDER"); v != "" {
		cfg.DefaultProvider = v
	}
	if v := os.Getenv("TRAE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	// 4. flag
	if opts.DefaultProvider != "" {
		cfg.DefaultProvider = opts.DefaultProvider
	}
	if opts.LogLevel != "" {
		cfg.LogLevel = opts.LogLevel
	}

	return cfg, nil
}

func userConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".trae", "config.yaml"), nil
}

func loadYaml(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// merge 用 src 覆盖 dst，零值不覆盖。
func merge(dst *Config, src Config) {
	if src.DefaultProvider != "" {
		dst.DefaultProvider = src.DefaultProvider
	}
	if src.LogLevel != "" {
		dst.LogLevel = src.LogLevel
	}
	if src.SystemPrompt != "" {
		dst.SystemPrompt = src.SystemPrompt
	}
	if src.MaxSteps != 0 {
		dst.MaxSteps = src.MaxSteps
	}
	if dst.Providers == nil {
		dst.Providers = map[string]ProviderConfig{}
	}
	for k, v := range src.Providers {
		// 字段级合并：非零字段覆盖，零值保留 dst 原值
		base := dst.Providers[k]
		if v.APIKey != "" {
			base.APIKey = v.APIKey
		}
		if v.Provider != "" {
			base.Provider = v.Provider
		}
		if v.BaseURL != "" {
			base.BaseURL = v.BaseURL
		}
		if v.DefaultModel != "" {
			base.DefaultModel = v.DefaultModel
		}
		if v.MaxTokens != 0 {
			base.MaxTokens = v.MaxTokens
		}
		if v.Temperature != 0 {
			base.Temperature = v.Temperature
		}
		dst.Providers[k] = base
	}
}
