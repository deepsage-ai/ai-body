package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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

	// 配置会话级MCP管理器（连接复用 + 调用去重）
	baseURL := "http://sn.7soft.cn/sse"
	fmt.Printf("%s配置会话级MCP管理器: %s%s\n", ColorYellow, baseURL, ColorReset)

	// 创建会话级MCP管理器（一个会话回合 = 一个连接 + 去重）
	sessionManager := NewSessionMCPManager(baseURL)
	mcpServers = append(mcpServers, sessionManager)
	fmt.Printf("%s✅ 会话级MCP管理器配置完成（连接复用+去重）%s\n", ColorGreen, ColorReset)

	// 测试连接以验证配置正确性
	fmt.Printf("%s正在测试连接和工具发现...%s\n", ColorYellow, ColorReset)
	tools, err := sessionManager.ListTools(context.Background())
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
		fmt.Printf("%sMCP集成: 支持 %d 个服务器的智能工具调用（会话级连接）%s\n", ColorGreen, len(mcpServers), ColorReset)
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
	fmt.Printf("%s特性: 会话级连接管理，连接复用+调用去重%s\n", ColorGray, ColorReset)
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

// SessionMCPManager - 会话级MCP连接管理器
// 特性：连接复用 + 调用去重 + 自动清理
type SessionMCPManager struct {
	baseURL       string
	connection    interfaces.MCPServer
	callCache     map[string]*interfaces.MCPToolResponse // tool_call_id -> response缓存
	lastActivity  time.Time                              // 最后活动时间
	sessionActive bool                                   // 会话是否活跃
	mutex         sync.RWMutex                           // 读写锁
}

// NewSessionMCPManager 创建会话级MCP管理器
func NewSessionMCPManager(baseURL string) *SessionMCPManager {
	return &SessionMCPManager{
		baseURL:   baseURL,
		callCache: make(map[string]*interfaces.MCPToolResponse),
		mutex:     sync.RWMutex{},
	}
}

// isConnectionAlive 检查连接是否仍然有效
func (s *SessionMCPManager) isConnectionAlive() bool {
	if s.connection == nil {
		return false
	}

	// 轻量级健康检查：测试ListTools
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := s.connection.ListTools(ctx)
	return err == nil
}

// createNewConnection 创建新的MCP连接
func (s *SessionMCPManager) createNewConnection(ctx context.Context) (interfaces.MCPServer, error) {
	fmt.Printf("%s[SessionMCP] 创建新连接...%s\n", ColorGreen, ColorReset)

	server, err := mcp.NewHTTPServer(context.Background(), mcp.HTTPServerConfig{
		BaseURL: s.baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建MCP连接失败: %w", err)
	}

	s.connection = server
	s.sessionActive = true
	s.lastActivity = time.Now()

	return server, nil
}

// cleanupConnection 清理连接和相关状态
func (s *SessionMCPManager) cleanupConnection() {
	if s.connection != nil {
		s.connection.Close()
		s.connection = nil
	}
	s.sessionActive = false
	s.callCache = make(map[string]*interfaces.MCPToolResponse) // 清空缓存
	fmt.Printf("%s[SessionMCP] 连接已清理%s\n", ColorGray, ColorReset)
}

// ensureConnection 确保有活跃的MCP连接（使用时验证）
func (s *SessionMCPManager) ensureConnection(ctx context.Context) (interfaces.MCPServer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 检查现有连接的有效性
	if s.connection != nil && s.sessionActive {
		// 时间检查：超过2分钟自动重建
		if time.Since(s.lastActivity) > 2*time.Minute {
			fmt.Printf("%s[SessionMCP] 连接超时(2分钟)，重建连接%s\n", ColorYellow, ColorReset)
			s.cleanupConnection()
		} else {
			// 健康检查：验证连接可用性
			if s.isConnectionAlive() {
				s.lastActivity = time.Now()
				fmt.Printf("%s[SessionMCP] 复用现有连接%s\n", ColorBlue, ColorReset)
				return s.connection, nil
			} else {
				fmt.Printf("%s[SessionMCP] 连接失效，重建连接%s\n", ColorYellow, ColorReset)
				s.cleanupConnection()
			}
		}
	}

	// 创建新连接
	return s.createNewConnection(ctx)
}

// Initialize 实现MCPServer接口
func (s *SessionMCPManager) Initialize(ctx context.Context) error {
	server, err := s.ensureConnection(ctx)
	if err != nil {
		return err
	}
	return server.Initialize(ctx)
}

// ListTools 实现MCPServer接口 - 使用会话连接
func (s *SessionMCPManager) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	server, err := s.ensureConnection(ctx)
	if err != nil {
		return nil, err
	}

	tools, err := server.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	// 添加详细的Schema调试输出
	fmt.Printf("%s[SessionMCP] Schema调试信息:%s\n", ColorYellow, ColorReset)
	for i, tool := range tools {
		fmt.Printf("%s  工具 %d: %s%s\n", ColorCyan, i+1, tool.Name, ColorReset)
		fmt.Printf("%s    描述: %s%s\n", ColorGray, tool.Description, ColorReset)

		if tool.Schema != nil {
			fmt.Printf("%s    Schema存在: %T%s\n", ColorGreen, tool.Schema, ColorReset)

			// 处理*jsonschema.Schema类型
			schemaStr := fmt.Sprintf("%v", tool.Schema)
			if strings.Contains(schemaStr, "<anonymous schema>") {
				fmt.Printf("%s    ⚠️ Schema信息被隐藏，尝试JSON序列化...%s\n", ColorYellow, ColorReset)

				// 尝试将schema转换为JSON来查看其内容
				if schemaBytes, err := json.Marshal(tool.Schema); err == nil {
					var schemaMap map[string]interface{}
					if err := json.Unmarshal(schemaBytes, &schemaMap); err == nil {
						fmt.Printf("%s    JSON序列化成功:%s\n", ColorGreen, ColorReset)

						if properties, exists := schemaMap["properties"]; exists {
							fmt.Printf("%s    参数定义: %+v%s\n", ColorBlue, properties, ColorReset)
						}

						if required, exists := schemaMap["required"]; exists {
							fmt.Printf("%s    必需参数: %+v%s\n", ColorGreen, required, ColorReset)
						}

						if schemaType, exists := schemaMap["type"]; exists {
							fmt.Printf("%s    Schema类型: %+v%s\n", ColorCyan, schemaType, ColorReset)
						}
					} else {
						fmt.Printf("%s    JSON反序列化失败: %v%s\n", ColorRed, err, ColorReset)
					}
				} else {
					fmt.Printf("%s    JSON序列化失败: %v%s\n", ColorRed, err, ColorReset)
				}
			} else {
				// 尝试直接作为map处理
				if schemaMap, ok := tool.Schema.(map[string]interface{}); ok {
					if properties, exists := schemaMap["properties"]; exists {
						fmt.Printf("%s    参数定义: %+v%s\n", ColorBlue, properties, ColorReset)
					} else {
						fmt.Printf("%s    ⚠️ 缺少properties字段%s\n", ColorYellow, ColorReset)
					}

					if required, exists := schemaMap["required"]; exists {
						fmt.Printf("%s    必需参数: %+v%s\n", ColorGreen, required, ColorReset)
					}
				} else {
					fmt.Printf("%s    ⚠️ Schema格式异常: %+v%s\n", ColorRed, tool.Schema, ColorReset)
				}
			}
		} else {
			fmt.Printf("%s    ❌ Schema为空%s\n", ColorRed, ColorReset)
		}
		fmt.Println()
	}

	// 转换schema格式，确保LLM能正确理解工具参数
	convertedTools := make([]interfaces.MCPTool, len(tools))
	for i, tool := range tools {
		convertedTools[i] = s.convertToolSchema(tool)
	}

	return convertedTools, nil
}

