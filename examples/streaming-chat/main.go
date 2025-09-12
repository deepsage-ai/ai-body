package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"

	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
)

// 颜色代码用于终端输出
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[37m"
)

func main() {
	// 创建日志器
	logger := logging.New()

	// 创建 OpenAI 兼容的客户端，连接到 Ollama
	openaiClient := openai.NewClient("", // Ollama 不需要 API Key
		openai.WithBaseURL("http://10.20.88.156:11434/v1"), // Ollama 的 OpenAI 兼容接口
		openai.WithModel("deepseek-r1:70b"),                // Ollama 中的 Qwen3 32B 模型
		openai.WithLogger(logger))

	// 创建工具注册器
	toolRegistry := tools.NewRegistry()
	// 可以在这里添加工具，暂时为空

	// 创建智能体
	agent, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(toolRegistry.List()...),
		agent.WithSystemPrompt("你是一个有用的AI助手，使用中文回答问题。请提供详细和有帮助的回答。"),
		agent.WithMaxIterations(5),
		agent.WithName("AIBodyStreamingAssistant"),
	)
	if err != nil {
		logger.Error(context.Background(), "创建智能体失败", map[string]interface{}{"error": err.Error()})
		return
	}

	fmt.Printf("%s=== AI-Body 流式对话示例 ===%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s连接到 Ollama (qwen3:32b) - 流式模式%s\n", ColorGreen, ColorReset)
	fmt.Printf("%s输入 'exit' 或 'quit' 退出%s\n", ColorYellow, ColorReset)
	fmt.Printf("%s========================================%s\n\n", ColorCyan, ColorReset)

	scanner := bufio.NewScanner(os.Stdin)

	// 按照官方示例创建上下文
	ctx := context.Background()
	// 添加组织ID
	ctx = multitenancy.WithOrgID(ctx, "ai-body-streaming-demo")
	// 添加对话ID，使用官方示例的方式
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "streaming-conversation-001")

	for {
		fmt.Printf("%s你: %s", ColorBlue, ColorReset)
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// 退出命令
		if input == "exit" || input == "quit" {
			fmt.Printf("%s再见！%s\n", ColorGreen, ColorReset)
			break
		}

		fmt.Printf("%sAI: %s", ColorPurple, ColorReset)

		// 尝试使用流式传输
		eventChan, err := agent.RunStream(ctx, input)
		if err != nil {
			// 如果流式传输不支持，使用普通模式
			fmt.Printf("%s[流式传输不可用，回退到普通模式]%s\n", ColorYellow, ColorReset)
			response, normalErr := agent.Run(ctx, input)
			if normalErr != nil {
				fmt.Printf("%s错误: %v%s\n", ColorRed, normalErr, ColorReset)
				continue
			}

			// 直接显示完整回答，不做任何模拟
			fmt.Print(response)
			fmt.Println("\n")
			continue
		}

		// 处理真实的流式事件
		fmt.Printf("%s[使用流式传输]%s", ColorGreen, ColorReset)
		var responseText strings.Builder
		eventCount := 0
		contentEvents := 0

		for event := range eventChan {
			eventCount++

			// 只显示有内容的事件，忽略调试信息
			if event.Content != "" {
				fmt.Print(event.Content)
				responseText.WriteString(event.Content)
				contentEvents++
			}
		}

		fmt.Printf("\n%s[流式传输完成 - 总事件: %d, 内容事件: %d]%s\n", ColorGreen, eventCount, contentEvents, ColorReset)

		fmt.Println("\n")
	}
}
