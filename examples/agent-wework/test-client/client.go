package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/wework"
)

const (
	// 企业微信配置（与服务端保持一致）
	TOKEN   = "9hLM5K4pnxRu8d"
	AES_KEY = "E2852LABnwUkzMQKciaNNDG2fhOOlQ2kCIwCHNZnrVa"
	BOT_ID  = "aib2luFCOChzgjguHi58WvVgwjJoeAHgkQo"

	// 服务器地址
	SERVER_URL = "http://localhost:8889/b0dy/webhook"

	// 固定的用户ID
	USER_ID = "test-user-001"

	// 颜色定义
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
)

// 简单的消息ID生成器
var msgCounter = 0

func generateMsgID() string {
	msgCounter++
	return fmt.Sprintf("msg_%d_%d", time.Now().Unix(), msgCounter)
}

// 计算签名
func calculateSignature(token, timestamp, nonce, encrypt string) string {
	// 将参数排序
	params := []string{token, timestamp, nonce, encrypt}
	sort.Strings(params)

	// 拼接字符串
	str := strings.Join(params, "")

	// SHA1加密
	h := sha1.New()
	h.Write([]byte(str))

	// 返回十六进制字符串
	return fmt.Sprintf("%x", h.Sum(nil))
}

func main() {
	fmt.Printf("%s🤖 企业微信智能助手测试客户端%s\n", ColorCyan, ColorReset)
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("服务器地址: %s%s%s\n", ColorYellow, SERVER_URL, ColorReset)
	fmt.Printf("用户ID: %s%s%s\n", ColorYellow, USER_ID, ColorReset)
	fmt.Printf("机器人ID: %s%s%s\n", ColorYellow, BOT_ID, ColorReset)
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Printf("%s提示: 输入消息并按回车发送，输入 'exit' 退出%s\n", ColorGray, ColorReset)
	fmt.Println()

	// 初始化加密器
	wxcpt, err := wework.NewWXBizJsonMsgCrypt(TOKEN, AES_KEY, "") // 智能机器人场景receiverId使用空字符串
	if err != nil {
		fmt.Printf("%s❌ 初始化加密器失败: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Printf("%s你: %s", ColorGreen, ColorReset)
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "exit" {
			fmt.Printf("%s👋 再见!%s\n", ColorYellow, ColorReset)
			break
		}

		if input == "" {
			continue
		}

		// 记录发送时间
		startTime := time.Now()

		// 发送消息并获取响应
		response, streamID, err := sendMessage(wxcpt, input)
		if err != nil {
			fmt.Printf("%s❌ 错误: %v%s\n", ColorRed, err, ColorReset)
			continue
		}

		// 如果返回了流式ID，处理流式响应
		if streamID != "" {
			response, err = handleStreamResponse(wxcpt, streamID, startTime)
			if err != nil {
				fmt.Printf("%s❌ 流式处理错误: %v%s\n", ColorRed, err, ColorReset)
			}
			fmt.Println() // 流式响应后额外换行
		} else {
			// 非流式响应，正常输出
			elapsed := time.Since(startTime)
			fmt.Printf("%s🤖 小兴: %s%s %s(耗时: %.2fs)%s\n\n",
				ColorPurple, ColorReset, response, ColorGray, elapsed.Seconds(), ColorReset)
		}
	}
}

// sendMessage 发送消息到服务器
func sendMessage(wxcpt *wework.WXBizJsonMsgCrypt, content string) (string, string, error) {
	// 构造消息
	msg := wework.IncomingMessage{
		BaseMessage: wework.BaseMessage{
			MsgID:    generateMsgID(),
			AIBotID:  BOT_ID,
			ChatType: wework.ChatTypeSingle,
			From: wework.From{
				UserID: USER_ID,
			},
			MsgType: wework.MsgTypeText,
		},
		Text: &wework.TextContent{
			Content: content,
		},
	}

	// 序列化消息
	msgData, err := json.Marshal(msg)
	if err != nil {
		return "", "", fmt.Errorf("序列化消息失败: %w", err)
	}

	// 加密消息
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := fmt.Sprintf("nonce_%d", time.Now().UnixNano())

	// EncryptMsg 返回的就是JSON格式的加密消息
	ret, encryptedJSON, err := wxcpt.EncryptMsg(string(msgData), nonce, &timestamp)
	if ret != wework.WXBizMsgCrypt_OK || err != nil {
		return "", "", fmt.Errorf("加密消息失败: ret=%d, err=%w", ret, err)
	}

	// 解析JSON获取msg_signature
	var encryptedData map[string]string
	if err := json.Unmarshal([]byte(encryptedJSON), &encryptedData); err != nil {
		return "", "", fmt.Errorf("解析加密JSON失败: %w", err)
	}

	// 重新生成紧凑的JSON（去除格式化的空格和换行）
	compactJSON, err := json.Marshal(map[string]string{
		"encrypt": encryptedData["encrypt"],
	})
	if err != nil {
		return "", "", fmt.Errorf("生成紧凑JSON失败: %w", err)
	}

	msgSignature := encryptedData["msgsignature"]

	// 调试输出
	fmt.Printf("%s[调试] 原始加密JSON:%s\n%s\n", ColorGray, ColorReset, encryptedJSON)
	fmt.Printf("%s[调试] 紧凑JSON:%s\n%s\n", ColorGray, ColorReset, string(compactJSON))
	fmt.Printf("%s[调试] timestamp=%s, nonce=%s, msg_signature=%s%s\n",
		ColorGray, timestamp, nonce, msgSignature, ColorReset)

	// 发送HTTP请求，msg_signature从加密结果中获取
	// 注意：需要对msg_signature进行URL编码
	requestURL := fmt.Sprintf("%s?timestamp=%s&nonce=%s&msg_signature=%s",
		SERVER_URL, timestamp, nonce, url.QueryEscape(msgSignature))

	// 创建HTTP客户端，设置超时
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Body发送紧凑的JSON（只包含encrypt字段）
	resp, err := client.Post(requestURL, "text/plain", strings.NewReader(string(compactJSON)))
	if err != nil {
		return "", "", fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("服务器返回错误: %d, %s", resp.StatusCode, string(body))
	}

	// 如果是成功但无内容，说明是异步处理
	if string(body) == "success" {
		return "消息已接收，正在处理中...", "", nil
	}

	// 尝试解密响应
	if len(body) > 0 {
		// 先尝试解析响应JSON，获取签名
		var respData map[string]string
		if err := json.Unmarshal(body, &respData); err == nil {
			// 如果是加密的JSON响应
			if respMsgSignature, ok := respData["msgsignature"]; ok {
				// 使用响应中的签名进行解密
				ret, decryptedMsg, err := wxcpt.DecryptMsg(string(body), respMsgSignature, timestamp, nonce)
				if ret == wework.WXBizMsgCrypt_OK && err == nil {
					// 解析响应消息
					var response wework.WeWorkResponse
					if err := json.Unmarshal([]byte(decryptedMsg), &response); err == nil {
						if response.Text != nil {
							return response.Text.Content, "", nil
						}
						if response.Stream != nil {
							return "", response.Stream.ID, nil
						}
					}
				}
			}
		}
	}

	return fmt.Sprintf("收到响应但无法解析: %s", string(body)), "", nil
}

// handleStreamResponse 处理流式响应
func handleStreamResponse(wxcpt *wework.WXBizJsonMsgCrypt, streamID string, startTime time.Time) (string, error) {
	fmt.Printf("%s🤖 小兴: %s", ColorPurple, ColorReset)

	var fullContent string
	var displayedLen int
	maxRetries := 60 // 最多重试60次（约60秒）

	for i := 0; i < maxRetries; i++ {
		time.Sleep(500 * time.Millisecond) // 每0.5秒刷新一次，更快的响应

		// 发送刷新请求
		content, finished, err := sendStreamRefresh(wxcpt, streamID)
		if err != nil {
			// 如果是暂时性错误，继续重试
			if i < 5 {
				continue
			}
			return fullContent, fmt.Errorf("刷新失败: %w", err)
		}

		fullContent = content // 累积模式，每次都是完整内容

		// 打印机效果：只打印新增的内容
		if len(fullContent) > displayedLen {
			newContent := fullContent[displayedLen:]
			fmt.Print(newContent) // 直接打印新内容，不换行
			displayedLen = len(fullContent)
		}

		if finished {
			elapsed := time.Since(startTime)
			fmt.Printf(" %s(耗时: %.2fs)%s\n", ColorGray, elapsed.Seconds(), ColorReset)
			return fullContent, nil
		}
	}

	fmt.Println() // 换行
	return fullContent, fmt.Errorf("流式响应超时")
}

// sendStreamRefresh 发送流式刷新请求
func sendStreamRefresh(wxcpt *wework.WXBizJsonMsgCrypt, streamID string) (string, bool, error) {
	// 构造流式刷新消息
	msg := wework.IncomingMessage{
		BaseMessage: wework.BaseMessage{
			MsgID:   generateMsgID(),
			AIBotID: BOT_ID,
			From: wework.From{
				UserID: USER_ID,
			},
			MsgType: wework.MsgTypeStream,
		},
		Stream: &wework.StreamContent{
			ID: streamID,
		},
	}

	// 序列化消息
	msgData, err := json.Marshal(msg)
	if err != nil {
		return "", false, err
	}

	// 加密消息
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := fmt.Sprintf("refresh_%d", time.Now().UnixNano())

	// EncryptMsg 返回JSON格式
	ret, encryptedJSON, err := wxcpt.EncryptMsg(string(msgData), nonce, &timestamp)
	if ret != wework.WXBizMsgCrypt_OK || err != nil {
		return "", false, fmt.Errorf("加密失败: ret=%d, err=%w", ret, err)
	}

	// 解析JSON获取msg_signature
	var encryptedData map[string]string
	if err := json.Unmarshal([]byte(encryptedJSON), &encryptedData); err != nil {
		return "", false, fmt.Errorf("解析加密JSON失败: %w", err)
	}

	// 重新生成紧凑的JSON（去除格式化的空格和换行）
	compactJSON, err := json.Marshal(map[string]string{
		"encrypt": encryptedData["encrypt"],
	})
	if err != nil {
		return "", false, fmt.Errorf("生成紧凑JSON失败: %w", err)
	}

	msgSignature := encryptedData["msgsignature"]

	// 发送请求
	requestURL := fmt.Sprintf("%s?timestamp=%s&nonce=%s&msg_signature=%s",
		SERVER_URL, timestamp, nonce, url.QueryEscape(msgSignature))

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Body发送紧凑的JSON（只包含encrypt字段）
	resp, err := client.Post(requestURL, "text/plain", strings.NewReader(string(compactJSON)))
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("服务器错误: %d", resp.StatusCode)
	}

	// 解密响应
	if len(body) > 0 && string(body) != "success" {
		// 先尝试解析响应JSON，获取签名
		var respData map[string]string
		if err := json.Unmarshal(body, &respData); err == nil {
			// 如果是加密的JSON响应
			if respMsgSignature, ok := respData["msgsignature"]; ok {
				// 使用响应中的签名进行解密
				ret, decryptedMsg, err := wxcpt.DecryptMsg(string(body), respMsgSignature, timestamp, nonce)
				if ret == wework.WXBizMsgCrypt_OK && err == nil {
					var response wework.WeWorkResponse
					if err := json.Unmarshal([]byte(decryptedMsg), &response); err == nil {
						if response.Stream != nil {
							return response.Stream.Content, response.Stream.Finish, nil
						}
					}
				}
			}
		}
	}

	return "", false, fmt.Errorf("无效的流式响应")
}
