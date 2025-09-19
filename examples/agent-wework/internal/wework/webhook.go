package wework

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// min 返回两个整数中的较小值

// MessageHandler 消息处理器接口
type MessageHandler interface {
	HandleMessage(msg *IncomingMessage) (*WeWorkResponse, error)
	HandleStreamRefresh(streamID string) (*WeWorkResponse, error)
}

// WebhookHandler Webhook处理器
type WebhookHandler struct {
	wxcpt      *WXBizJsonMsgCrypt // 我们自己的加解密库
	botID      string             // 机器人ID
	handler    MessageHandler
	msgCache   map[string]time.Time // 消息去重缓存
	cacheMutex sync.RWMutex         // 缓存锁
	cacheSize  int                  // 缓存大小限制
}

// NewWebhookHandler 创建Webhook处理器
func NewWebhookHandler(token, aesKey, botID string, handler MessageHandler) (*WebhookHandler, error) {
	// 使用我们自己实现的加解密库，严格按照Python逻辑
	wxcpt, err := NewWXBizJsonMsgCrypt(token, aesKey, "") // 智能机器人场景receiverId使用空字符串
	if err != nil {
		return nil, fmt.Errorf("创建加解密实例失败: %w", err)
	}

	return &WebhookHandler{
		wxcpt:     wxcpt,
		botID:     botID,
		handler:   handler,
		msgCache:  make(map[string]time.Time),
		cacheSize: 1000, // 缓存1000条消息用于去重
	}, nil
}

// HandleWebhook 处理Webhook请求
func (w *WebhookHandler) HandleWebhook(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodGet:
		w.handleVerification(c)
	case http.MethodPost:
		w.handleMessage(c)
	default:
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method not allowed"})
	}
}

// handleVerification 处理URL验证（GET请求）
func (w *WebhookHandler) handleVerification(c *gin.Context) {
	// 获取查询参数（Gin已自动URL解码）
	signature := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	echostr := c.Query("echostr")

	if signature == "" || timestamp == "" || nonce == "" || echostr == "" {
		// URL验证失败: 缺少必要参数
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	// URL验证请求

	// 使用我们自己的加解密库进行验证（严格按照Python逻辑）
	ret, echoStr, err := w.wxcpt.VerifyURL(signature, timestamp, nonce, echostr)
	if ret != WXBizMsgCrypt_OK || err != nil {
		// URL验证失败
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Verification failed"})
		return
	}

	// URL验证成功
	c.String(http.StatusOK, echoStr)
}

// handleMessage 处理消息（POST请求）
func (w *WebhookHandler) handleMessage(c *gin.Context) {
	signature := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")

	if signature == "" || timestamp == "" || nonce == "" {
		// 消息处理失败: 缺少必要参数
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		// 读取请求体失败
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// 使用我们自己的加解密库解密消息（严格按照Python逻辑）
	// 直接传递原始JSON格式给解密函数
	ret, decryptedContent, err := w.wxcpt.DecryptMsg(string(body), signature, timestamp, nonce)
	if ret != WXBizMsgCrypt_OK || err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Decryption failed"})
		return
	}

	decryptedData := []byte(decryptedContent)

	// 解析JSON格式的解密消息
	msg, err := ParseMessage(decryptedData)
	if err != nil {
		// 消息解析失败
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message format"})
		return
	}

	// 消息去重检查
	if w.isDuplicateMessage(msg.MsgID) {
		c.String(http.StatusOK, "success") // 企业微信期望返回success
		return
	}

	// 记录消息用于去重
	w.recordMessage(msg.MsgID)

	// 处理消息
	var response *WeWorkResponse
	if msg.MsgType == MsgTypeStream {
		// 流式消息刷新
		if msg.Stream != nil {
			response, err = w.handler.HandleStreamRefresh(msg.Stream.ID)
		} else {
			err = fmt.Errorf("stream content is nil")
		}
	} else {
		// 普通消息
		response, err = w.handler.HandleMessage(msg)
	}

	if err != nil {
		// 消息处理失败
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Message processing failed"})
		return
	}

	// 如果有回复内容，则加密并返回
	if response != nil {
		w.sendEncryptedResponse(c, response, timestamp, nonce)
	} else {
		// 无回复内容，返回success
		c.String(http.StatusOK, "success")
	}
}

// sendEncryptedResponse 发送加密响应
func (w *WebhookHandler) sendEncryptedResponse(c *gin.Context, response *WeWorkResponse, timestamp, nonce string) {
	// 转换为JSON
	responseData, err := response.ToJSON()
	if err != nil {
		// 响应JSON序列化失败
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Response serialization failed"})
		return
	}

	// 使用我们自己的加解密库加密响应（严格按照Python逻辑）
	// Python: EncryptMsg(sReplyMsg, sNonce, timestamp)
	ret, encryptedResp, err := w.wxcpt.EncryptMsg(string(responseData), nonce, &timestamp)
	if ret != WXBizMsgCrypt_OK || err != nil {
		// 响应加密失败
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Response encryption failed"})
		return
	}

	// 发送加密响应

	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, encryptedResp)
}

// isDuplicateMessage 检查是否为重复消息
func (w *WebhookHandler) isDuplicateMessage(msgID string) bool {
	w.cacheMutex.RLock()
	_, exists := w.msgCache[msgID]
	w.cacheMutex.RUnlock()
	return exists
}

// recordMessage 记录消息用于去重
func (w *WebhookHandler) recordMessage(msgID string) {
	w.cacheMutex.Lock()
	defer w.cacheMutex.Unlock()

	// 清理过期消息（保留最近1小时的消息）
	now := time.Now()
	if len(w.msgCache) > w.cacheSize {
		cutoff := now.Add(-time.Hour)
		for id, timestamp := range w.msgCache {
			if timestamp.Before(cutoff) {
				delete(w.msgCache, id)
			}
		}
	}

	// 记录新消息
	w.msgCache[msgID] = now
}

// HealthCheck 健康检查处理器
func (w *WebhookHandler) HealthCheck(c *gin.Context) {
	activeTasks := 0
	if taskManager, ok := w.handler.(interface{ GetActiveStreamCount() int }); ok {
		activeTasks = taskManager.GetActiveStreamCount()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "healthy",
		"service":      "AI-Body 企业微信智能机器人（Python流式模式）",
		"version":      "1.0.0",
		"timestamp":    time.Now().Unix(),
		"cache_size":   len(w.msgCache),
		"active_tasks": activeTasks,
		"features":     []string{"encryption", "deduplication", "mcp_tools", "task_cache", "python_stream_mode"},
	})
}
