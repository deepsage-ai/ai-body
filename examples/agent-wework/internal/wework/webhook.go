package wework

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// MessageHandler 消息处理器接口
type MessageHandler interface {
	HandleMessage(msg *IncomingMessage) (*WeWorkResponse, error)
	HandleStreamRefresh(streamID string) (*WeWorkResponse, error)
}

// WebhookHandler Webhook处理器
type WebhookHandler struct {
	crypto     *Crypto
	handler    MessageHandler
	msgCache   map[string]time.Time // 消息去重缓存
	cacheMutex sync.RWMutex         // 缓存锁
	cacheSize  int                  // 缓存大小限制
}

// NewWebhookHandler 创建Webhook处理器
func NewWebhookHandler(token, aesKey string, handler MessageHandler) (*WebhookHandler, error) {
	crypto, err := NewCrypto(token, aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create crypto: %w", err)
	}

	return &WebhookHandler{
		crypto:    crypto,
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
	// 获取查询参数并进行URL解码
	signature := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	echostr := c.Query("echostr")

	// URL解码处理
	if decodedEchostr, err := url.QueryUnescape(echostr); err == nil {
		echostr = decodedEchostr
	}

	if signature == "" || timestamp == "" || nonce == "" || echostr == "" {
		fmt.Printf("❌ URL验证失败: 缺少必要参数\n")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	fmt.Printf("🔍 URL验证请求: signature=%s, timestamp=%s, nonce=%s\n",
		signature[:8]+"...", timestamp, nonce[:8]+"...")

	// 验证并解密echostr
	decrypted, err := w.crypto.VerifyURL(signature, timestamp, nonce, echostr)
	if err != nil {
		fmt.Printf("❌ URL验证失败: %v\n", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Verification failed"})
		return
	}

	fmt.Printf("✅ URL验证成功\n")
	c.String(http.StatusOK, decrypted)
}

// handleMessage 处理消息（POST请求）
func (w *WebhookHandler) handleMessage(c *gin.Context) {
	signature := c.Query("msg_signature")
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")

	if signature == "" || timestamp == "" || nonce == "" {
		fmt.Printf("❌ 消息处理失败: 缺少必要参数\n")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	// 读取请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fmt.Printf("❌ 读取请求体失败: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// 解析加密的XML
	encReq, err := ParseEncryptedXML(body)
	if err != nil {
		fmt.Printf("❌ 解析XML失败: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid XML format"})
		return
	}

	// 解密消息
	decryptedData, err := w.crypto.DecryptMessage(signature, timestamp, nonce, encReq.Encrypt)
	if err != nil {
		fmt.Printf("❌ 消息解密失败: %v\n", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Decryption failed"})
		return
	}

	// 解析消息
	msg, err := ParseMessage(decryptedData)
	if err != nil {
		fmt.Printf("❌ 消息解析失败: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message format"})
		return
	}

	// 消息去重检查
	if w.isDuplicateMessage(msg.MsgID) {
		fmt.Printf("🔄 重复消息忽略: %s\n", msg.MsgID)
		c.String(http.StatusOK, "success") // 企业微信期望返回success
		return
	}

	// 记录消息用于去重
	w.recordMessage(msg.MsgID)

	fmt.Printf("📨 收到消息: type=%s, from=%s, msgid=%s\n",
		msg.MsgType, msg.From.UserID, msg.MsgID[:8]+"...")

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
		fmt.Printf("❌ 消息处理失败: %v\n", err)
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
		fmt.Printf("❌ 响应JSON序列化失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Response serialization failed"})
		return
	}

	// 加密响应
	encryptedResp, err := w.crypto.EncryptMessage(responseData, timestamp, nonce)
	if err != nil {
		fmt.Printf("❌ 响应加密失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Response encryption failed"})
		return
	}

	fmt.Printf("✅ 发送加密响应: type=%s\n", response.MsgType)

	c.Header("Content-Type", "application/xml")
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
	activeStreams := 0
	if streamManager, ok := w.handler.(interface{ GetActiveStreamCount() int }); ok {
		activeStreams = streamManager.GetActiveStreamCount()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "healthy",
		"service":        "AI-Body 企业微信智能机器人（流式版本）",
		"version":        "1.0.0",
		"timestamp":      time.Now().Unix(),
		"cache_size":     len(w.msgCache),
		"active_streams": activeStreams,
		"features":       []string{"encryption", "deduplication", "mcp_tools", "streaming"},
	})
}
