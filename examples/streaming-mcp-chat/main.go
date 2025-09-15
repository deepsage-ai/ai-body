package main

import (
	"bufio"
	"context"
	"encoding/json"
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

	// === MCP 按需连接配置 ===
	fmt.Printf("%s=== MCP按需连接配置 ===%s\n", ColorCyan, ColorReset)
	var mcpServers []interfaces.MCPServer

	// 配置LazyMCP服务器（不立即连接，模拟大模型调用模式）
	baseURL := "http://sn.7soft.cn/sse"
	fmt.Printf("%s配置LazyMCP服务器: %s%s\n", ColorYellow, baseURL, ColorReset)

	// 创建延迟连接的MCP服务器（真正按需：用时连接，用完关闭）
	lazyServer := NewLazyMCPServer(baseURL)
	mcpServers = append(mcpServers, lazyServer)
	fmt.Printf("%s✅ LazyMCP服务器配置完成（用时连接，用完关闭）%s\n", ColorGreen, ColorReset)

	// 测试连接以验证配置正确性
	fmt.Printf("%s正在测试连接和工具发现...%s\n", ColorYellow, ColorReset)
	tools, err := lazyServer.ListTools(context.Background())
	if err != nil {
		fmt.Printf("%sWarning: 测试连接失败: %v%s\n", ColorYellow, err, ColorReset)
	} else {
		fmt.Printf("%s发现 %d 个MCP工具:%s\n", ColorGreen, len(tools), ColorReset)
		for i, tool := range tools {
			fmt.Printf("%s  [%d] %s: %s%s\n", ColorGray, i+1, tool.Name, tool.Description, ColorReset)
		}
	}

	// === 创建智能体 - 基于streaming-chat + MCP集成 ===
	var agentInstance *agent.Agent

	if len(mcpServers) > 0 {
		// 有MCP服务器时，使用WithMCPServers
		fmt.Printf("%s创建MCP智能体 (连接 %d 个MCP服务器)...%s\n", ColorYellow, len(mcpServers), ColorReset)
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
		fmt.Printf("%sMCP集成: 支持 %d 个服务器的智能工具调用（按需连接）%s\n", ColorGreen, len(mcpServers), ColorReset)
		fmt.Printf("%s输入 'tools' 查看可用MCP工具%s\n", ColorYellow, ColorReset)
	}
	fmt.Printf("%s输入 'exit' 或 'quit' 退出%s\n", ColorYellow, ColorReset)
	fmt.Printf("%s================================================%s\n\n", ColorCyan, ColorReset)

	scanner := bufio.NewScanner(os.Stdin)

	// 按照streaming-chat示例创建上下文
	ctx := context.Background()
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

	for i, server := range mcpServers {
		fmt.Printf("\n%s[MCP服务器 %d]%s\n", ColorYellow, i+1, ColorReset)

		// 显示工具详细信息
		tools, err := server.ListTools(ctx)
		if err != nil {
			fmt.Printf("%s  工具获取失败: %v%s\n", ColorRed, err, ColorReset)
			fmt.Printf("%s  提示: 请检查MCP服务器连接状态%s\n", ColorGray, ColorReset)
		} else {
			totalTools += len(tools)
			fmt.Printf("%s  工具 (%d个):%s\n", ColorGreen, len(tools), ColorReset)
			for j, tool := range tools {
				showDetailedToolInfo(j+1, tool)
			}
		}
	}

	fmt.Printf("\n%s总计: %d个MCP服务器, %d个工具%s\n", ColorCyan, len(mcpServers), totalTools, ColorReset)
	fmt.Printf("%s特性: LazyMCP按需连接，用时连接用完关闭%s\n", ColorGray, ColorReset)
}

// showDetailedToolInfo 动态显示工具的详细信息（通用化处理）
func showDetailedToolInfo(index int, tool interfaces.MCPTool) {
	fmt.Printf("%s    [%d] %s: %s%s\n", ColorGray, index, tool.Name, tool.Description, ColorReset)

	// 动态解析Schema
	if tool.Schema != nil {
		analyzeToolSchema(tool)
	} else {
		fmt.Printf("%s        无参数要求%s\n", ColorGray, ColorReset)
	}

	fmt.Println() // 空行分隔
}

