package bot

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
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

// === 任务缓存管理器 - 模拟Python LLMDemo类 ===

// TaskInfo 任务信息
type TaskInfo struct {
	StreamID     string          `json:"stream_id"`
	Question     string          `json:"question"`
	CreatedTime  time.Time       `json:"created_time"`
	CurrentStep  int             `json:"current_step"`
	MaxSteps     int             `json:"max_steps"`
	Content      strings.Builder `json:"-"`             // 累积内容
	IsProcessing bool            `json:"is_processing"` // AI是否正在处理
	IsFinished   bool            `json:"is_finished"`   // 是否已完成
	LastUpdate   time.Time       `json:"last_update"`
	mutex        sync.RWMutex    `json:"-"`
}

// TaskCacheManager 任务缓存管理器 - 模拟Python LLMDemo
type TaskCacheManager struct {
	tasks         map[string]*TaskInfo
	mutex         sync.RWMutex
	agentInstance *agent.Agent // 用于执行AI处理
}

// NewTaskCacheManager 创建任务缓存管理器
func NewTaskCacheManager(agentInstance *agent.Agent) *TaskCacheManager {
	return &TaskCacheManager{
		tasks:         make(map[string]*TaskInfo),
		agentInstance: agentInstance,
	}
}

// Close 关闭任务缓存管理器
func (tcm *TaskCacheManager) Close() {
	tcm.mutex.Lock()
	defer tcm.mutex.Unlock()

	// 清理所有任务
	for id := range tcm.tasks {
		delete(tcm.tasks, id)
	}
	fmt.Printf("✅ 任务缓存管理器已关闭\n")
}

// generateTaskID 生成任务ID - 严格按照Python示例的_generate_random_string(10)
func generateTaskID() (string, error) {
	// Python: letters = string.ascii_letters + string.digits
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 10 // Python固定生成10位

	result := make([]byte, length)
	for i := range result {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		result[i] = letters[randomIndex.Int64()]
	}

	return string(result), nil
}

// Invoke 创建新任务 - 模拟Python LLMDemo.invoke()
func (tcm *TaskCacheManager) Invoke(ctx context.Context, question string) (string, error) {
	streamID, err := generateTaskID()
	if err != nil {
		return "", fmt.Errorf("生成任务ID失败: %w", err)
	}

	// 创建任务信息
	task := &TaskInfo{
		StreamID:     streamID,
		Question:     question,
		CreatedTime:  time.Now(),
		CurrentStep:  0,
		MaxSteps:     10, // 模拟Python的MAX_STEPS = 10
		IsProcessing: false,
		IsFinished:   false,
		LastUpdate:   time.Now(),
	}

	tcm.mutex.Lock()
	tcm.tasks[streamID] = task
	tcm.mutex.Unlock()

	fmt.Printf("📋 创建任务: streamID=%s, question=%s\n", streamID, question)

	// 启动异步AI处理（模拟Python的后台处理）
	go tcm.processTaskAsync(ctx, streamID)

	return streamID, nil
}

// processTaskAsync 异步处理任务
func (tcm *TaskCacheManager) processTaskAsync(ctx context.Context, streamID string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("❌ 任务处理异常: streamID=%s, error=%v\n", streamID, r)
		}
	}()

	tcm.mutex.RLock()
	task, exists := tcm.tasks[streamID]
	tcm.mutex.RUnlock()

	if !exists {
		fmt.Printf("❌ 任务不存在: %s\n", streamID)
		return
	}

	task.mutex.Lock()
	task.IsProcessing = true
	task.LastUpdate = time.Now()
	task.mutex.Unlock()

	fmt.Printf("🚀 开始异步AI处理: streamID=%s\n", streamID)

	// 调用Agent进行流式处理
	events, err := tcm.agentInstance.RunStream(ctx, task.Question)
	if err != nil {
		fmt.Printf("❌ Agent运行失败: streamID=%s, error=%v\n", streamID, err)

		task.mutex.Lock()
		task.IsProcessing = false
		task.IsFinished = true
		task.Content.WriteString(fmt.Sprintf("处理失败: %v", err))
		task.LastUpdate = time.Now()
		task.mutex.Unlock()
		return
	}

	// 接收并累积AI事件流
	for event := range events {
		task.mutex.Lock()
		if event.Content != "" {
			task.Content.WriteString(event.Content)
			task.LastUpdate = time.Now()
		}
		task.mutex.Unlock()
	}

	// AI处理完成
	task.mutex.Lock()
	task.IsProcessing = false
	task.IsFinished = true
	task.CurrentStep = task.MaxSteps // 设置为最大步数表示完成
	task.LastUpdate = time.Now()
	task.mutex.Unlock()

	fmt.Printf("✅ AI处理完成: streamID=%s\n", streamID)
}

