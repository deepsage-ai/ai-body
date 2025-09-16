package config

import "fmt"

// WeWorkConfig 企业微信配置
type WeWorkConfig struct {
	// 企业微信配置（已预设）
	Token  string
	AESKey string
	BotID  string

	// 千问API配置（需要设置）
	QwenAPIKey  string
	QwenModel   string
	QwenBaseURL string

	// MCP服务器配置
	MCPServerURL string

	// 服务配置
	Port string
}

// 预设的企业微信配置常量
const (
	WEWORK_TOKEN   = "LYDMxF6qKEPWVGdKaQdAYw9xxfSzDsIC"
	WEWORK_AES_KEY = "f4gfCYzaqGAfh4rqxWjqG9udsZwI0d3uRlx5cXVUgDu"
	WEWORK_BOT_ID  = "aibNFPh36tB2I_wrYlPhlPyOY_fQavVb3mc" // 企业微信机器人ID

	// 千问API配置（复用qwen-http版本的密钥）
	DEFAULT_QWEN_API_KEY  = "sk-0d8bebab081044f682fbeb6c147d8f2c" // 千问API密钥
	DEFAULT_QWEN_MODEL    = "qwen-max"
	DEFAULT_QWEN_BASE_URL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

	// MCP服务器配置
	DEFAULT_MCP_SERVER_URL = "http://sn.7soft.cn/sse"

	// 服务配置
	DEFAULT_PORT = "8889"
)

// LoadConfig 加载配置
func LoadConfig() (*WeWorkConfig, error) {
	config := &WeWorkConfig{
		// 企业微信配置（已预设）
		Token:  WEWORK_TOKEN,
		AESKey: WEWORK_AES_KEY,
		BotID:  WEWORK_BOT_ID,

		// 千问API配置
		QwenAPIKey:  DEFAULT_QWEN_API_KEY,
		QwenModel:   DEFAULT_QWEN_MODEL,
		QwenBaseURL: DEFAULT_QWEN_BASE_URL,

		// MCP服务器配置
		MCPServerURL: DEFAULT_MCP_SERVER_URL,

		// 服务配置
		Port: DEFAULT_PORT,
	}

	// 验证必要配置
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// validateConfig 验证配置
func validateConfig(config *WeWorkConfig) error {
	if config.Token == "" {
		return fmt.Errorf("企业微信Token不能为空")
	}

	if config.AESKey == "" {
		return fmt.Errorf("企业微信AESKey不能为空")
	}

	if len(config.AESKey) != 43 {
		return fmt.Errorf("企业微信AESKey长度必须为43位，当前长度: %d", len(config.AESKey))
	}

	if config.QwenAPIKey == "" || config.QwenAPIKey == "your_dashscope_api_key" {
		return fmt.Errorf("请设置有效的千问API密钥")
	}

	if config.Port == "" {
		return fmt.Errorf("服务端口不能为空")
	}

	return nil
}

// MaskSecret 掩码敏感信息用于日志显示
func (c *WeWorkConfig) MaskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}