// analyzeToolSchema 动态分析工具的参数schema
func analyzeToolSchema(tool interfaces.MCPTool) {
	fmt.Printf("%s        参数分析:%s\n", ColorBlue, ColorReset)

	// 先进行类型断言
	schemaMap, ok := tool.Schema.(map[string]interface{})
	if !ok {
		fmt.Printf("%s          Schema格式不支持解析%s\n", ColorRed, ColorReset)
		return
	}

	// 显示整体类型
	if schemaType, ok := schemaMap["type"]; ok {
		fmt.Printf("%s          Schema类型: %v%s\n", ColorGray, schemaType, ColorReset)
	}

	// 动态解析properties
	if properties, ok := schemaMap["properties"]; ok {
		if propsMap, ok := properties.(map[string]interface{}); ok {
			fmt.Printf("%s          可用参数:%s\n", ColorGray, ColorReset)

			for paramName, paramInfo := range propsMap {
				analyzeParameter(paramName, paramInfo)
			}
		}
	}

	// 显示必需参数
	if required, ok := schemaMap["required"]; ok {
		if reqList, ok := required.([]interface{}); ok {
			fmt.Printf("%s          必需参数: [%s", ColorYellow, ColorReset)
			for i, req := range reqList {
				if i > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("%s%v%s", ColorYellow, req, ColorReset)
			}
			fmt.Printf("%s]%s\n", ColorYellow, ColorReset)
		}
	}

	// 生成动态使用示例
	generateDynamicUsageExample(tool)
}

// analyzeParameter 动态分析单个参数
func analyzeParameter(paramName string, paramInfo interface{}) {
	if paramMap, ok := paramInfo.(map[string]interface{}); ok {
		fmt.Printf("%s            %s:%s\n", ColorCyan, paramName, ColorReset)

		// 参数类型
		if paramType, ok := paramMap["type"]; ok {
			fmt.Printf("%s              类型: %v%s\n", ColorGray, paramType, ColorReset)
		}

		// 参数描述
		if paramDesc, ok := paramMap["description"]; ok {
			fmt.Printf("%s              描述: %v%s\n", ColorGray, paramDesc, ColorReset)
		}

		// 枚举值（关键信息！）
		if enumValues, ok := paramMap["enum"]; ok {
			fmt.Printf("%s              支持的值: %s%v%s\n", ColorGreen, ColorGreen, enumValues, ColorReset)
		}

		// 示例值
		if examples, ok := paramMap["examples"]; ok {
			fmt.Printf("%s              示例: %s%v%s\n", ColorGreen, ColorGreen, examples, ColorReset)
		}

		// 默认值
		if defaultValue, ok := paramMap["default"]; ok {
			fmt.Printf("%s              默认值: %s%v%s\n", ColorGreen, ColorGreen, defaultValue, ColorReset)
		}

		// 格式限制
		if format, ok := paramMap["format"]; ok {
			fmt.Printf("%s              格式: %v%s\n", ColorYellow, format, ColorReset)
		}

		// 值范围
		if minimum, ok := paramMap["minimum"]; ok {
			fmt.Printf("%s              最小值: %v%s\n", ColorYellow, minimum, ColorReset)
		}
		if maximum, ok := paramMap["maximum"]; ok {
			fmt.Printf("%s              最大值: %v%s\n", ColorYellow, maximum, ColorReset)
		}

		// 字符串长度限制
		if minLength, ok := paramMap["minLength"]; ok {
			fmt.Printf("%s              最小长度: %v%s\n", ColorYellow, minLength, ColorReset)
		}
		if maxLength, ok := paramMap["maxLength"]; ok {
			fmt.Printf("%s              最大长度: %v%s\n", ColorYellow, maxLength, ColorReset)
		}

		// 正则模式
		if pattern, ok := paramMap["pattern"]; ok {
			fmt.Printf("%s              模式: %v%s\n", ColorYellow, pattern, ColorReset)
		}
	}
}

