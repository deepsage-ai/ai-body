package config

// Config 完整的应用配置
type Config struct {
	WeWork  WeWorkConfig  `json:"wework"`
	LLM     LLMConfigs    `json:"llm"`
	MCP     MCPConfigs    `json:"mcp"`
	Server  ServerConfig  `json:"server"`
	Logging LoggingConfig `json:"logging"`
}

// WeWorkConfig 企业微信配置
type WeWorkConfig struct {
	Token  string `json:"token"`
	AESKey string `json:"aes_key"`
	BotID  string `json:"bot_id"`
}

// LLMConfigs LLM配置集合
type LLMConfigs struct {
	Default      string                       `json:"default"`       // 默认使用的LLM
	SystemPrompt string                       `json:"system_prompt"` // 系统提示词
	Providers    map[string]LLMProviderConfig `json:"providers"`     // 可用的LLM提供商
}

// LLMProviderConfig 单个LLM提供商配置
type LLMProviderConfig struct {
	Provider       string `json:"provider"`                  // 提供商类型: qwen, ollama, claude, openai, custom
	APIKey         string `json:"api_key,omitempty"`         // API密钥（某些提供商需要）
	Model          string `json:"model"`                     // 模型名称
	BaseURL        string `json:"base_url,omitempty"`        // API基础URL（可选）
	ThinkingMode   bool   `json:"thinking_mode"`             // 深入思考模式开关
	ReasoningLevel string `json:"reasoning_level,omitempty"` // 推理等级: minimal(简洁) 或 comprehensive(详细)
}

// MCPConfigs MCP服务器配置集合
type MCPConfigs struct {
	Servers []MCPServerConfig `json:"servers"`
}

// MCPServerConfig 单个MCP服务器配置
type MCPServerConfig struct {
	Name    string `json:"name"`    // 服务器名称
	Type    string `json:"type"`    // 类型: http 或 stdio
	Enabled bool   `json:"enabled"` // 是否启用

	// HTTP类型配置
	BaseURL string `json:"base_url,omitempty"`
	Path    string `json:"path,omitempty"`
	Token   string `json:"token,omitempty"`

	// Stdio类型配置
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ServerConfig HTTP服务器配置
type ServerConfig struct {
	Port string `json:"port"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Enabled bool   `json:"enabled"` // 是否启用日志
	LogDir  string `json:"log_dir"` // 日志目录
}
