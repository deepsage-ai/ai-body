package llm

import (
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"

	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/config"
)

// CreateLLMFromConfig 根据配置创建LLM客户端
func CreateLLMFromConfig(cfg *config.Config, logger logging.Logger) (interfaces.LLM, error) {
	// 获取要使用的LLM名称
	llmName := cfg.LLM.Default

	// 支持环境变量覆盖
	if override := os.Getenv("LLM_PROVIDER"); override != "" {
		llmName = override
	}

	// 查找对应的provider配置
	provider, ok := cfg.LLM.Providers[llmName]
	if !ok {
		return nil, fmt.Errorf("LLM provider '%s' not found in config", llmName)
	}

	// 处理环境变量引用
	provider.APIKey = processEnvVar(provider.APIKey)
	provider.BaseURL = processEnvVar(provider.BaseURL)

	return createLLMClient(provider, logger)
}

// createLLMClient 根据provider配置创建具体的LLM客户端
func createLLMClient(config config.LLMProviderConfig, logger logging.Logger) (interfaces.LLM, error) {
	switch config.Provider {
	case "ollama":
		// Ollama使用OpenAI兼容接口，不需要API Key
		return openai.NewClient("",
			openai.WithBaseURL(config.BaseURL),
			openai.WithModel(config.Model),
			openai.WithLogger(logger)), nil

	case "qwen":
		// 千问使用DashScope的OpenAI兼容接口
		if config.APIKey == "" {
			return nil, fmt.Errorf("qwen requires API key")
		}
		return openai.NewClient(config.APIKey,
			openai.WithBaseURL(config.BaseURL),
			openai.WithModel(config.Model),
			openai.WithLogger(logger)), nil

	case "openai":
		// 标准OpenAI
		if config.APIKey == "" {
			return nil, fmt.Errorf("openai requires API key")
		}
		client := openai.NewClient(config.APIKey,
			openai.WithModel(config.Model),
			openai.WithLogger(logger))
		// 如果有自定义BaseURL，设置它
		if config.BaseURL != "" {
			client = openai.NewClient(config.APIKey,
				openai.WithBaseURL(config.BaseURL),
				openai.WithModel(config.Model),
				openai.WithLogger(logger))
		}
		return client, nil

	case "claude":
		// Claude使用专门的anthropic包
		if config.APIKey == "" {
			return nil, fmt.Errorf("claude requires API key")
		}
		return anthropic.NewClient(config.APIKey,
			anthropic.WithModel(config.Model),
			anthropic.WithLogger(logger)), nil

	case "custom":
		// 自定义OpenAI兼容端点
		if config.BaseURL == "" {
			return nil, fmt.Errorf("custom provider requires base_url")
		}
		return openai.NewClient(config.APIKey,
			openai.WithBaseURL(config.BaseURL),
			openai.WithModel(config.Model),
			openai.WithLogger(logger)), nil

	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", config.Provider)
	}
}

// processEnvVar 处理环境变量引用
func processEnvVar(value string) string {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		envVar := strings.Trim(value, "${}")
		return os.Getenv(envVar)
	}
	return value
}
