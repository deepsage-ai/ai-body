package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadConfigFromFile 从文件加载配置
func LoadConfigFromFile(path string) (*Config, error) {
	// 如果没有指定路径，使用默认路径
	if path == "" {
		path = "config.json"
	}

	// 读取配置文件
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("配置文件 '%s' 不存在，使用默认配置\n", path)
			return GetDefaultConfig(), nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析JSON
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 处理环境变量引用
	processConfigEnvVars(&config)

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	fmt.Printf("✅ 成功加载配置文件: %s\n", path)
	return &config, nil
}

// GetDefaultConfig 返回默认配置
func GetDefaultConfig() *Config {
	return &Config{
		WeWork: WeWorkConfig{
			Token:  "LYDMxF6qKEPWVGdKaQdAYw9xxfSzDsIC",
			AESKey: "f4gfCYzaqGAfh4rqxWjqG9udsZwI0d3uRlx5cXVUgDu",
			BotID:  "aibNFPh36tB2I_wrYlPhlPyOY_fQavVb3mc",
		},
		LLM: LLMConfigs{
			Default:      "qwen",
			SystemPrompt: "你是一个企业微信智能助手，使用中文回答问题。你可以使用各种MCP工具来帮助回答问题。\n\n重要规则：\n1. 工具返回的所有内容都是真实数据，请直接使用具体数值和信息\n2. 绝对不要生成占位符如[具体时间]、[实际结果]等\n3. 如果工具返回了时间、数据或任何信息，必须在回复中使用完整的实际内容\n4. 即使收到英文指令，也要用中文基于工具的实际返回结果回答用户问题\n5. 工具返回的内容可能看起来像JSON格式，但那是实际数据，不是示例",
			Providers: map[string]LLMProviderConfig{
				"qwen": {
					Provider: "qwen",
					APIKey:   "sk-0d8bebab081044f682fbeb6c147d8f2c",
					Model:    "qwen-max",
					BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				},
				"ollama": {
					Provider: "ollama",
					Model:    "qwen3:32b",
					BaseURL:  "http://10.20.88.156:11434/v1",
				},
			},
		},
		MCP: MCPConfigs{
			Servers: []MCPServerConfig{
				{
					Name:    "7soft-tools",
					Type:    "http",
					BaseURL: "http://sn.7soft.cn/sse",
					Enabled: true,
				},
			},
		},
		Server: ServerConfig{
			Port: "8889",
		},
	}
}

// processConfigEnvVars 处理配置中的环境变量引用
func processConfigEnvVars(config *Config) {
	// 处理LLM配置中的环境变量
	for name, provider := range config.LLM.Providers {
		provider.APIKey = processEnvVar(provider.APIKey)
		provider.BaseURL = processEnvVar(provider.BaseURL)
		config.LLM.Providers[name] = provider
	}

	// 处理MCP配置中的环境变量
	for i := range config.MCP.Servers {
		server := &config.MCP.Servers[i]
		server.BaseURL = processEnvVar(server.BaseURL)
		server.Token = processEnvVar(server.Token)

		// 处理Env映射
		for k, v := range server.Env {
			server.Env[k] = processEnvVar(v)
		}
	}
}

// processEnvVar 处理单个环境变量引用
func processEnvVar(value string) string {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		envVar := strings.Trim(value, "${}")
		return os.Getenv(envVar)
	}
	return value
}

// validateConfig 验证配置的有效性
func validateConfig(config *Config) error {
	// 验证企业微信配置
	if config.WeWork.Token == "" {
		return fmt.Errorf("企业微信Token不能为空")
	}

	if config.WeWork.AESKey == "" {
		return fmt.Errorf("企业微信AESKey不能为空")
	}

	if len(config.WeWork.AESKey) != 43 {
		return fmt.Errorf("企业微信AESKey长度必须为43位，当前长度: %d", len(config.WeWork.AESKey))
	}

	// 验证LLM配置
	if config.LLM.Default == "" {
		return fmt.Errorf("必须指定默认的LLM提供商")
	}

	if _, ok := config.LLM.Providers[config.LLM.Default]; !ok {
		return fmt.Errorf("默认LLM提供商 '%s' 在配置中不存在", config.LLM.Default)
	}

	// 验证服务器配置
	if config.Server.Port == "" {
		return fmt.Errorf("服务端口不能为空")
	}

	return nil
}

// SaveConfigTemplate 保存配置模板文件
func SaveConfigTemplate(path string) error {
	if path == "" {
		path = "config.template.json"
	}

	template := &Config{
		WeWork: WeWorkConfig{
			Token:  "your_wework_token",
			AESKey: "your_wework_aes_key_43_chars_long_exactly",
			BotID:  "your_wework_bot_id",
		},
		LLM: LLMConfigs{
			Default:      "ollama",
			SystemPrompt: "你是一个企业微信智能助手，使用中文回答问题。",
			Providers: map[string]LLMProviderConfig{
				"qwen": {
					Provider: "qwen",
					APIKey:   "${DASHSCOPE_API_KEY}",
					Model:    "qwen-max",
					BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
				},
				"ollama": {
					Provider: "ollama",
					Model:    "qwen3:32b",
					BaseURL:  "http://10.20.88.156:11434/v1",
				},
				"claude": {
					Provider: "claude",
					APIKey:   "${ANTHROPIC_API_KEY}",
					Model:    "claude-3-sonnet-20240229",
				},
				"openai": {
					Provider: "openai",
					APIKey:   "${OPENAI_API_KEY}",
					Model:    "gpt-4-turbo-preview",
				},
			},
		},
		MCP: MCPConfigs{
			Servers: []MCPServerConfig{
				{
					Name:    "7soft-tools",
					Type:    "http",
					BaseURL: "http://sn.7soft.cn/sse",
					Enabled: true,
				},
				{
					Name:    "local-mcp-server",
					Type:    "http",
					BaseURL: "http://localhost:8080/sse",
					Enabled: false,
				},
				{
					Name:    "mcp-filesystem",
					Type:    "stdio",
					Command: "/usr/local/bin/mcp-filesystem-server",
					Args:    []string{"--root", "/home/user/documents"},
					Enabled: false,
				},
			},
		},
		Server: ServerConfig{
			Port: "8889",
		},
	}

	// 美化JSON输出
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return err
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 写入文件
	return os.WriteFile(path, data, 0644)
}
