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

// === 真正的流式传输架构 - 生产者消费者模式 ===

// StreamBuffer 流式内容缓冲区 - 实现累积模式（按照Python示例）
type StreamBuffer struct {
	chunks     []string     // 所有内容块（累积存储，不移除）
	mutex      sync.RWMutex // 线程安全锁
	aiFinished bool         // AI是否完成生成
	lastIndex  int          // 最后返回的块索引（模拟Python的current_step）
	lastUpdate time.Time    // 最后更新时间
}

// NewStreamBuffer 创建流式缓冲区
func NewStreamBuffer() *StreamBuffer {
	return &StreamBuffer{
		chunks:     make([]string, 0),
		lastUpdate: time.Now(),
	}
}

// Push AI生产内容到缓冲区
func (sb *StreamBuffer) Push(content string) {
	if content == "" {
		return
	}

	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	sb.chunks = append(sb.chunks, content)
	sb.lastUpdate = time.Now()
}

// GetAccumulated 获取累积内容（严格按照Python的get_answer逻辑）
func (sb *StreamBuffer) GetAccumulated() (string, bool) {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	// 检查是否有新内容可以展示
	if sb.lastIndex < len(sb.chunks) {
		// 模拟Python的current_step += 1
		sb.lastIndex++
		sb.lastUpdate = time.Now()

		// 构建累积内容（从第0块到lastIndex-1块）
		var accumulated strings.Builder
		for i := 0; i < sb.lastIndex; i++ {
			accumulated.WriteString(sb.chunks[i])
		}

		content := accumulated.String()
		return content, false // 有内容，未完成
	}

	// 无新内容，检查AI是否完成
	isFinished := sb.aiFinished
	return "", isFinished // 无内容，返回完成状态
}

// SetAIFinished 标记AI完成生成
func (sb *StreamBuffer) SetAIFinished() {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	sb.aiFinished = true
	sb.lastUpdate = time.Now()
}

// IsEmpty 检查是否还有未展示的内容
func (sb *StreamBuffer) IsEmpty() bool {
	sb.mutex.RLock()
	defer sb.mutex.RUnlock()

	// 累积模式：检查是否所有内容都已展示
	return sb.lastIndex >= len(sb.chunks)
}

// IsAIFinished 检查AI是否完成
func (sb *StreamBuffer) IsAIFinished() bool {
	sb.mutex.RLock()
	defer sb.mutex.RUnlock()

	return sb.aiFinished
}

// GetStatus 获取缓冲区状态（用于调试）
func (sb *StreamBuffer) GetStatus() (totalChunks int, displayedChunks int, aiFinished bool) {
	sb.mutex.RLock()
	defer sb.mutex.RUnlock()

	return len(sb.chunks), sb.lastIndex, sb.aiFinished
}

// TaskInfo 任务信息 - 基于StreamBuffer的真正流式架构
type TaskInfo struct {
	StreamID     string        `json:"stream_id"`
	Question     string        `json:"question"`
	CreatedTime  time.Time     `json:"created_time"`
	Buffer       *StreamBuffer `json:"-"`             // 流式缓冲区（替换累积内容）
	IsProcessing bool          `json:"is_processing"` // AI是否正在处理
	LastUpdate   time.Time     `json:"last_update"`
	mutex        sync.RWMutex  `json:"-"`

	// ❌ 已移除的累积模式字段：
	// CurrentStep  int             - 不再需要固定步数
	// MaxSteps     int             - 不再需要最大步数限制
	// Content      strings.Builder - 不再累积内容，改为缓冲区
	// IsFinished   bool            - 通过Buffer.IsAIFinished()获取
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
	// 任务缓存管理器已关闭
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

	// 创建任务信息 - 基于StreamBuffer的真正流式架构
	task := &TaskInfo{
		StreamID:     streamID,
		Question:     question,
		CreatedTime:  time.Now(),
		Buffer:       NewStreamBuffer(), // ✅ 创建流式缓冲区
		IsProcessing: false,
		LastUpdate:   time.Now(),
	}

	tcm.mutex.Lock()
	tcm.tasks[streamID] = task
	tcm.mutex.Unlock()

	// 启动异步AI处理（模拟Python的后台处理）
	go tcm.processTaskAsync(ctx, streamID)

	return streamID, nil
}

