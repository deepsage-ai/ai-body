package wework

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MessageType 消息类型常量
const (
	MsgTypeText   = "text"   // 文本消息
	MsgTypeImage  = "image"  // 图片消息
	MsgTypeMixed  = "mixed"  // 图文混排
	MsgTypeStream = "stream" // 流式消息刷新
)

// ChatType 会话类型常量
const (
	ChatTypeSingle = "single" // 单聊
	ChatTypeGroup  = "group"  // 群聊
)

// BaseMessage 基础消息结构
type BaseMessage struct {
	MsgID    string `json:"msgid"`    // 消息唯一标识
	AIBotID  string `json:"aibotid"`  // 智能机器人ID
	ChatID   string `json:"chatid"`   // 会话ID（仅群聊返回）
	ChatType string `json:"chattype"` // 会话类型：single|group
	From     From   `json:"from"`     // 发送者信息
	MsgType  string `json:"msgtype"`  // 消息类型
}

// From 发送者信息
type From struct {
	UserID string `json:"userid"` // 用户ID
}

// TextContent 文本内容
type TextContent struct {
	Content string `json:"content"` // 文本内容
}

// ImageContent 图片内容
type ImageContent struct {
	URL string `json:"url"` // 图片下载URL（5分钟有效，加密）
}

// MixedContent 图文混排内容
type MixedContent struct {
	MsgItem []MixedItem `json:"msg_item"` // 图文混排项目列表
}

// MixedItem 图文混排单项
type MixedItem struct {
	MsgType string        `json:"msgtype"`         // 项目类型：text|image
	Text    *TextContent  `json:"text,omitempty"`  // 文本内容（当msgtype为text时）
	Image   *ImageContent `json:"image,omitempty"` // 图片内容（当msgtype为image时）
}

// StreamContent 流式消息内容
type StreamContent struct {
	ID string `json:"id"` // 流式消息ID
}

// IncomingMessage 通用接收消息结构
type IncomingMessage struct {
	BaseMessage
	// 各种消息类型的内容（根据MsgType判断使用哪个）
	Text   *TextContent   `json:"text,omitempty"`
	Image  *ImageContent  `json:"image,omitempty"`
	Mixed  *MixedContent  `json:"mixed,omitempty"`
	Stream *StreamContent `json:"stream,omitempty"`
}

// ParseMessage 解析企业微信消息
func ParseMessage(data []byte) (*IncomingMessage, error) {
	var msg IncomingMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	// 验证必要字段
	if msg.MsgID == "" {
		return nil, fmt.Errorf("msgid is required")
	}
	if msg.MsgType == "" {
		return nil, fmt.Errorf("msgtype is required")
	}
	if msg.From.UserID == "" {
		return nil, fmt.Errorf("from.userid is required")
	}

	return &msg, nil
}

// GetTextContent 获取消息的文本内容
func (m *IncomingMessage) GetTextContent() string {
	switch m.MsgType {
	case MsgTypeText:
		if m.Text != nil {
			return m.Text.Content
		}
	case MsgTypeMixed:
		if m.Mixed != nil {
			var textParts []string
			for _, item := range m.Mixed.MsgItem {
				if item.MsgType == MsgTypeText && item.Text != nil {
					textParts = append(textParts, item.Text.Content)
				}
			}
			if len(textParts) > 0 {
				return strings.Join(textParts, " ")
			}
		}
	}
	return ""
}

// GetImageURLs 获取消息中的图片URL列表
func (m *IncomingMessage) GetImageURLs() []string {
	var urls []string

	switch m.MsgType {
	case MsgTypeImage:
		if m.Image != nil && m.Image.URL != "" {
			urls = append(urls, m.Image.URL)
		}
	case MsgTypeMixed:
		if m.Mixed != nil {
			for _, item := range m.Mixed.MsgItem {
				if item.MsgType == MsgTypeImage && item.Image != nil && item.Image.URL != "" {
					urls = append(urls, item.Image.URL)
				}
			}
		}
	}

	return urls
}

// IsGroupChat 判断是否为群聊
func (m *IncomingMessage) IsGroupChat() bool {
	return m.ChatType == ChatTypeGroup
}

// NeedsReply 判断消息是否需要回复
func (m *IncomingMessage) NeedsReply() bool {
	// 所有消息类型都需要回复
	return m.MsgType == MsgTypeText || m.MsgType == MsgTypeImage ||
		m.MsgType == MsgTypeMixed || m.MsgType == MsgTypeStream
}

// GetConversationKey 获取会话唯一标识
func (m *IncomingMessage) GetConversationKey() string {
	if m.IsGroupChat() {
		return fmt.Sprintf("group_%s", m.ChatID)
	}
	return fmt.Sprintf("single_%s", m.From.UserID)
}

// === 企业微信回复消息结构 ===

// WeWorkResponse 企业微信回复消息基础结构
type WeWorkResponse struct {
	MsgType      string               `json:"msgtype"`                 // 消息类型
	Text         *WeWorkTextContent   `json:"text,omitempty"`          // 文本消息
	Stream       *WeWorkStreamContent `json:"stream,omitempty"`        // 流式消息
	TemplateCard *WeWorkTemplateCard  `json:"template_card,omitempty"` // 模板卡片
}

// WeWorkTextContent 企业微信文本回复内容
type WeWorkTextContent struct {
	Content string `json:"content"` // 文本内容
}

// WeWorkStreamContent 企业微信流式回复内容
type WeWorkStreamContent struct {
	ID      string                `json:"id"`                 // 流式消息ID
	Finish  bool                  `json:"finish"`             // 是否结束
	Content string                `json:"content"`            // 流式消息内容
	MsgItem []WeWorkStreamMsgItem `json:"msg_item,omitempty"` // 流式消息图文混排列表
}

// WeWorkStreamMsgItem 企业微信流式消息图文混排项
type WeWorkStreamMsgItem struct {
	MsgType string             `json:"msgtype"`         // 消息类型：image
	Image   *WeWorkStreamImage `json:"image,omitempty"` // 图片内容
}

// WeWorkStreamImage 企业微信流式消息图片
type WeWorkStreamImage struct {
	Base64 string `json:"base64"` // 图片内容的base64编码
	MD5    string `json:"md5"`    // 图片内容的md5值
}

// WeWorkTemplateCard 企业微信模板卡片（预留扩展）
type WeWorkTemplateCard struct {
	// 模板卡片结构，后续扩展
}

// NewTextResponse 创建文本回复
func NewTextResponse(content string) *WeWorkResponse {
	return &WeWorkResponse{
		MsgType: MsgTypeText,
		Text: &WeWorkTextContent{
			Content: content,
		},
	}
}

// NewStreamResponse 创建流式回复
func NewStreamResponse(streamID, content string, finish bool) *WeWorkResponse {
	return &WeWorkResponse{
		MsgType: MsgTypeStream,
		Stream: &WeWorkStreamContent{
			ID:      streamID,
			Finish:  finish,
			Content: content,
		},
	}
}

// ToJSON 转换为JSON字符串
func (r *WeWorkResponse) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}
