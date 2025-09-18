package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
)

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

	// 🔧 关键修复：转换MCP响应格式
	// MCP协议返回的Content可能是JSON数组格式：[{"type":"text","text":"actual content"}]
	// 我们需要提取其中的文本内容，让agent-sdk-go能正确处理
	if response != nil && response.Content != nil {
		response.Content = s.extractTextFromMCPContent(response.Content)
	}

	// 工具调用完成
	return response, nil
}

// extractTextFromMCPContent 从MCP响应中提取文本内容
func (s *SessionMCPManager) extractTextFromMCPContent(content interface{}) interface{} {
	// 尝试将content转换为[]interface{}（JSON数组）
	if arr, ok := content.([]interface{}); ok && len(arr) > 0 {
		// 遍历数组，查找包含text字段的元素
		var textParts []string
		for _, item := range arr {
			if obj, ok := item.(map[string]interface{}); ok {
				// 检查是否有type="text"和text字段
				if typeVal, hasType := obj["type"].(string); hasType && typeVal == "text" {
					if textVal, hasText := obj["text"].(string); hasText {
						textParts = append(textParts, textVal)
					}
				}
			}
		}

		// 如果找到文本内容，返回拼接后的字符串
		if len(textParts) > 0 {
			return strings.Join(textParts, "\n")
		}
	}

	// 如果不是MCP格式，返回原始内容
	return content
}

// Close 实现MCPServer接口 - 手动清理会话连接
func (s *SessionMCPManager) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 手动关闭会话连接
	s.cleanupConnection()
	return nil
}