// processTaskAsync 异步处理任务
func (tcm *TaskCacheManager) processTaskAsync(ctx context.Context, streamID string) {
	defer func() {
		if r := recover(); r != nil {
			// 任务处理异常
		}
	}()

	tcm.mutex.RLock()
	task, exists := tcm.tasks[streamID]
	tcm.mutex.RUnlock()

	if !exists {
		// 任务不存在
		return
	}

	task.mutex.Lock()
	task.IsProcessing = true
	task.LastUpdate = time.Now()
	task.mutex.Unlock()

	// ✅ 关键修改：使用streamID作为conversation ID，确保每个任务独立
	// 这样可以避免同一用户的不同问题之间的memory污染
	ctx = context.WithValue(ctx, memory.ConversationIDKey, streamID)

	// 调用Agent进行流式处理
	events, err := tcm.agentInstance.RunStream(ctx, task.Question)
	if err != nil {

		// 推送错误信息到缓冲区
		errorMsg := fmt.Sprintf("处理失败: %v", err)
		task.Buffer.Push(errorMsg)
		task.Buffer.SetAIFinished() // 标记AI完成（错误情况）

		task.mutex.Lock()
		task.IsProcessing = false
		task.LastUpdate = time.Now()
		task.mutex.Unlock()
		return
	}

	// ✅ 关键改造：从累积模式改为推送模式
	// AI生成内容实时推送到StreamBuffer，供企业微信消费
	for event := range events {
		if event.Content != "" {
			// 推送到缓冲区（生产者模式）
			task.Buffer.Push(event.Content)

			task.mutex.Lock()
			task.LastUpdate = time.Now()
			task.mutex.Unlock()
		}
	}

	// AI处理完成，标记缓冲区状态
	task.mutex.Lock()
	task.IsProcessing = false
	task.LastUpdate = time.Now()
	task.mutex.Unlock()

	// ✅ 标记AI完成生成（但可能还有内容在缓冲区等待消费）
	task.Buffer.SetAIFinished()
}

// GetAnswer 获取当前答案 - 真正的流式消费模式
func (tcm *TaskCacheManager) GetAnswer(streamID string) string {
	tcm.mutex.RLock()
	task, exists := tcm.tasks[streamID]
	tcm.mutex.RUnlock()

	if !exists {
		return "任务不存在或已过期"
	}

	// ✅ 核心改造：获取累积内容（严格按照Python示例）
	accumulatedContent, _ := task.Buffer.GetAccumulated()

	// 更新任务状态
	task.mutex.Lock()
	task.LastUpdate = time.Now()
	task.mutex.Unlock()

	// ✅ 关键：返回累积的完整内容（企业微信用此替换整个消息）
	return accumulatedContent
}

