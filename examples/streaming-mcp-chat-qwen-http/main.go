package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"github.com/gin-gonic/gin"
)

// === 完全复用千问版本的SessionMCPManager ===
// SessionMCPManager - 会话级MCP连接管理器
// 特性：连接复用 + 健康检查
type SessionMCPManager struct {
	baseURL       string
	connection    interfaces.MCPServer
	lastActivity  time.Time    // 最后活动时间
	sessionActive bool         // 会话是否活跃
	mutex         sync.RWMutex // 读写锁
}

// NewSessionMCPManager 创建会话级MCP管理器
func NewSessionMCPManager(baseURL string) *SessionMCPManager {
	return &SessionMCPManager{
		baseURL: baseURL,
		mutex:   sync.RWMutex{},
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
	fmt.Printf("[SessionMCP] 创建新连接...\n")

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
	fmt.Printf("[SessionMCP] 连接已清理\n")
}

// ensureConnection 确保有活跃的MCP连接（使用时验证）
func (s *SessionMCPManager) ensureConnection(ctx context.Context) (interfaces.MCPServer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 检查现有连接的有效性
	if s.connection != nil && s.sessionActive {
		// 时间检查：超过2分钟自动重建
		if time.Since(s.lastActivity) > 2*time.Minute {
			fmt.Printf("[SessionMCP] 连接超时(2分钟)，重建连接\n")
			s.cleanupConnection()
		} else {
			// 健康检查：验证连接可用性
			if s.isConnectionAlive() {
				s.lastActivity = time.Now()
				fmt.Printf("[SessionMCP] 复用现有连接\n")
				return s.connection, nil
			} else {
				fmt.Printf("[SessionMCP] 连接失效，重建连接\n")
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
			// 创建新的工具对象，使用转换后的schema
			return interfaces.MCPTool{
				Name:        tool.Name,
				Description: tool.Description,
				Schema:      schemaMap, // 使用转换后的map格式
			}
		}
	}

	// 如果转换失败，返回原始工具
	return tool
}

// CallTool 实现MCPServer接口 - 会话连接复用（无缓存）
func (s *SessionMCPManager) CallTool(ctx context.Context, name string, args interface{}) (*interfaces.MCPToolResponse, error) {
	fmt.Printf("[SessionMCP] 调用工具: %s\n", name)

	// 获取会话连接
	server, err := s.ensureConnection(ctx)
	if err != nil {
		return nil, err
	}

	// 执行工具调用
	response, err := server.CallTool(ctx, name, args)
	if err != nil {
		return nil, err
	}

	// 更新活动时间
	s.mutex.Lock()
	s.lastActivity = time.Now()
	s.mutex.Unlock()

	fmt.Printf("[SessionMCP] 工具调用完成: %s\n", name)
	return response, nil
}

// Close 实现MCPServer接口 - 手动清理会话连接
func (s *SessionMCPManager) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	fmt.Printf("[SessionMCP] 手动关闭会话连接\n")
	s.cleanupConnection()
	return nil
}

// === HTTP API 相关结构 ===
type ChatRequest struct {
	Message string `json:"message" binding:"required"`
}

type SSEEvent struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Events  int    `json:"events,omitempty"`
}

// === 全局变量 ===
var (
	agentInstance  *agent.Agent
	sessionManager *SessionMCPManager
)

// initAgent 完全复用千问版本的智能体初始化逻辑
func initAgent() error {
	// 创建日志器
	logger := logging.New()

	// 创建千问客户端配置 - 完全与千问版本一致
	apiKey := "sk-0d8bebab081044f682fbeb6c147d8f2c" // 千问API密钥
	modelName := "qwen-max"                         // 千问最强模型
	baseURL := "https://dashscope.aliyuncs.com/compatible-mode/v1"

	fmt.Printf("使用千问模型: %s (支持工具调用)\n", modelName)
	fmt.Printf("连接到: %s\n", baseURL)

	qwenClient := openai.NewClient(apiKey,
		openai.WithBaseURL(baseURL),
		openai.WithModel(modelName),
		openai.WithLogger(logger))

	// 创建工具注册器 - 保持streaming-chat原有结构
	toolRegistry := tools.NewRegistry()

	// === MCP 按需连接配置 - 完全复用千问版本逻辑 ===
	fmt.Printf("=== MCP按需连接配置 ===\n")
	var mcpServers []interfaces.MCPServer

	// 配置会话级MCP管理器（连接复用 + 调用去重）
	mcpURL := "http://sn.7soft.cn/sse"
	fmt.Printf("配置会话级MCP管理器: %s\n", mcpURL)

	// 创建会话级MCP管理器（一个会话回合 = 一个连接 + 去重）
	sessionManager = NewSessionMCPManager(mcpURL)
	mcpServers = append(mcpServers, sessionManager)
	fmt.Printf("✅ 会话级MCP管理器配置完成（连接复用+去重）\n")

	// 测试连接以验证配置正确性
	fmt.Printf("正在测试连接和工具发现...\n")
	tools, err := sessionManager.ListTools(context.Background())
	if err != nil {
		fmt.Printf("Warning: 测试连接失败: %v\n", err)
	} else {
		fmt.Printf("发现 %d 个MCP工具:\n", len(tools))
		for i, tool := range tools {
			fmt.Printf("  [%d] %s: %s\n", i+1, tool.Name, tool.Description)
		}
	}

	// === 创建智能体 - 完全复用千问版本逻辑 ===
	if len(mcpServers) > 0 {
		// 有MCP服务器时，使用WithMCPServers
		// 千问DashScope API对工具消息格式要求严格，限制记忆大小避免格式问题
		fmt.Printf("创建MCP智能体 (连接 %d 个MCP服务器)...\n", len(mcpServers))
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(qwenClient),
			agent.WithMemory(memory.NewConversationBuffer(memory.WithMaxSize(3))), // 限制记忆大小避免工具消息格式问题
			agent.WithTools(toolRegistry.List()...),
			agent.WithMCPServers(mcpServers),
			agent.WithRequirePlanApproval(false), // 自动执行工具，不需要审批
			agent.WithSystemPrompt("你是一个有用的AI助手，使用中文回答问题。你可以使用各种MCP工具来帮助回答问题，请根据用户问题智能选择和调用合适的工具。当你需要获取实时信息（如时间）或执行特定任务时，请主动使用相关工具。"),
			agent.WithMaxIterations(5),
			agent.WithName("AIBodyQwenHTTPAssistant"),
		)
	} else {
		// 没有MCP服务器时，使用基础配置（完全兼容streaming-chat）
		fmt.Printf("创建基础智能体 (无MCP支持)...\n")
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(qwenClient),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithTools(toolRegistry.List()...),
			agent.WithSystemPrompt("你是一个有用的AI助手，使用中文回答问题。请提供详细和有帮助的回答。"),
			agent.WithMaxIterations(5),
			agent.WithName("AIBodyQwenHTTPAssistant"),
		)
	}

	if err != nil {
		return fmt.Errorf("创建智能体失败: %w", err)
	}

	return nil
}