// convertToolSchema 将*jsonschema.Schema转换为标准的map格式
func (s *SessionMCPManager) convertToolSchema(tool interfaces.MCPTool) interfaces.MCPTool {
	if tool.Schema == nil {
		return tool
	}

	// 尝试将*jsonschema.Schema转换为map[string]interface{}
	if schemaBytes, err := json.Marshal(tool.Schema); err == nil {
		var schemaMap map[string]interface{}
		if err := json.Unmarshal(schemaBytes, &schemaMap); err == nil {
			fmt.Printf("%s[Schema转换] %s: 成功转换为标准格式%s\n", ColorGreen, tool.Name, ColorReset)

			// 创建新的工具对象，使用转换后的schema
			return interfaces.MCPTool{
				Name:        tool.Name,
				Description: tool.Description,
				Schema:      schemaMap, // 使用转换后的map格式
			}
		} else {
			fmt.Printf("%s[Schema转换] %s: JSON反序列化失败: %v%s\n", ColorRed, tool.Name, err, ColorReset)
		}
	} else {
		fmt.Printf("%s[Schema转换] %s: JSON序列化失败: %v%s\n", ColorRed, tool.Name, err, ColorReset)
	}

	// 如果转换失败，返回原始工具
	return tool
}

// CallTool 实现MCPServer接口 - 会话连接复用 + 调用去重（修复竞态条件）
func (s *SessionMCPManager) CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error) {
	// 生成调用唯一标识（用于去重）
	callID := s.generateCallID(name, args)

	// 使用写锁保护整个调用过程，防止竞态条件
	s.mutex.Lock()

	// 检查缓存（去重机制）
	if cachedResponse, exists := s.callCache[callID]; exists {
		s.mutex.Unlock()
		fmt.Printf("%s[SessionMCP] 去重：使用缓存结果 %s (ID: %s)%s\n", ColorBlue, name, callID[:8], ColorReset)
		return cachedResponse, nil
	}

	fmt.Printf("%s[SessionMCP] 调用工具: %s (ID: %s)%s\n", ColorYellow, name, callID[:8], ColorReset)

	// 临时释放锁获取连接（避免与ensureConnection死锁）
	s.mutex.Unlock()
	server, err := s.ensureConnection(ctx)
	if err != nil {
		return nil, err
	}

	// 执行工具调用
	response, err := server.CallTool(ctx, name, args)
	if err != nil {
		return nil, err
	}

	// 重新获取锁进行缓存操作
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 双重检查：防止在锁释放期间其他调用已完成相同操作
	if cachedResponse, exists := s.callCache[callID]; exists {
		fmt.Printf("%s[SessionMCP] 去重：锁释放期间已缓存 %s (ID: %s)%s\n", ColorBlue, name, callID[:8], ColorReset)
		return cachedResponse, nil
	}

	// 缓存结果
	s.callCache[callID] = response
	s.lastActivity = time.Now() // 更新活动时间

	fmt.Printf("%s[SessionMCP] 工具调用完成并缓存: %s%s\n", ColorGreen, name, ColorReset)
	return response, nil
}

// generateCallID 生成调用唯一标识
func (s *SessionMCPManager) generateCallID(name string, args interface{}) string {
	argsJSON, _ := json.Marshal(args)
	data := fmt.Sprintf("%s:%s", name, string(argsJSON))
	return fmt.Sprintf("%x", data) // 简单hash
}

// Close 实现MCPServer接口 - 手动清理会话连接
func (s *SessionMCPManager) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	fmt.Printf("%s[SessionMCP] 手动关闭会话连接%s\n", ColorYellow, ColorReset)
	s.cleanupConnection()
	return nil
}