// IsTaskFinish 检查任务是否完成 - 基于StreamBuffer的真正流式架构
func (tcm *TaskCacheManager) IsTaskFinish(streamID string) bool {
	tcm.mutex.RLock()
	task, exists := tcm.tasks[streamID]
	tcm.mutex.RUnlock()

	if !exists {
		return true // 任务不存在视为已完成
	}

	task.mutex.RLock()
	defer task.mutex.RUnlock()

	// ✅ 新逻辑：AI完成且所有内容都已展示才算真正完成
	// 这确保了所有生成的内容都被企业微信展示完毕
	aiFinished := !task.IsProcessing && task.Buffer.IsAIFinished()
	allDisplayed := task.Buffer.IsEmpty() // 在累积模式下，IsEmpty表示所有内容都已展示
	isFinished := aiFinished && allDisplayed

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
	// 任务缓存管理器已初始化

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
	// 使用千问模型

	qwenClient := openai.NewClient(b.config.QwenAPIKey,
		openai.WithBaseURL(b.config.QwenBaseURL),
		openai.WithModel(b.config.QwenModel),
		openai.WithLogger(logger))

	// 创建工具注册器
	toolRegistry := tools.NewRegistry()

	// === MCP 按需连接配置 - 完全复用qwen-http版本逻辑 ===
	// MCP按需连接配置
	var mcpServers []interfaces.MCPServer

	// 配置会话级MCP管理器
	// 配置会话级MCP管理器

	// 创建会话级MCP管理器（完全复用qwen-http版本实现）
	b.sessionMCP = NewSessionMCPManager(b.config.MCPServerURL)
	mcpServers = append(mcpServers, b.sessionMCP)
	// 会话级MCP管理器配置完成

	// 测试连接以验证配置正确性
	//// 测试连接和工具发现
	//tools, err := b.sessionMCP.ListTools(context.Background())
	//if err != nil {
	//	// 测试连接失败
	//} else {
	//	// 发现MCP工具
	//}

	// === 创建智能体 - 完全复用qwen-http版本逻辑 ===
	var agentErr error
	if len(mcpServers) > 0 {
		// 创建MCP智能体
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
		// 创建基础智能体
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
	// ✅ 注意：conversation ID已移至processTaskAsync中使用streamID设置
	// 这样确保每个任务有独立的对话上下文，避免memory污染

	// 1. 创建任务（模拟Python LLMDemo.invoke()）
	streamID, err := b.taskCache.Invoke(ctx, textContent)
	if err != nil {
		return wework.NewTextResponse("系统忙，请稍后再试"), err
	}

	// 2. 获取第一步答案（模拟Python LLMDemo.get_answer()）
	answer := b.taskCache.GetAnswer(streamID)

	// 3. 检查是否完成（模拟Python LLMDemo.is_task_finish()）
	finish := b.taskCache.IsTaskFinish(streamID)

	// ✅ 优化返回策略：首次必须有内容，即使AI还在处理中
	if answer == "" && !finish {
		// 如果没有内容且未完成，返回处理中提示
		answer = "正在为您思考中..."
	}

	// 4. 返回stream消息（模拟Python MakeTextStream + EncryptMessage）
	// 关键：finish=false时企业微信会发送刷新请求！
	return wework.NewStreamResponse(streamID, answer, finish), nil
}

// HandleStreamRefresh 处理流式消息刷新 - 模拟Python示例的stream消息处理
func (b *BotHandler) HandleStreamRefresh(streamID string) (*wework.WeWorkResponse, error) {
	// 1. 获取最新答案（模拟Python LLMDemo.get_answer()）
	answer := b.taskCache.GetAnswer(streamID)

	// 2. 检查是否完成（模拟Python LLMDemo.is_task_finish()）
	finish := b.taskCache.IsTaskFinish(streamID)

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
		// 使用新的完成状态检查逻辑（累积模式）
		isProcessing := task.IsProcessing
		aiFinished := task.Buffer.IsAIFinished()
		allDisplayed := task.Buffer.IsEmpty() // 累积模式：所有内容都已展示
		isActive := isProcessing || !aiFinished || !allDisplayed
		if isActive {
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
	// 创建新连接

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
	// 连接已清理
}

// ensureConnection 确保有活跃的MCP连接（使用时验证）
func (s *SessionMCPManager) ensureConnection(ctx context.Context) (interfaces.MCPServer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 检查现有连接的有效性
	if s.connection != nil && s.sessionActive {
		// 时间检查：超过2分钟自动重建
		if time.Since(s.lastActivity) > 2*time.Minute {
			// 连接超时，重建连接
			s.cleanupConnection()
		} else {
			// 健康检查：验证连接可用性
			if s.isConnectionAlive() {
				s.lastActivity = time.Now()
				// 复用现有连接
				return s.connection, nil
			} else {
				// 连接失效，重建连接
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
	// 调用工具

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

	// 工具调用完成
	return response, nil
}

// Close 实现MCPServer接口 - 手动清理会话连接
func (s *SessionMCPManager) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 手动关闭会话连接
	s.cleanupConnection()
	return nil
}
