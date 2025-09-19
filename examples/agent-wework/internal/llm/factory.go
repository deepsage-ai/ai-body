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

	// 如果启用思考模式，输出提示信息
	if provider.ThinkingMode {
		fmt.Printf("🧠 深入思考模式已启用 (Provider: %s)\n", provider.Provider)
	}

	return createLLMClient(provider, logger)
}

// createLLMClient 根据provider配置创建具体的LLM客户端
func createLLMClient(config config.LLMProviderConfig, logger logging.Logger) (interfaces.LLM, error) {
	switch config.Provider {
	case "ollama":
		// Ollama使用OpenAI兼容接口，不需要API Key
		client := openai.NewClient("",
			openai.WithBaseURL(config.BaseURL),
			openai.WithModel(config.Model),
			openai.WithLogger(logger),
			// 添加温度控制，让输出更确定性
			openai.WithTemperature(0.3))

		// 如果启用思考模式，创建支持reasoning的包装器
		if config.ThinkingMode {
			reasoningLevel := config.ReasoningLevel
			if reasoningLevel == "" {
				reasoningLevel = "minimal" // 默认简洁
			}
			fmt.Printf("✅ Ollama 思考模式已启用 (推理级别: %s)\n", reasoningLevel)
			return NewOpenAIThinkingWrapperWithLevel(client, reasoningLevel), nil
		}

		return client, nil

	case "qwen":
		// 千问使用DashScope的OpenAI兼容接口
		if config.APIKey == "" {
			return nil, fmt.Errorf("qwen requires API key")
		}

		client := openai.NewClient(config.APIKey,
			openai.WithBaseURL(config.BaseURL),
			openai.WithModel(config.Model),
			openai.WithLogger(logger))

		// 如果启用思考模式，创建支持reasoning的包装器
		if config.ThinkingMode {
			reasoningLevel := config.ReasoningLevel
			if reasoningLevel == "" {
				reasoningLevel = "minimal" // 默认简洁
			}
			fmt.Printf("✅ 千问 思考模式已启用 (推理级别: %s)\n", reasoningLevel)
			return NewOpenAIThinkingWrapperWithLevel(client, reasoningLevel), nil
		}

		return client, nil

	case "openai":
		// 标准OpenAI
		if config.APIKey == "" {
			return nil, fmt.Errorf("openai requires API key")
		}

		var client interfaces.LLM
		if config.BaseURL != "" {
			client = openai.NewClient(config.APIKey,
				openai.WithBaseURL(config.BaseURL),
				openai.WithModel(config.Model),
				openai.WithLogger(logger))
		} else {
			client = openai.NewClient(config.APIKey,
				openai.WithModel(config.Model),
				openai.WithLogger(logger))
		}

		// 如果启用思考模式，创建支持reasoning的包装器
		// 对于o1系列模型，这会自动启用内部推理
		if config.ThinkingMode {
			reasoningLevel := config.ReasoningLevel
			if reasoningLevel == "" {
				reasoningLevel = "minimal" // 默认简洁
			}
			fmt.Printf("✅ OpenAI 思考模式已启用 (模型: %s, 推理级别: %s)\n", config.Model, reasoningLevel)
			return NewOpenAIThinkingWrapperWithLevel(client, reasoningLevel), nil
		}

		return client, nil

	case "claude":
		// Claude使用专门的anthropic包
		if config.APIKey == "" {
			return nil, fmt.Errorf("claude requires API key")
		}

		// 创建基础客户端
		client := anthropic.NewClient(config.APIKey,
			anthropic.WithModel(config.Model),
			anthropic.WithLogger(logger))

		// 检查是否支持thinking mode
		if config.ThinkingMode && anthropic.SupportsThinking(config.Model) {
			fmt.Printf("✅ 模型 %s 支持深入思考模式\n", config.Model)
			// 创建包装客户端以启用thinking
			return NewThinkingLLMWrapper(client, config.Model), nil
		} else if config.ThinkingMode {
			fmt.Printf("⚠️  警告: 模型 %s 不支持深入思考模式\n", config.Model)
		}

		return client, nil

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
