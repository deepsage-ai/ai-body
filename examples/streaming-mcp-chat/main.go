package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
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
	// 对于MCP工具调用，需要支持Function Calling的模型
	modelName := "qwen3:32b" // 尝试使用支持工具的模型
	fmt.Printf("%s尝试使用模型: %s (支持工具调用)%s\n", ColorYellow, modelName, ColorReset)

	openaiClient := openai.NewClient("", // Ollama 不需要 API Key
		openai.WithBaseURL("http://10.20.88.156:11434/v1"), // Ollama 的 OpenAI 兼容接口
		openai.WithModel(modelName),
		openai.WithLogger(logger))

	// 创建工具注册器 - 保持streaming-chat原有结构
	toolRegistry := tools.NewRegistry()

	// === MCP 服务器自动发现和连接 ===
	fmt.Printf("%s=== MCP服务器自动发现 ===%s\n", ColorCyan, ColorReset)
	var mcpServers []interfaces.MCPServer
	ctx := context.Background()

	// 1. 尝试连接HTTP MCP服务器
	fmt.Printf("%s正在连接HTTP MCP服务器: http://sn.7soft.cn/sse%s\n", ColorYellow, ColorReset)
	httpServer, err := mcp.NewHTTPServer(ctx, mcp.HTTPServerConfig{
		BaseURL: "http://sn.7soft.cn/sse",
	})
	if err != nil {
		fmt.Printf("%sWarning: HTTP MCP服务器连接失败: %v%s\n", ColorYellow, err, ColorReset)
	} else {
		mcpServers = append(mcpServers, httpServer)
		fmt.Printf("%s✅ HTTP MCP服务器连接成功%s\n", ColorGreen, ColorReset)

		// 列出HTTP MCP工具
		tools, err := httpServer.ListTools(ctx)
		if err != nil {
			fmt.Printf("%sWarning: 获取HTTP工具失败: %v%s\n", ColorYellow, err, ColorReset)
		} else {
			fmt.Printf("%s发现 %d 个HTTP MCP工具:%s\n", ColorGreen, len(tools), ColorReset)
			for i, tool := range tools {
				fmt.Printf("%s  [%d] %s: %s%s\n", ColorGray, i+1, tool.Name, tool.Description, ColorReset)
			}
		}

		// MCP服务器只支持工具发现，不支持提示词
	}

	// STDIO MCP服务器已移除，专注HTTP MCP集成

	// === 创建智能体 - 基于streaming-chat + MCP集成 ===
	var agentInstance *agent.Agent

	if len(mcpServers) > 0 {
		// 有MCP服务器时，使用WithMCPServers
		fmt.Printf("%s创建支持MCP的智能体 (连接 %d 个MCP服务器)...%s\n", ColorYellow, len(mcpServers), ColorReset)
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(openaiClient),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithTools(toolRegistry.List()...),
			agent.WithMCPServers(mcpServers),
			agent.WithRequirePlanApproval(false), // 自动执行工具，不需要审批
			agent.WithSystemPrompt("你是一个有用的AI助手，使用中文回答问题。你可以使用各种MCP工具来帮助回答问题，请根据用户问题智能选择和调用合适的工具。当你需要获取实时信息（如时间）或执行特定任务时，请主动使用相关工具。"),
			agent.WithMaxIterations(5),
			agent.WithName("AIBodyStreamingMCPAssistant"),
		)
	} else {
		// 没有MCP服务器时，使用基础配置（完全兼容streaming-chat）
		fmt.Printf("%s创建基础智能体 (无MCP支持)...%s\n", ColorYellow, ColorReset)
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(openaiClient),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithTools(toolRegistry.List()...),
			agent.WithSystemPrompt("你是一个有用的AI助手，使用中文回答问题。请提供详细和有帮助的回答。"),
			agent.WithMaxIterations(5),
			agent.WithName("AIBodyStreamingAssistant"),
		)
	}

	if err != nil {
		logger.Error(context.Background(), "创建智能体失败", map[string]interface{}{"error": err.Error()})
		return
	}

	// === 用户界面 - 完全保持streaming-chat风格 ===
	fmt.Printf("\n%s=== AI-Body 智能流式对话 (MCP增强版) ===%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s连接到 Ollama (%s) - 流式模式%s\n", ColorGreen, modelName, ColorReset)
	if len(mcpServers) > 0 {
		fmt.Printf("%s智能MCP集成: 支持 %d 个服务器的自动工具调用%s\n", ColorGreen, len(mcpServers), ColorReset)
		fmt.Printf("%s输入 'tools' 查看可用MCP工具%s\n", ColorYellow, ColorReset)
	}
	fmt.Printf("%s输入 'exit' 或 'quit' 退出%s\n", ColorYellow, ColorReset)
	fmt.Printf("%s================================================%s\n\n", ColorCyan, ColorReset)

	scanner := bufio.NewScanner(os.Stdin)

	// 按照streaming-chat示例创建上下文
	ctx = multitenancy.WithOrgID(ctx, "ai-body-streaming-mcp-demo")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, "streaming-mcp-conversation-001")

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

		// 工具查看命令
		if input == "tools" {
			showMCPCapabilities(mcpServers)
			continue
		}

		fmt.Printf("%sAI: %s", ColorPurple, ColorReset)

		// === 完全保持streaming-chat的流式处理逻辑 ===
		// 尝试使用流式传输
		eventChan, err := agentInstance.RunStream(ctx, input)
		if err != nil {
			// 如果流式传输不支持，使用普通模式
			fmt.Printf("%s[流式传输不可用，回退到普通模式]%s\n", ColorYellow, ColorReset)
			response, normalErr := agentInstance.Run(ctx, input)
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
		fmt.Printf("%s[使用智能流式传输]%s", ColorGreen, ColorReset)
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

// 显示MCP服务器的能力
func showMCPCapabilities(mcpServers []interfaces.MCPServer) {
	fmt.Printf("%s=== MCP服务器能力总览 ===%s\n", ColorCyan, ColorReset)

	if len(mcpServers) == 0 {
		fmt.Printf("%s当前无可用MCP服务器%s\n", ColorGray, ColorReset)
		return
	}

	ctx := context.Background()
	totalTools := 0
	// totalPrompts := 0 // 当前不支持提示词

	for i, server := range mcpServers {
		fmt.Printf("\n%s[MCP服务器 %d]%s\n", ColorYellow, i+1, ColorReset)

		// 显示工具
		tools, err := server.ListTools(ctx)
		if err != nil {
			fmt.Printf("%s  工具获取失败: %v%s\n", ColorRed, err, ColorReset)
		} else {
			totalTools += len(tools)
			fmt.Printf("%s  工具 (%d个):%s\n", ColorGreen, len(tools), ColorReset)
			for j, tool := range tools {
				fmt.Printf("%s    [%d] %s: %s%s\n", ColorGray, j+1, tool.Name, tool.Description, ColorReset)
			}
		}

		// MCP服务器当前只支持工具，不支持提示词
	}

	fmt.Printf("\n%s总计: %d个MCP服务器, %d个工具%s\n", ColorCyan, len(mcpServers), totalTools, ColorReset)
}