// handleChat 处理聊天请求 - 复用千问版本的流式处理逻辑
func handleChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求格式"})
		return
	}

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// 创建上下文 - 完全复用千问版本的上下文创建方式
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "ai-body-streaming-mcp-demo")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, fmt.Sprintf("http-session-%d", time.Now().Unix()))

	// === 完全保持千问版本的流式处理逻辑 ===
	// 尝试使用流式传输
	eventChan, err := agentInstance.RunStream(ctx, req.Message)
	if err != nil {
		// 如果流式传输不支持，使用普通模式
		response, normalErr := agentInstance.Run(ctx, req.Message)
		if normalErr != nil {
			event := SSEEvent{Type: "error", Content: fmt.Sprintf("处理失败: %v", normalErr)}
			data, _ := json.Marshal(event)
			c.SSEvent("", string(data))
			c.Writer.Flush()
			return
		}

		// 发送完整响应
		event := SSEEvent{Type: "content", Content: response}
		data, _ := json.Marshal(event)
		c.SSEvent("", string(data))

		doneEvent := SSEEvent{Type: "done", Events: 1}
		doneData, _ := json.Marshal(doneEvent)
		c.SSEvent("", string(doneData))
		c.Writer.Flush()
		return
	}

	// 处理真实的流式事件 - 完全复用千问版本的事件处理逻辑
	var responseText strings.Builder
	eventCount := 0
	contentEvents := 0

	for event := range eventChan {
		eventCount++

		// 只显示有内容的事件，忽略调试信息 - 与千问版本一致
		if event.Content != "" {
			sseEvent := SSEEvent{Type: "content", Content: event.Content}
			data, _ := json.Marshal(sseEvent)
			c.SSEvent("", string(data))
			c.Writer.Flush()

			responseText.WriteString(event.Content)
			contentEvents++
		}
	}

	// 发送完成事件
	doneEvent := SSEEvent{Type: "done", Events: eventCount}
	doneData, _ := json.Marshal(doneEvent)
	c.SSEvent("", string(doneData))
	c.Writer.Flush()
}

// handleHealth 健康检查
func handleHealth(c *gin.Context) {
	// 检查MCP连接状态
	mcpStatus := "disconnected"
	if sessionManager != nil {
		if sessionManager.isConnectionAlive() {
			mcpStatus = "connected"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "healthy",
		"service":    "AI-Body 千问 HTTP API",
		"mcp_status": mcpStatus,
		"features":   []string{"streaming", "mcp_tools", "session_management"},
	})
}

// handleTools 获取可用工具列表
func handleTools(c *gin.Context) {
	if sessionManager == nil {
		c.JSON(http.StatusOK, gin.H{"tools": []interface{}{}, "count": 0})
		return
	}

	tools, err := sessionManager.ListTools(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取工具失败: %v", err)})
		return
	}

	// 简化工具信息
	simplifiedTools := make([]map[string]interface{}, len(tools))
	for i, tool := range tools {
		simplifiedTools[i] = map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": simplifiedTools,
		"count": len(tools),
	})
}

func main() {
	// 初始化智能体
	fmt.Println("🚀 初始化AI助手（基于千问版本）...")
	if err := initAgent(); err != nil {
		fmt.Printf("❌ 初始化失败: %v\n", err)
		return
	}
	fmt.Println("✅ AI助手初始化完成")

	// 创建Gin引擎
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 添加CORS中间件
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 路由配置
	r.POST("/chat", handleChat)
	r.GET("/health", handleHealth)
	r.GET("/tools", handleTools)

	// 启动服务器
	port := "8080"
	fmt.Printf("\n🌐 HTTP API 服务启动在: http://localhost:%s\n", port)
	fmt.Printf("📡 聊天端点: POST http://localhost:%s/chat\n", port)
	fmt.Printf("🛠️  工具查看: GET http://localhost:%s/tools\n", port)
	fmt.Printf("❤️  健康检查: GET http://localhost:%s/health\n", port)
	fmt.Println("\n基于千问版本，完整复用SessionMCPManager和流式处理逻辑")

	if err := r.Run(":" + port); err != nil {
		fmt.Printf("❌ 服务启动失败: %v\n", err)
	}
}
