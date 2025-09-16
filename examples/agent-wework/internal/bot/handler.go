package bot

import (
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/config"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/wework"
)

// BotHandler 机器人处理器
type BotHandler struct {
	config        *config.WeWorkConfig
	agentInstance *agent.Agent
	streamManager *wework.StreamManager
	sessionMCP    *SessionMCPManager
}

// NewBotHandler 创建机器人处理器
func NewBotHandler(cfg *config.WeWorkConfig) (*BotHandler, error) {
	handler := &BotHandler{
		config:        cfg,
		streamManager: wework.NewStreamManager(),
	}

	if err := handler.initAgent(); err != nil {
		return nil, fmt.Errorf("failed to initialize agent: %w", err)
	}

	return handler, nil
}

// Close 关闭机器人处理器
func (b *BotHandler) Close() {
	if b.streamManager != nil {
		b.streamManager.Close()
	}
	if b.sessionMCP != nil {
		b.sessionMCP.Close()
	}
}

// initAgent 初始化智能体 - 完全复用qwen-http版本逻辑
func (b *BotHandler) initAgent() error {
	logger := logging.New()

	// 创建千问客户端配置 - 完全与qwen-http版本一致
	fmt.Printf("🤖 使用千问模型: %s (支持工具调用)\n", b.config.QwenModel)
	fmt.Printf("🔗 连接到: %s\n", b.config.QwenBaseURL)

	qwenClient := openai.NewClient(b.config.QwenAPIKey,
		openai.WithBaseURL(b.config.QwenBaseURL),
		openai.WithModel(b.config.QwenModel),
		openai.WithLogger(logger))

	// 创建工具注册器
	toolRegistry := tools.NewRegistry()

	// === MCP 按需连接配置 - 完全复用qwen-http版本逻辑 ===
	fmt.Printf("=== MCP按需连接配置 ===\n")
	var mcpServers []interfaces.MCPServer

	// 配置会话级MCP管理器
	fmt.Printf("🔧 配置会话级MCP管理器: %s\n", b.config.MCPServerURL)

	// 创建会话级MCP管理器（完全复用qwen-http版本实现）
	b.sessionMCP = NewSessionMCPManager(b.config.MCPServerURL)
	mcpServers = append(mcpServers, b.sessionMCP)
	fmt.Printf("✅ 会话级MCP管理器配置完成（连接复用+去重）\n")

	// 测试连接以验证配置正确性
	fmt.Printf("🔍 正在测试连接和工具发现...\n")
	tools, err := b.sessionMCP.ListTools(context.Background())
	if err != nil {
		fmt.Printf("⚠️  Warning: 测试连接失败: %v\n", err)
	} else {
		fmt.Printf("✅ 发现 %d 个MCP工具:\n", len(tools))
		for i, tool := range tools {
			fmt.Printf("  [%d] %s: %s\n", i+1, tool.Name, tool.Description)
		}
	}

	// === 创建智能体 - 完全复用qwen-http版本逻辑 ===
	var agentErr error
	if len(mcpServers) > 0 {
		fmt.Printf("🚀 创建MCP智能体 (连接 %d 个MCP服务器)...\n", len(mcpServers))
		b.agentInstance, agentErr = agent.NewAgent(
			agent.WithLLM(qwenClient),
			agent.WithMemory(memory.NewConversationBuffer(memory.WithMaxSize(3))), // 限制记忆大小避免工具消息格式问题
			agent.WithTools(toolRegistry.List()...),
			agent.WithMCPServers(mcpServers),
			agent.WithRequirePlanApproval(false), // 自动执行工具，不需要审批
			agent.WithSystemPrompt("你是一个企业微信智能助手，使用中文回答问题。你可以使用各种MCP工具来帮助回答问题，请根据用户问题智能选择和调用合适的工具。当你需要获取实时信息（如时间）或执行特定任务时，请主动使用相关工具。请保持回答简洁明了，适合企业微信聊天场景。"),
			agent.WithMaxIterations(5),
			agent.WithName("AIBodyWeWorkAssistant"),
		)
	} else {
		fmt.Printf("🚀 创建基础智能体 (无MCP支持)...\n")
		b.agentInstance, agentErr = agent.NewAgent(
			agent.WithLLM(qwenClient),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithTools(toolRegistry.List()...),
			agent.WithSystemPrompt("你是一个企业微信智能助手，使用中文回答问题。请提供详细和有帮助的回答，保持简洁明了。"),
			agent.WithMaxIterations(5),
			agent.WithName("AIBodyWeWorkAssistant"),
		)
	}

	return agentErr
}

