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
	StreamID       string        `json:"stream_id"`
	Question       string        `json:"question"`
	ConversationID string        `json:"conversation_id"` // 会话ID（用于记忆连续性）
	CreatedTime    time.Time     `json:"created_time"`
	Buffer         *StreamBuffer `json:"-"`             // 流式缓冲区（替换累积内容）
	IsProcessing   bool          `json:"is_processing"` // AI是否正在处理
	LastUpdate     time.Time     `json:"last_update"`
	mutex          sync.RWMutex  `json:"-"`

	// ❌ 已移除的累积模式字段：
	// CurrentStep  int             - 不再需要固定步数
	// MaxSteps     int             - 不再需要最大步数限制
	// Content      strings.Builder - 不再累积内容，改为缓冲区
	// IsFinished   bool            - 通过Buffer.IsAIFinished()获取
}

// TaskCacheManager 任务缓存管理器 - 模拟Python LLMDemo
type TaskCacheManager struct {
	tasks            map[string]*TaskInfo
	mutex            sync.RWMutex
	convAgentManager *ConversationAgentManager // 会话级Agent管理器
}

// NewTaskCacheManager 创建任务缓存管理器
func NewTaskCacheManager(convAgentManager *ConversationAgentManager) *TaskCacheManager {
	return &TaskCacheManager{
		tasks:            make(map[string]*TaskInfo),
		convAgentManager: convAgentManager,
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
func (tcm *TaskCacheManager) Invoke(ctx context.Context, question string, conversationID string) (string, error) {
	streamID, err := generateTaskID()
	if err != nil {
		return "", fmt.Errorf("生成任务ID失败: %w", err)
	}

	// 创建任务信息 - 基于StreamBuffer的真正流式架构
	task := &TaskInfo{
		StreamID:       streamID,
		Question:       question,
		ConversationID: conversationID, // ✅ 保存会话ID
		CreatedTime:    time.Now(),
		Buffer:         NewStreamBuffer(), // ✅ 创建流式缓冲区
		IsProcessing:   false,
		LastUpdate:     time.Now(),
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

	// ✅ 关键修改：使用conversationID作为会话标识，实现连续对话记忆
	// 同一用户/群组的对话会共享记忆上下文
	ctx = context.WithValue(ctx, memory.ConversationIDKey, task.ConversationID)

	// 获取或创建会话Agent
	convAgent, err := tcm.convAgentManager.GetOrCreateAgent(task.ConversationID)
	if err != nil {
		fmt.Printf("❌ 获取会话Agent失败: %v\n", err)
		task.Buffer.Push(fmt.Sprintf("系统错误: %v", err))
		task.Buffer.SetAIFinished()
		task.mutex.Lock()
		task.IsProcessing = false
		task.LastUpdate = time.Now()
		task.mutex.Unlock()
		return
	}

	// 记录调用分析
	callCount := 0
	chunkCount := 0

	// 调用Agent进行流式处理
	events, err := convAgent.RunStream(ctx, task.Question)
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
	// AI生成内容实时推送到StreamBuffer，供企业微信消趟

	// 跟踪状态，用于识别final call
	var hasToolCall bool = false
	var isAfterToolCall bool = false

	for event := range events {
		// 检查是否有工具调用
		if event.Type == interfaces.AgentEventToolCall || event.Type == interfaces.AgentEventToolResult {
			hasToolCall = true
			isAfterToolCall = true
		}

		// 检查metadata中的final_call标记
		var isFinalCall bool
		if event.Metadata != nil {
			if fc, ok := event.Metadata["final_call"].(bool); ok && fc {
				isFinalCall = true
			}
		}

		if event.Content != "" {
			chunkCount++

			// ✨ Final Call内容过滤策略
			// 1. 如果检测到final_call标记，过滤
			if isFinalCall {
				continue
			}

			// 2. 如果已经有工具调用，且内容包含特定模式，过滤
			if hasToolCall && isAfterToolCall {
				// 检测是否是final call的特征内容
				if strings.Contains(event.Content, "示例时间") ||
					strings.Contains(event.Content, "请让我再次查询") ||
					strings.Contains(event.Content, "具体时间请以实际查询结果为准") ||
					(strings.Contains(event.Content, "时间是") && strings.Contains(event.Content, "2023-")) {
					fmt.Printf("🛑 过滤疑似final call内容: %.50s...\n", event.Content)
					continue
				}

				// 3. 如果是"当前的"开头的重复时间描述
				if strings.HasPrefix(event.Content, "当前的") && strings.Contains(event.Content, "时间是") {
					fmt.Printf("🛑 过滤重复时间描述: %.50s...\n", event.Content)
					continue
				}
			}

			// 检测是否是新的调用开始（通过内容模式识别）
			if strings.Contains(event.Content, "企业微信") && strings.Contains(event.Content, "智能助手") {
				callCount++
				fmt.Printf("🔍 检测到第 %d 次模型调用开始\n", callCount)
			}

			// 通过过滤，推送到缓冲区（生产者模式）
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

// ConversationAgent 会话级Agent
type ConversationAgent struct {
	agentInstance *agent.Agent
	lastActivity  time.Time
	mutex         sync.RWMutex
}

// ConversationAgentManager 会话级Agent管理器
type ConversationAgentManager struct {
	agents     map[string]*ConversationAgent // conversationID -> agent
	config     *config.WeWorkConfig
	sessionMCP *SessionMCPManager
	mutex      sync.RWMutex
}

// BotHandler 机器人处理器
type BotHandler struct {
	config           *config.WeWorkConfig
	convAgentManager *ConversationAgentManager // 会话级Agent管理器
	taskCache        *TaskCacheManager
	sessionMCP       *SessionMCPManager
}

// NewConversationAgentManager 创建会话级Agent管理器
func NewConversationAgentManager(config *config.WeWorkConfig, sessionMCP *SessionMCPManager) *ConversationAgentManager {
	return &ConversationAgentManager{
		agents:     make(map[string]*ConversationAgent),
		config:     config,
		sessionMCP: sessionMCP,
	}
}

// GetOrCreateAgent 获取或创建会话Agent
func (cam *ConversationAgentManager) GetOrCreateAgent(conversationID string) (*agent.Agent, error) {
	cam.mutex.Lock()
	defer cam.mutex.Unlock()

	// 检查是否已存在
	if convAgent, exists := cam.agents[conversationID]; exists {
		convAgent.mutex.Lock()
		convAgent.lastActivity = time.Now()
		convAgent.mutex.Unlock()
		fmt.Printf("♻️ 复用会话Agent: %s\n", conversationID)
		return convAgent.agentInstance, nil
	}

	// 创建新的Agent
	fmt.Printf("🆕 创建新会话Agent: %s\n", conversationID)
	newAgent, err := cam.createNewAgent()
	if err != nil {
		return nil, err
	}

	// 保存到缓存
	cam.agents[conversationID] = &ConversationAgent{
		agentInstance: newAgent,
		lastActivity:  time.Now(),
	}

	return newAgent, nil
}

// createNewAgent 创建新的Agent实例
func (cam *ConversationAgentManager) createNewAgent() (*agent.Agent, error) {
	logger := logging.New()

	// 创建千问客户端
	qwenClient := openai.NewClient(cam.config.QwenAPIKey,
		openai.WithBaseURL(cam.config.QwenBaseURL),
		openai.WithModel(cam.config.QwenModel),
		openai.WithLogger(logger))

	// 创建工具注册器
	toolRegistry := tools.NewRegistry()

	// MCP服务器配置
	var mcpServers []interfaces.MCPServer
	if cam.sessionMCP != nil {
		mcpServers = append(mcpServers, cam.sessionMCP)
	}

	// 创建Agent
	var agentInstance *agent.Agent
	var err error

	if len(mcpServers) > 0 {
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(qwenClient),
			agent.WithMemory(memory.NewConversationBuffer(memory.WithMaxSize(3))),
			agent.WithTools(toolRegistry.List()...),
			agent.WithMCPServers(mcpServers),
			agent.WithRequirePlanApproval(false),
			agent.WithSystemPrompt("你是一个企业微信智能助手，使用中文回答问题。你可以使用各种MCP工具来帮助回答问题，请根据用户问题智能选择和调用合适的工具。当你需要获取实时信息（如时间）或执行特定任务时，请主动使用相关工具。请保持回答简洁明了，适合企业微信聊天场景。"),
			agent.WithMaxIterations(2),
			agent.WithName("AIBodyWeWorkAssistant"),
		)
	} else {
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(qwenClient),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithTools(toolRegistry.List()...),
			agent.WithSystemPrompt("你是一个企业微信智能助手，使用中文回答问题。请提供详细和有帮助的回答，保持简洁明了。"),
			agent.WithMaxIterations(2),
			agent.WithName("AIBodyWeWorkAssistant"),
		)
	}

	return agentInstance, err
}

// NewBotHandler 创建机器人处理器
func NewBotHandler(cfg *config.WeWorkConfig) (*BotHandler, error) {
	handler := &BotHandler{
		config: cfg,
	}

	// 创建SessionMCP管理器
	handler.sessionMCP = NewSessionMCPManager(cfg.MCPServerURL)

	// 创建会话级Agent管理器
	handler.convAgentManager = NewConversationAgentManager(cfg, handler.sessionMCP)

	// 初始化任务缓存管理器（注意：现在需要传入会话管理器）
	handler.taskCache = NewTaskCacheManager(handler.convAgentManager)

	return handler, nil
}

// Close 关闭机器人处理器
func (b *BotHandler) Close() {
	if b.taskCache != nil {
		b.taskCache.Close()
	}
	if b.convAgentManager != nil {
		b.convAgentManager.Close()
	}
	if b.sessionMCP != nil {
		b.sessionMCP.Close()
	}
}

// Close 关闭会话Agent管理器
func (cam *ConversationAgentManager) Close() {
	cam.mutex.Lock()
	defer cam.mutex.Unlock()

	for id := range cam.agents {
		delete(cam.agents, id)
	}
	fmt.Println("✅ 会话Agent管理器已关闭")
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
	// 使用稳定的会话ID确保对话连续性
	conversationID := msg.GetConversationKey()
	streamID, err := b.taskCache.Invoke(ctx, textContent, conversationID)
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
