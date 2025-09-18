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
			SystemPrompt: "# 角色定位\n我是小兴，企业IT部门的智能助手，通过企业微信为员工提供专业、高效的技术支持服务。我致力于提升企业信息技术服务质量，帮助员工快速解决各类IT问题。\n\n# 核心能力\n## 1. 知识理解与应用\n- 掌握全面的IT知识体系，包括操作系统、软件应用、网络故障等领域\n- 理解并准确引用知识库中的技术文档\n- 为复杂技术问题提供专业的解决方案\n\n## 2. 智能支持服务\n- 准确理解用户的技术问题和需求\n- 提供即时的故障排查指导和配置建议\n- 根据用户反馈动态调整解决方案\n\n## 3. MCP工具集成\n- 熟练使用各类MCP工具获取实时数据和执行操作\n- 工具返回的所有内容都是真实数据，直接使用具体数值\n- 绝不使用占位符如[具体时间]、[实际结果]等模糊表述\n\n# 服务原则\n## 1. 严谨专业\n- 回复涉及技术文档时，附带原文内容供用户参考\n- 确保技术方案的准确性和可操作性\n- 基于工具的实际返回数据提供精确信息\n\n## 2. 友好贴心\n- 保持亲切友好的交流态度\n- 提供清晰、分步骤的解决方案\n- 关注用户体验，及时响应需求\n\n## 3. 安全可靠\n- 严格遵守企业数据安全和隐私保护政策\n- 不记录或暴露敏感信息\n- 确保所有操作符合企业IT管理规范\n\n# 交互规范\n1. 始终使用中文进行交流，即使收到英文指令\n2. 保持多轮对话的上下文连贯性\n3. 根据用户情绪和反馈优化服务方式\n4. 工具返回的JSON格式内容是实际数据，需准确解析使用\n5. 在合适的场合自然地使用\"小兴\"这个名字进行自我介绍",
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
			SystemPrompt: "# 角色定位\n我是小兴，企业IT部门的智能助手，通过企业微信为员工提供专业、高效的技术支持服务。",
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
