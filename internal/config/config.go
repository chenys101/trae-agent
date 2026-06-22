package config

// Config 是合并后的最终配置。
type Config struct {
	DefaultProvider string                     `yaml:"default_provider"`
	Providers       map[string]ProviderConfig  `yaml:"model_providers"`
	LogLevel        string                     `yaml:"log_level"`
	SystemPrompt    string                     `yaml:"system_prompt"`
	MaxSteps        int                        `yaml:"max_steps"`
	MCPServers      map[string]MCPServerConfig `yaml:"mcp_servers"`
}

// MCPServerConfig MCP server 配置。
type MCPServerConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`
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

// redactedProviderConfig 是 ProviderConfig 的脱敏视图，用于序列化输出。
// APIKey 仅保留前 4 位 + 后 4 位，中间用 *** 代替；过短的 key 全部脱敏为 ***。
type redactedProviderConfig struct {
	APIKey       string  `yaml:"api_key"`
	Provider     string  `yaml:"provider"`
	BaseURL      string  `yaml:"base_url"`
	DefaultModel string  `yaml:"default_model"`
	MaxTokens    int     `yaml:"max_tokens"`
	Temperature  float64 `yaml:"temperature"`
}

// Redacted 返回脱敏后的视图，用于 show-config 等输出场景，避免泄漏 API key。
func (p ProviderConfig) Redacted() redactedProviderConfig {
	return redactedProviderConfig{
		APIKey:       redactKey(p.APIKey),
		Provider:     p.Provider,
		BaseURL:      p.BaseURL,
		DefaultModel: p.DefaultModel,
		MaxTokens:    p.MaxTokens,
		Temperature:  p.Temperature,
	}
}

// redactKey 脱敏 API key：长度 > 8 时保留前 4 + 后 4，否则全部 ***。
func redactKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "***"
	}
	return k[:4] + "***" + k[len(k)-4:]
}

// redactedConfig 是 Config 的脱敏视图。
type redactedConfig struct {
	DefaultProvider string                            `yaml:"default_provider"`
	Providers       map[string]redactedProviderConfig `yaml:"model_providers"`
	LogLevel        string                            `yaml:"log_level"`
	SystemPrompt    string                            `yaml:"system_prompt"`
	MaxSteps        int                               `yaml:"max_steps"`
	MCPServers      map[string]MCPServerConfig        `yaml:"mcp_servers"`
}

// Redacted 返回脱敏后的配置视图，用于 show-config 输出。
func (c Config) Redacted() redactedConfig {
	providers := make(map[string]redactedProviderConfig, len(c.Providers))
	for k, v := range c.Providers {
		providers[k] = v.Redacted()
	}
	// MCP servers 配置直接透传，无需脱敏（不含密钥）
	var mcpServers map[string]MCPServerConfig
	if c.MCPServers != nil {
		mcpServers = make(map[string]MCPServerConfig, len(c.MCPServers))
		for k, v := range c.MCPServers {
			mcpServers[k] = v
		}
	}
	return redactedConfig{
		DefaultProvider: c.DefaultProvider,
		Providers:       providers,
		LogLevel:        c.LogLevel,
		SystemPrompt:    c.SystemPrompt,
		MaxSteps:        c.MaxSteps,
		MCPServers:      mcpServers,
	}
}

// Default 返回内置默认配置。
// MaxSteps 等默认值统一在此设置，调用方（run/interactive）不再各自兜底。
func Default() Config {
	return Config{
		DefaultProvider: "",
		Providers:       map[string]ProviderConfig{},
		LogLevel:        "info",
		MaxSteps:        20,
		MCPServers:      map[string]MCPServerConfig{},
	}
}
