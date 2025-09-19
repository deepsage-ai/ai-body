package bot

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"

	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/config"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/llm"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/mcp"
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

// GetAccumulated 获取累积内容（优化版本：一次性返回所有已生成内容）
func (sb *StreamBuffer) GetAccumulated() (string, bool) {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	// 关键修改：直接更新lastIndex到当前chunks长度，而不是每次只加1
	currentChunkCount := len(sb.chunks)
	if sb.lastIndex < currentChunkCount {
		// 一次性更新到当前所有chunks
		sb.lastIndex = currentChunkCount
		sb.lastUpdate = time.Now()
	}

	// 构建累积内容（返回所有已生成的内容）
	var accumulated strings.Builder
	for i := 0; i < sb.lastIndex; i++ {
		accumulated.WriteString(sb.chunks[i])
	}

	// 检查AI是否完成
	isFinished := sb.aiFinished && sb.lastIndex >= len(sb.chunks)

	// 合并多个think标签（企业微信只能识别一个）
	content := mergeThinkTags(accumulated.String())
	return content, isFinished
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
		// 获取会话Agent失败
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

	var hasNormalContent bool = false // 是否有正常内容生成

	for event := range events {
		// 检查是否有工具调用
		if event.Type == interfaces.AgentEventToolCall {
			hasToolCall = true

			// 不再推送工具调用提示，让用户专注于最终结果
		} else if event.Type == interfaces.AgentEventToolResult {
			// 工具结果不直接显示，等待AI整理后的内容
			hasToolCall = true
			// 记录工具结果用于调试
			if event.Metadata != nil {
				if result, ok := event.Metadata["result"].(string); ok {
					fmt.Printf("🔧 工具结果 - %v: %s\n", event.ToolCall, result)
				}
			}
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

			// 在final call时记录详细信息
			if isFinalCall && hasToolCall {
			}

			// ✨ Final Call内容过滤策略
			// 如果已经有正常内容生成，final call是多余的，应该过滤
			// 因为agent-sdk-go在没有新工具调用时会break并触发final call
			// 但此时AI可能已经在生成正确的最终回复
			if isFinalCall && hasNormalContent {
				// 已有正常内容，过滤final call
				continue
			}

			// 标记有正常内容生成
			hasNormalContent = true

			// 检测是否是新的调用开始（通过内容模式识别）
			if strings.Contains(event.Content, "企业微信") && strings.Contains(event.Content, "智能助手") {
				callCount++
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
	config     *config.Config
	mcpServers []interfaces.MCPServer
	mutex      sync.RWMutex
}

// BotHandler 机器人处理器
type BotHandler struct {
	config           *config.Config
	convAgentManager *ConversationAgentManager // 会话级Agent管理器
	taskCache        *TaskCacheManager
	mcpServers       []interfaces.MCPServer
	logger           *ChatLogger // 聊天日志记录器
}

// NewConversationAgentManager 创建会话级Agent管理器
func NewConversationAgentManager(config *config.Config, mcpServers []interfaces.MCPServer) *ConversationAgentManager {
	return &ConversationAgentManager{
		agents:     make(map[string]*ConversationAgent),
		config:     config,
		mcpServers: mcpServers,
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
		// 复用会话Agent
		return convAgent.agentInstance, nil
	}

	// 创建新的Agent
	// 创建新会话Agent
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

	// 使用LLM工厂创建LLM客户端
	llmClient, err := llm.CreateLLMFromConfig(cam.config, logger)
	if err != nil {
		return nil, fmt.Errorf("创建LLM客户端失败: %w", err)
	}

	// 创建工具注册器
	toolRegistry := tools.NewRegistry()

	// 创建Agent
	var agentInstance *agent.Agent

	if len(cam.mcpServers) > 0 {
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(llmClient),
			agent.WithMemory(memory.NewConversationBuffer(memory.WithMaxSize(3))),
			agent.WithTools(toolRegistry.List()...),
			agent.WithMCPServers(cam.mcpServers),
			agent.WithRequirePlanApproval(false),
			agent.WithSystemPrompt(cam.config.LLM.SystemPrompt),
			agent.WithMaxIterations(5), // 增加迭代次数，避免过早触发final call
			agent.WithName("AIBodyWeWorkAssistant"),
		)
	} else {
		agentInstance, err = agent.NewAgent(
			agent.WithLLM(llmClient),
			agent.WithMemory(memory.NewConversationBuffer()),
			agent.WithTools(toolRegistry.List()...),
			agent.WithSystemPrompt(cam.config.LLM.SystemPrompt),
			agent.WithMaxIterations(5), // 增加迭代次数，避免过早触发final call
			agent.WithName("AIBodyWeWorkAssistant"),
		)
	}

	return agentInstance, err
}

// NewBotHandler 创建机器人处理器
func NewBotHandler(cfg *config.Config) (*BotHandler, error) {
	// 创建MCP服务器
	mcpServers, err := mcp.CreateMCPServersFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建MCP服务器失败: %w", err)
	}

	handler := &BotHandler{
		config:     cfg,
		mcpServers: mcpServers,
	}

	// 创建会话级Agent管理器
	handler.convAgentManager = NewConversationAgentManager(cfg, mcpServers)

	// 初始化任务缓存管理器
	handler.taskCache = NewTaskCacheManager(handler.convAgentManager)

	// 初始化日志记录器（如果启用）
	if cfg.Logging.Enabled {
		logger, err := NewChatLogger(cfg.Logging.LogDir)
		if err != nil {
			// 日志初始化失败不影响主程序运行，只打印警告
		} else {
			handler.logger = logger
		}
	}

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
	// 关闭所有MCP服务器
	for _, server := range b.mcpServers {
		if closer, ok := server.(interface{ Close() error }); ok {
			closer.Close()
		}
	}
	// 关闭日志记录器
	if b.logger != nil {
		if err := b.logger.Close(); err != nil {
		}
	}
}

// Close 关闭会话Agent管理器
func (cam *ConversationAgentManager) Close() {
	cam.mutex.Lock()
	defer cam.mutex.Unlock()

	for id := range cam.agents {
		delete(cam.agents, id)
	}
	// 会话Agent管理器已关闭
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

	// 统一为所有消息添加用户信息
	messageWithUserInfo := fmt.Sprintf("[用户 %s]: %s", msg.From.UserID, textContent)

	// 创建上下文
	ctx := context.Background()
	ctx = multitenancy.WithOrgID(ctx, "wework-org")
	// ✅ 注意：conversation ID已移至processTaskAsync中使用streamID设置
	// 这样确保每个任务有独立的对话上下文，避免memory污染

	// 1. 创建任务（模拟Python LLMDemo.invoke()）
	// 使用稳定的会话ID确保对话连续性
	conversationID := msg.GetConversationKey()

	// 记录用户消息到日志文件
	if b.logger != nil {
		if err := b.logger.LogMessage(conversationID, msg.From.UserID, textContent); err != nil {
			// 日志记录失败不影响主流程
		}
	}

	streamID, err := b.taskCache.Invoke(ctx, messageWithUserInfo, conversationID)
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

	// 记录初始返回内容

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

	// 记录实际返回的文本内容

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

// mergeThinkTags 合并多个think标签为一个（企业微信只识别第一个）
func mergeThinkTags(content string) string {
	// 如果内容为空或不包含think标签，直接返回
	if content == "" || !strings.Contains(content, "<think>") {
		return content
	}

	// 使用正则表达式匹配所有的think标签及其内容
	thinkRegex := regexp.MustCompile(`(?s)<think>(.*?)</think>`)
	matches := thinkRegex.FindAllStringSubmatch(content, -1)

	// 如果没有匹配或只有一个think标签，直接返回
	if len(matches) <= 1 {
		return content
	}

	// 收集所有think内容
	var thinkContents []string
	for _, match := range matches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			thinkContents = append(thinkContents, strings.TrimSpace(match[1]))
		}
	}

	// 移除所有think标签
	cleanContent := thinkRegex.ReplaceAllString(content, "")

	// 如果没有收集到think内容，返回清理后的内容
	if len(thinkContents) == 0 {
		return cleanContent
	}

	// 合并所有think内容，用换行分隔
	mergedThink := "<think>\n" + strings.Join(thinkContents, "\n\n") + "\n</think>\n"

	// 将合并后的think标签放在内容开头
	return mergedThink + strings.TrimSpace(cleanContent)
}
