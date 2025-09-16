package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/bot"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/config"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/wework"
)

func main() {
	// 显示启动信息
	fmt.Println("🚀 启动 AI-Body 企业微信智能机器人（流式版本）...")
	fmt.Println("基于qwen-http版本，完整复用SessionMCP管理和流式处理逻辑")

	// 加载配置
	fmt.Println("📋 加载配置...")
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ 配置加载失败: %v", err)
	}

	// 显示配置信息（掩码敏感信息）
	fmt.Printf("📋 企业微信配置: Token=%s, AESKey=%s\n",
		cfg.MaskSecret(cfg.Token), cfg.MaskSecret(cfg.AESKey))
	fmt.Printf("🤖 千问配置: Model=%s, BaseURL=%s\n",
		cfg.QwenModel, cfg.QwenBaseURL)
	fmt.Printf("🔧 MCP服务器: %s\n", cfg.MCPServerURL)

	// 初始化机器人处理器
	fmt.Println("🤖 初始化AI机器人...")
	botHandler, err := bot.NewBotHandler(cfg)
	if err != nil {
		log.Fatalf("❌ 机器人初始化失败: %v", err)
	}
	defer botHandler.Close()
	fmt.Println("✅ AI机器人初始化完成")

	// 初始化Webhook处理器
	fmt.Println("🔒 初始化Webhook处理器...")
	webhookHandler, err := wework.NewWebhookHandler(
		cfg.Token,
		cfg.AESKey,
		botHandler,
	)
	if err != nil {
		log.Fatalf("❌ Webhook处理器初始化失败: %v", err)
	}
	fmt.Println("✅ Webhook处理器初始化完成")

	// 创建Gin引擎
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 添加CORS中间件（可选）
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 路由配置
	r.Any("/b0dy/webhook", webhookHandler.HandleWebhook) // 企业微信Webhook
	r.GET("/b0dy/health", webhookHandler.HealthCheck)    // 健康检查

	// 显示服务信息
	fmt.Printf("\n🌐 企业微信机器人服务启动在: http://localhost:%s\n", cfg.Port)
	fmt.Printf("📡 Webhook地址: http://localhost:%s/webhook\n", cfg.Port)
	fmt.Printf("❤️  健康检查: http://localhost:%s/health\n", cfg.Port)

	fmt.Println("\n📖 配置说明:")
	fmt.Println("1. 确保已在企业微信后台配置Webhook URL")
	fmt.Println("2. 设置正确的Token和AESKey")
	fmt.Println("3. 确保服务器可被企业微信访问")
	fmt.Println("4. 请配置有效的千问API密钥")

	fmt.Printf("\n🔧 预设配置:\n")
	fmt.Printf("   Token: %s\n", cfg.MaskSecret(cfg.Token))
	fmt.Printf("   AESKey: %s\n", cfg.MaskSecret(cfg.AESKey))

	fmt.Println("\n🎯 核心特性:")
	fmt.Println("✅ 企业微信原生流式传输")
	fmt.Println("✅ 完全复用qwen-http的SessionMCP逻辑")
	fmt.Println("✅ 消息加密解密和签名验证")
	fmt.Println("✅ 自动去重和错误处理")
	fmt.Println("✅ 实时AI工具调用")

	fmt.Println("\n🚀 服务已启动，等待企业微信消息...")

	// 启动服务器
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("❌ 服务启动失败: %v", err)
	}
}