// GetAnswer 获取当前答案 - 模拟Python LLMDemo.get_answer()
func (tcm *TaskCacheManager) GetAnswer(streamID string) string {
	tcm.mutex.RLock()
	task, exists := tcm.tasks[streamID]
	tcm.mutex.RUnlock()

	if !exists {
		return "任务不存在或已过期"
	}

	task.mutex.Lock()
	defer task.mutex.Unlock()

	// 更新步数（模拟Python的递增逻辑）
	if !task.IsFinished && task.CurrentStep < task.MaxSteps {
		task.CurrentStep++
		task.LastUpdate = time.Now()
	}

	// 构造返回内容（模拟Python的格式）
	response := fmt.Sprintf("收到问题：%s\n\n", task.Question)

	// 如果AI还在处理，显示进度
	if task.IsProcessing && task.Content.Len() == 0 {
		for i := 0; i < task.CurrentStep; i++ {
			response += fmt.Sprintf("处理步骤 %d: 准备中...\n", i+1)
		}
	} else {
		// AI有内容输出，显示实际内容
		if task.Content.Len() > 0 {
			response += "AI回复:\n" + task.Content.String()
		} else {
			// 还没有内容，显示进度
			for i := 0; i < task.CurrentStep; i++ {
				response += fmt.Sprintf("处理步骤 %d: 已完成\n", i+1)
			}
		}
	}

	fmt.Printf("📊 获取答案: streamID=%s, step=%d/%d, 内容长度=%d\n",
		streamID, task.CurrentStep, task.MaxSteps, len(response))

	return response
}

// IsTaskFinish 检查任务是否完成 - 模拟Python LLMDemo.is_task_finish()
func (tcm *TaskCacheManager) IsTaskFinish(streamID string) bool {
	tcm.mutex.RLock()
	task, exists := tcm.tasks[streamID]
	tcm.mutex.RUnlock()

	if !exists {
		return true // 任务不存在视为已完成
	}

	task.mutex.RLock()
	defer task.mutex.RUnlock()

	// 满足以下条件之一视为完成：
	// 1. 明确标记为已完成
	// 2. 步数达到最大值
	// 3. AI处理完成且有内容输出
	isFinished := task.IsFinished ||
		task.CurrentStep >= task.MaxSteps ||
		(!task.IsProcessing && task.Content.Len() > 0)

	fmt.Printf("🔍 检查任务完成状态: streamID=%s, finished=%v, step=%d/%d, processing=%v\n",
		streamID, isFinished, task.CurrentStep, task.MaxSteps, task.IsProcessing)

	return isFinished
}

// BotHandler 机器人处理器
type BotHandler struct {
	config        *config.WeWorkConfig
	agentInstance *agent.Agent
	taskCache     *TaskCacheManager
	sessionMCP    *SessionMCPManager
}

// NewBotHandler 创建机器人处理器
func NewBotHandler(cfg *config.WeWorkConfig) (*BotHandler, error) {
	handler := &BotHandler{
		config: cfg,
	}

	if err := handler.initAgent(); err != nil {
		return nil, fmt.Errorf("failed to initialize agent: %w", err)
	}

	// 初始化任务缓存管理器
	handler.taskCache = NewTaskCacheManager(handler.agentInstance)
	fmt.Printf("✅ 任务缓存管理器已初始化\n")

	return handler, nil
}

// Close 关闭机器人处理器
func (b *BotHandler) Close() {
	if b.taskCache != nil {
		b.taskCache.Close()
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

	fmt.Printf("🤖 收到text消息: %s (来自: %s)\n", textContent, msg.From.UserID)

	// === 严格按照Python示例流程处理text消息 ===
	fmt.Printf("📋 按照Python示例创建任务...\n")

	// 1. 创建任务（模拟Python LLMDemo.invoke()）
	streamID, err := b.taskCache.Invoke(ctx, textContent)
	if err != nil {
		fmt.Printf("❌ 创建任务失败: %v\n", err)
		return wework.NewTextResponse("系统忙，请稍后再试"), err
	}

	// 2. 获取第一步答案（模拟Python LLMDemo.get_answer()）
	answer := b.taskCache.GetAnswer(streamID)

	// 3. 检查是否完成（模拟Python LLMDemo.is_task_finish()）
	finish := b.taskCache.IsTaskFinish(streamID)

	fmt.Printf("📡 返回stream消息: streamID=%s, finish=%v, 内容长度=%d\n",
		streamID, finish, len(answer))

	// 4. 返回stream消息（模拟Python MakeTextStream + EncryptMessage）
	// 关键：finish=false时企业微信会发送刷新请求！
	return wework.NewStreamResponse(streamID, answer, finish), nil
}

// HandleStreamRefresh 处理流式消息刷新 - 模拟Python示例的stream消息处理
func (b *BotHandler) HandleStreamRefresh(streamID string) (*wework.WeWorkResponse, error) {
	fmt.Printf("🔄 收到stream刷新请求: %s\n", streamID)

	// === 严格按照Python示例流程处理stream消息 ===
	fmt.Printf("📋 按照Python示例处理stream刷新...\n")

	// 1. 获取最新答案（模拟Python LLMDemo.get_answer()）
	answer := b.taskCache.GetAnswer(streamID)

	// 2. 检查是否完成（模拟Python LLMDemo.is_task_finish()）
	finish := b.taskCache.IsTaskFinish(streamID)

	fmt.Printf("📡 stream刷新结果: streamID=%s, finish=%v, 内容长度=%d\n",
		streamID, finish, len(answer))

	// 3. 返回stream消息（模拟Python MakeTextStream + EncryptMessage）
	// 继续返回，直到finish=true为止
	return wework.NewStreamResponse(streamID, answer, finish), nil
}

// GetActiveStreamCount 获取活跃任务数量
func (b *BotHandler) GetActiveStreamCount() int {
	if b.taskCache == nil {
		return 0
	}

	b.taskCache.mutex.RLock()
	defer b.taskCache.mutex.RUnlock()

	count := 0
	for _, task := range b.taskCache.tasks {
		task.mutex.RLock()
		if !task.IsFinished {
			count++
		}
		task.mutex.RUnlock()
	}

	return count
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