// generateDynamicUsageExample 根据schema动态生成使用示例
func generateDynamicUsageExample(tool interfaces.MCPTool) {
	fmt.Printf("%s        动态使用示例:%s\n", ColorCyan, ColorReset)

	if tool.Schema == nil {
		fmt.Printf("%s          %s()%s\n", ColorGreen, tool.Name, ColorReset)
		return
	}

	// 构建示例参数
	exampleArgs := make(map[string]interface{})

	// 先进行Schema类型断言
	schemaMap, ok := tool.Schema.(map[string]interface{})
	if !ok {
		fmt.Printf("%s          %s({})%s\n", ColorGreen, tool.Name, ColorReset)
		return
	}

	if properties, ok := schemaMap["properties"]; ok {
		if propsMap, ok := properties.(map[string]interface{}); ok {
			for paramName, paramInfo := range propsMap {
				if paramMap, ok := paramInfo.(map[string]interface{}); ok {
					// 优先使用默认值
					if defaultValue, ok := paramMap["default"]; ok {
						exampleArgs[paramName] = defaultValue
					} else if examples, ok := paramMap["examples"]; ok {
						// 使用示例值
						if exampleList, ok := examples.([]interface{}); ok && len(exampleList) > 0 {
							exampleArgs[paramName] = exampleList[0]
						}
					} else if enumValues, ok := paramMap["enum"]; ok {
						// 使用枚举值的第一个
						if enumList, ok := enumValues.([]interface{}); ok && len(enumList) > 0 {
							exampleArgs[paramName] = enumList[0]
						}
					} else {
						// 根据类型生成占位符
						if paramType, ok := paramMap["type"]; ok {
							switch paramType {
							case "string":
								exampleArgs[paramName] = fmt.Sprintf("<%s>", paramName)
							case "integer", "number":
								exampleArgs[paramName] = 0
							case "boolean":
								exampleArgs[paramName] = false
							}
						}
					}
				}
			}
		}
	}

	// 生成JSON格式的调用示例
	if len(exampleArgs) > 0 {
		exampleJSON, _ := json.MarshalIndent(exampleArgs, "          ", "  ")
		fmt.Printf("%s          %s(%s)%s\n", ColorGreen, tool.Name, string(exampleJSON), ColorReset)
	} else {
		fmt.Printf("%s          %s({})%s\n", ColorGreen, tool.Name, ColorReset)
	}
}

// LazyMCPServer - 真正按需连接的MCP服务器包装器
// 模拟大模型调用模式：需要时连接，用完立即关闭
type LazyMCPServer struct {
	baseURL string
}

// NewLazyMCPServer 创建延迟连接的MCP服务器
func NewLazyMCPServer(baseURL string) *LazyMCPServer {
	return &LazyMCPServer{
		baseURL: baseURL,
	}
}

// createFreshConnection 创建新的MCP连接（每次调用独立连接）
func (l *LazyMCPServer) createFreshConnection(ctx context.Context) (interfaces.MCPServer, error) {
	// 直接使用传入的上下文，不设置超时，避免连接被提前关闭
	server, err := mcp.NewHTTPServer(ctx, mcp.HTTPServerConfig{
		BaseURL: l.baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建MCP连接失败: %w", err)
	}

	return server, nil
}

// executeWithFreshConnection 使用新连接执行操作的通用模式
func (l *LazyMCPServer) executeWithFreshConnection(ctx context.Context, operation func(interfaces.MCPServer) (interface{}, error)) (interface{}, error) {
	fmt.Printf("%s[LazyMCP] 创建新连接...%s\n", ColorGreen, ColorReset)

	// 创建新连接
	server, err := l.createFreshConnection(ctx)
	if err != nil {
		return nil, err
	}

	// 确保连接在函数结束时立即关闭
	defer func() {
		fmt.Printf("%s[LazyMCP] 关闭连接...%s\n", ColorGreen, ColorReset)
		if closeErr := server.Close(); closeErr != nil {
			fmt.Printf("%sWarning: 关闭MCP连接失败: %v%s\n", ColorYellow, closeErr, ColorReset)
		}
	}()

	// 执行操作
	result, err := operation(server)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Initialize 实现MCPServer接口
func (l *LazyMCPServer) Initialize(ctx context.Context) error {
	// 对于LazyMCP，初始化就是测试连接能力
	_, err := l.executeWithFreshConnection(ctx, func(server interfaces.MCPServer) (interface{}, error) {
		return nil, server.Initialize(ctx)
	})
	return err
}

// ListTools 实现MCPServer接口 - 每次调用创建新连接
func (l *LazyMCPServer) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	result, err := l.executeWithFreshConnection(ctx, func(server interfaces.MCPServer) (interface{}, error) {
		return server.ListTools(ctx)
	})
	if err != nil {
		return nil, err
	}
	return result.([]interfaces.MCPTool), nil
}

// CallTool 实现MCPServer接口 - 每次调用创建新连接
func (l *LazyMCPServer) CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error) {
	fmt.Printf("%s[LazyMCP] 调用工具: %s%s\n", ColorYellow, name, ColorReset)

	result, err := l.executeWithFreshConnection(ctx, func(server interfaces.MCPServer) (interface{}, error) {
		return server.CallTool(ctx, name, args)
	})
	if err != nil {
		return nil, err
	}
	return result.(*interfaces.MCPToolResponse), nil
}

// Close 实现MCPServer接口 - 对于LazyMCP，Close不需要做任何事
func (l *LazyMCPServer) Close() error {
	// LazyMCP每次调用后都会立即关闭连接
	// 所以这里不需要做任何清理工作
	fmt.Printf("%s[LazyMCP] Close调用（无需清理）%s\n", ColorGray, ColorReset)
	return nil
}
