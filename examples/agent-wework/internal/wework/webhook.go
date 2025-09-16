package wework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sbzhu/weworkapi_golang/wxbizmsgcrypt"
)

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// EncryptedRequest 企业微信加密请求结构（JSON格式）
type EncryptedRequest struct {
	Encrypt string `json:"encrypt"`
}

// MessageHandler 消息处理器接口
type MessageHandler interface {
	HandleMessage(msg *IncomingMessage) (*WeWorkResponse, error)
	HandleStreamRefresh(streamID string) (*WeWorkResponse, error)
}

// WebhookHandler Webhook处理器
type WebhookHandler struct {
	wxcpt      *wxbizmsgcrypt.WXBizMsgCrypt // 官方加解密库
	botID      string                       // 机器人ID
	handler    MessageHandler
	msgCache   map[string]time.Time // 消息去重缓存
	cacheMutex sync.RWMutex         // 缓存锁
	cacheSize  int                  // 缓存大小限制
}

// NewWebhookHandler 创建Webhook处理器
func NewWebhookHandler(token, aesKey, botID string, handler MessageHandler) (*WebhookHandler, error) {
	// 使用官方SDK，智能机器人场景receiverId使用空字符串（参照官方文档）
	wxcpt := wxbizmsgcrypt.NewWXBizMsgCrypt(token, aesKey, "", wxbizmsgcrypt.XmlType)

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
		fmt.Printf("❌ URL验证失败: 缺少必要参数\n")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters"})
		return
	}

	fmt.Printf("🔍 URL验证请求: signature=%s..., timestamp=%s, nonce=%s...\n",
		signature[:8], timestamp, nonce[:8])

	// 使用官方SDK进行验证（参照官方示例）
	echoStr, cryptErr := w.wxcpt.VerifyURL(signature, timestamp, nonce, echostr)
	if nil != cryptErr {
		fmt.Printf("❌ URL验证失败: %v\n", cryptErr)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Verification failed"})
		return
	}

	fmt.Printf("✅ URL验证成功\n")
	c.String(http.StatusOK, string(echoStr))
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

	// 解析JSON格式的加密请求（智能机器人使用JSON格式）
	var encReq EncryptedRequest
	if err := json.Unmarshal(body, &encReq); err != nil {
		fmt.Printf("❌ JSON解析失败: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
		return
	}

	fmt.Printf("🔍 接收到加密消息，encrypt字段长度: %d\n", len(encReq.Encrypt))

	// 构造XML格式传给官方SDK（ToUserName使用BotID）
	xmlData := fmt.Sprintf(`<xml><ToUserName><![CDATA[%s]]></ToUserName><Encrypt><![CDATA[%s]]></Encrypt></xml>`, w.botID, encReq.Encrypt)

	// 使用官方SDK解密消息
	decryptedData, cryptErr := w.wxcpt.DecryptMsg(signature, timestamp, nonce, []byte(xmlData))
	if nil != cryptErr {
		fmt.Printf("❌ 消息解密失败: %v\n", cryptErr)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Decryption failed"})
		return
	}

	fmt.Printf("✅ 消息解密成功，解密后内容: %s\n", string(decryptedData))

	// 解析JSON格式的解密消息
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

	// 使用官方SDK加密响应（参照官方示例）
	encryptedResp, cryptErr := w.wxcpt.EncryptMsg(string(responseData), timestamp, nonce)
	if nil != cryptErr {
		fmt.Printf("❌ 响应加密失败: %v\n", cryptErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Response encryption failed"})
		return
	}

	fmt.Printf("✅ 发送加密响应: type=%s\n", response.MsgType)

	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, string(encryptedResp))
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
