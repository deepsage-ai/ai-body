package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

	// === MCP 服务器自动发现和连接 ===
	fmt.Printf("%s=== MCP服务器自动发现 ===%s\n", ColorCyan, ColorReset)
	var mcpServers []interfaces.MCPServer
	ctx := context.Background()

	// 1. 创建弹性HTTP MCP服务器（带自动重连）
	baseURL := "http://sn.7soft.cn/sse"
	fmt.Printf("%s正在创建弹性HTTP MCP服务器: %s%s\n", ColorYellow, baseURL, ColorReset)

	httpServer, err := mcp.NewHTTPServer(ctx, mcp.HTTPServerConfig{
		BaseURL: baseURL,
	})
	if err != nil {
		fmt.Printf("%sWarning: 初始HTTP MCP服务器创建失败: %v%s\n", ColorYellow, err, ColorReset)
	} else {
		// 使用弹性包装器
		resilientServer := NewResilientMCPServer(httpServer, baseURL)
		mcpServers = append(mcpServers, resilientServer)
		fmt.Printf("%s✅ 弹性HTTP MCP服务器创建成功（支持自动重连）%s\n", ColorGreen, ColorReset)

		// 列出初始工具
		tools, err := resilientServer.ListTools(ctx)
		if err != nil {
			fmt.Printf("%sWarning: 获取初始工具列表失败: %v%s\n", ColorYellow, err, ColorReset)
		} else {
			fmt.Printf("%s发现 %d 个弹性MCP工具:%s\n", ColorGreen, len(tools), ColorReset)
			for i, tool := range tools {
				fmt.Printf("%s  [%d] %s: %s%s\n", ColorGray, i+1, tool.Name, tool.Description, ColorReset)
			}
		}

		// 显示健康状态
		if resilientServer.IsHealthy() {
			fmt.Printf("%s✅ MCP服务器健康状态：正常%s\n", ColorGreen, ColorReset)
		} else {
			fmt.Printf("%s⚠️ MCP服务器健康状态：异常%s\n", ColorYellow, ColorReset)
		}
	}

	// STDIO MCP服务器已移除，专注HTTP MCP集成

	// === 创建智能体 - 基于streaming-chat + MCP集成 ===
	var agentInstance *agent.Agent

	if len(mcpServers) > 0 {
		// 有弹性MCP服务器时，使用WithMCPServers
		fmt.Printf("%s创建弹性MCP智能体 (连接 %d 个弹性MCP服务器)...%s\n", ColorYellow, len(mcpServers), ColorReset)
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
		fmt.Printf("%s弹性MCP集成: 支持 %d 个服务器的自动工具调用+自动重连%s\n", ColorGreen, len(mcpServers), ColorReset)
		fmt.Printf("%s输入 'tools' 查看可用MCP工具和健康状态%s\n", ColorYellow, ColorReset)
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
	fmt.Printf("%s=== 弹性MCP服务器能力总览 ===%s\n", ColorCyan, ColorReset)

	if len(mcpServers) == 0 {
		fmt.Printf("%s当前无可用MCP服务器%s\n", ColorGray, ColorReset)
		return
	}

	ctx := context.Background()
	totalTools := 0
	healthyServers := 0

	for i, server := range mcpServers {
		fmt.Printf("\n%s[弹性MCP服务器 %d]%s\n", ColorYellow, i+1, ColorReset)

		// 检查是否是ResilientMCPServer并显示健康状态
		if resilientServer, ok := server.(*ResilientMCPServer); ok {
			if resilientServer.IsHealthy() {
				fmt.Printf("%s  健康状态: ✅ 正常%s\n", ColorGreen, ColorReset)
				healthyServers++
			} else {
				fmt.Printf("%s  健康状态: ⚠️ 异常（自动重连中）%s\n", ColorYellow, ColorReset)
			}
		}

		// 显示工具详细信息
		tools, err := server.ListTools(ctx)
		if err != nil {
			fmt.Printf("%s  工具获取失败: %v%s\n", ColorRed, err, ColorReset)
			fmt.Printf("%s  提示: 弹性服务器将自动尝试重连%s\n", ColorGray, ColorReset)
		} else {
			totalTools += len(tools)
			fmt.Printf("%s  工具 (%d个):%s\n", ColorGreen, len(tools), ColorReset)
			for j, tool := range tools {
				showDetailedToolInfo(j+1, tool)
			}
		}

		// MCP服务器当前只支持工具，不支持提示词
	}

	fmt.Printf("\n%s总计: %d个MCP服务器, %d个工具, %d个健康服务器%s\n", ColorCyan, len(mcpServers), totalTools, healthyServers, ColorReset)
	fmt.Printf("%s弹性特性: 自动重连、健康监控、故障恢复%s\n", ColorGray, ColorReset)
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

// MCPHealthManager - SSE连接健康管理器
type MCPHealthManager struct {
	server      interfaces.MCPServer
	baseURL     string
	isHealthy   atomic.Bool
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	reconnectCh chan struct{}
}

// NewMCPHealthManager 创建健康管理器
func NewMCPHealthManager(server interfaces.MCPServer, baseURL string) *MCPHealthManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &MCPHealthManager{
		server:      server,
		baseURL:     baseURL,
		ctx:         ctx,
		cancel:      cancel,
		reconnectCh: make(chan struct{}, 1),
	}
	m.isHealthy.Store(true)
	return m
}

// Start 启动健康检查
func (m *MCPHealthManager) Start() {
	go m.healthCheckLoop()
	fmt.Printf("%s🔄 MCP健康管理器已启动%s\n", ColorGreen, ColorReset)
}

