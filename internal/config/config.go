package config

// Config 是合并后的最终配置。
type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	Providers       map[string]ProviderConfig `yaml:"model_providers"`
	LogLevel        string                    `yaml:"log_level"`
	SystemPrompt    string                    `yaml:"system_prompt"`
	MaxSteps        int                       `yaml:"max_steps"`
}

// ProviderConfig 单个 LLM provider 配置。
type ProviderConfig struct {
	APIKey       string  `yaml:"api_key"`
	Provider     string  `yaml:"provider"`
	BaseURL      string  `yaml:"base_url"`
	DefaultModel string  `yaml:"default_model"`
	MaxTokens    int     `yaml:"max_tokens"`
	Temperature  float64 `yaml:"temperature"`
}

// Default 返回内置默认配置。
func Default() Config {
	return Config{
		DefaultProvider: "",
		Providers:       map[string]ProviderConfig{},
		LogLevel:        "info",
	}
}
