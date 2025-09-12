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

func main() {
	// 创建日志器
	logger := logging.New()

	// 创建 OpenAI 兼容的客户端，连接到 Ollama
	openaiClient := openai.NewClient("", // Ollama 不需要 API Key
		openai.WithBaseURL("http://10.20.88.156:11434/v1"), // Ollama 的 OpenAI 兼容接口
		openai.WithModel("qwen3:32b"),                      // Ollama 中的 Qwen3 32B 模型
		openai.WithLogger(logger))

	// 创建工具注册器
	toolRegistry := tools.NewRegistry()
	// 可以在这里添加工具，暂时为空

	// 创建智能体
	agent, err := agent.NewAgent(
		agent.WithLLM(openaiClient),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithTools(toolRegistry.List()...),
		agent.WithSystemPrompt("你是一个有用的AI助手，使用中文回答问题。"),
		agent.WithMaxIterations(5),
		agent.WithName("AIBodyAssistant"),
	)
	if err != nil {
		logger.Error(context.Background(), "创建智能体失败", map[string]interface{}{"error": err.Error()})
		return
	}

	fmt.Println("AI-Body 简单对话示例")
	fmt.Println("连接到 Ollama (qwen3:32b)")
	fmt.Println("输入 'exit' 或 'quit' 退出")
	fmt.Println("----------------------------------------")

	scanner := bufio.NewScanner(os.Stdin)

	// 按照官方示例创建上下文
	ctx := context.Background()
	// 添加组织ID
	ctx = multitenancy.WithOrgID(ctx, "ai-body-demo")
	// 添加对话ID，使用官方示例的方式
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "demo-conversation-001")

	for {
		fmt.Print("你: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// 退出命令
		if input == "exit" || input == "quit" {
			fmt.Println("再见！")
			break
		}

		// 调用智能体处理用户输入
		response, err := agent.Run(ctx, input)
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			continue
		}

		fmt.Printf("AI: %s\n\n", response)
	}
}