// Stop 停止健康检查
func (m *MCPHealthManager) Stop() {
	m.cancel()
	close(m.reconnectCh)
}

// IsHealthy 检查连接是否健康
func (m *MCPHealthManager) IsHealthy() bool {
	return m.isHealthy.Load()
}

// healthCheckLoop 健康检查循环
func (m *MCPHealthManager) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performHealthCheck()
		case <-m.ctx.Done():
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (m *MCPHealthManager) performHealthCheck() {
	// 使用轻量级的ListTools调用检查连接状态
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.server.ListTools(ctx)
	if err != nil {
		if m.isHealthy.Load() {
			fmt.Printf("%s⚠️ MCP连接不健康: %v，触发重连...%s\n", ColorYellow, err, ColorReset)
			m.isHealthy.Store(false)
			m.triggerReconnect()
		}
	} else {
		if !m.isHealthy.Load() {
			fmt.Printf("%s✅ MCP连接已恢复健康%s\n", ColorGreen, ColorReset)
		}
		m.isHealthy.Store(true)
	}
}

// triggerReconnect 触发重连
func (m *MCPHealthManager) triggerReconnect() {
	go m.reconnectLoop()
}

// reconnectLoop 重连循环，使用指数退避
func (m *MCPHealthManager) reconnectLoop() {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	maxRetries := 10
	retryCount := 0

	for !m.isHealthy.Load() && retryCount < maxRetries {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		retryCount++
		fmt.Printf("%s🔄 尝试重连MCP服务器 (第%d次)...%s\n", ColorYellow, retryCount, ColorReset)

		// 重新创建MCP服务器连接
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		newServer, err := mcp.NewHTTPServer(ctx, mcp.HTTPServerConfig{
			BaseURL: m.baseURL,
		})
		cancel()

		if err == nil {
			// 测试新连接
			testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, testErr := newServer.ListTools(testCtx)
			testCancel()

			if testErr == nil {
				// 更新服务器引用
				m.mu.Lock()
				m.server = newServer
				m.mu.Unlock()
				m.isHealthy.Store(true)
				fmt.Printf("%s✅ MCP服务器重连成功%s\n", ColorGreen, ColorReset)

				// 通知等待的调用者
				select {
				case m.reconnectCh <- struct{}{}:
				default:
				}
				return
			}
		}

		// 等待后重试
		time.Sleep(backoff)
		if backoff < maxBackoff {
			backoff *= 2
		}
	}

	if !m.isHealthy.Load() {
		fmt.Printf("%s❌ MCP服务器重连失败，已达到最大重试次数%s\n", ColorRed, ColorReset)
	}
}

// GetServer 获取健康的服务器实例
func (m *MCPHealthManager) GetServer() (interfaces.MCPServer, error) {
	if m.isHealthy.Load() {
		m.mu.RLock()
		server := m.server
		m.mu.RUnlock()
		return server, nil
	}

	// 等待重连完成
	select {
	case <-m.reconnectCh:
		if m.isHealthy.Load() {
			m.mu.RLock()
			server := m.server
			m.mu.RUnlock()
			return server, nil
		}
	case <-time.After(10 * time.Second):
		return nil, errors.New("MCP服务器重连超时")
	case <-m.ctx.Done():
		return nil, errors.New("MCP健康管理器已停止")
	}

	return nil, errors.New("MCP服务器不可用")
}

// ResilientMCPServer - 具备自动恢复能力的MCP服务器包装
type ResilientMCPServer struct {
	healthManager *MCPHealthManager
}

// NewResilientMCPServer 创建弹性MCP服务器
func NewResilientMCPServer(server interfaces.MCPServer, baseURL string) *ResilientMCPServer {
	healthManager := NewMCPHealthManager(server, baseURL)
	healthManager.Start()

	return &ResilientMCPServer{
		healthManager: healthManager,
	}
}

// Initialize 实现MCPServer接口
func (r *ResilientMCPServer) Initialize(ctx context.Context) error {
	server, err := r.healthManager.GetServer()
	if err != nil {
		return err
	}
	return server.Initialize(ctx)
}

// ListTools 实现MCPServer接口 - 带自动重连
func (r *ResilientMCPServer) ListTools(ctx context.Context) ([]interfaces.MCPTool, error) {
	server, err := r.healthManager.GetServer()
	if err != nil {
		return nil, err
	}

	tools, err := server.ListTools(ctx)
	if err != nil {
		// 工具调用失败，可能是连接问题，标记为不健康
		r.healthManager.isHealthy.Store(false)
		r.healthManager.triggerReconnect()
		return nil, err
	}

	return tools, nil
}

// CallTool 实现MCPServer接口 - 带自动重连
func (r *ResilientMCPServer) CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error) {
	server, err := r.healthManager.GetServer()
	if err != nil {
		return nil, err
	}

	result, err := server.CallTool(ctx, name, args)
	if err != nil {
		// 工具调用失败，可能是连接问题，标记为不健康
		r.healthManager.isHealthy.Store(false)
		r.healthManager.triggerReconnect()
		return nil, err
	}

	return result, nil
}

// Close 实现MCPServer接口
func (r *ResilientMCPServer) Close() error {
	r.healthManager.Stop()
	server, err := r.healthManager.GetServer()
	if err != nil {
		return nil // 如果获取不到server，说明已经关闭了
	}
	return server.Close()
}

// IsHealthy 检查服务器健康状态
func (r *ResilientMCPServer) IsHealthy() bool {
	return r.healthManager.IsHealthy()
}