// HandleMessage 处理普通消息
func (b *BotHandler) HandleMessage(msg *wework.IncomingMessage) (*wework.WeWorkResponse, error) {
	// 提取文本内容
	textContent := msg.GetTextContent()
	if textContent == "" {
		// 如果有图片但没有文本，提供默认提示
		if len(msg.GetImageURLs()) > 0 {
			return wework.NewTextResponse("我收到了您发送的图片，但目前暂不支持图片分析功能。您可以用文字描述问题，我来帮您解答。"), nil
		}
		return nil, nil // 无需回复
	}

	// 创建上下文
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "wework-org")
	ctx = context.WithValue(ctx, memory.ConversationIDKey, msg.GetConversationKey())

	fmt.Printf("🤖 处理消息: %s (来自: %s)\n", textContent, msg.From.UserID)

	// === 使用流式处理 - 严格按照qwen-http模式 ===
	eventChan, err := b.agentInstance.RunStream(ctx, textContent)
	if err != nil {
		// 流式传输失败，使用普通模式回退
		fmt.Printf("⚠️  流式传输不可用，回退到普通模式: %v\n", err)
		response, normalErr := b.agentInstance.Run(ctx, textContent)
		if normalErr != nil {
			return nil, fmt.Errorf("AI处理失败: %w", normalErr)
		}
		// 返回文本回复
		return wework.NewTextResponse(response), nil
	}

	// 创建流式状态
	streamID, err := b.streamManager.CreateStream()
	if err != nil {
		return nil, fmt.Errorf("创建流式状态失败: %w", err)
	}

	fmt.Printf("📡 开始流式传输: stream_id=%s\n", streamID)

	// 启动协程处理流式事件 - 完全复用qwen-http逻辑
	go func() {
		defer func() {
			// 标记流式传输完成
			b.streamManager.UpdateStreamContent(streamID, "", true)
			fmt.Printf("✅ 流式传输完成: %s\n", streamID)
		}()

		var responseText strings.Builder

		// 处理真实的流式事件 - 完全复用qwen-http版本的事件处理逻辑
		for event := range eventChan {
			// 只处理有内容的事件，忽略调试信息 - 与qwen-http版本一致
			if event.Content != "" {
				responseText.WriteString(event.Content)

				// 更新流式状态
				b.streamManager.UpdateStreamContent(streamID, responseText.String(), false)
				fmt.Printf("📡 流式更新: %s (长度: %d)\n", streamID, responseText.Len())
			}
		}
	}()

	// 立即返回流式开始消息
	return wework.NewStreamResponse(streamID, "", false), nil
}

// HandleStreamRefresh 处理流式消息刷新
func (b *BotHandler) HandleStreamRefresh(streamID string) (*wework.WeWorkResponse, error) {
	fmt.Printf("🔄 处理流式刷新: %s\n", streamID)

	// 获取流式状态
	state := b.streamManager.GetStream(streamID)
	if state == nil {
		// 流式状态不存在，可能已过期
		return wework.NewTextResponse("对话已结束，请发送新消息开始新的对话。"), nil
	}

	// 获取当前内容和状态
	content, isActive := state.GetStreamContent()

	if !isActive {
		// 流式结束，删除状态
		b.streamManager.DeleteStream(streamID)
		fmt.Printf("✅ 流式传输完成: %s\n", streamID)
		return wework.NewStreamResponse(streamID, content, true), nil
	}

	// 返回当前累积的内容
	fmt.Printf("📡 流式传输中: %s (长度: %d)\n", streamID, len(content))
	return wework.NewStreamResponse(streamID, content, false), nil
}

// GetActiveStreamCount 获取活跃流式数量
func (b *BotHandler) GetActiveStreamCount() int {
	return b.streamManager.GetActiveStreamCount()
}

// === 完全复用qwen-http版本的SessionMCPManager实现 ===

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
