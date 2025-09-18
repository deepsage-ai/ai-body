package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/bot"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/config"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/wework"
)

func main() {
	// 解析命令行参数
	var configPath string
	flag.StringVar(&configPath, "config", "config.json", "配置文件路径")
	flag.StringVar(&configPath, "c", "config.json", "配置文件路径 (短参数)")
	flag.Parse()

	// 显示启动信息
	fmt.Println("🚀 启动 AI-Body 企业微信智能机器人（Python流式模式）...")
	fmt.Println("严格模拟Python示例实现，基于TaskCache任务缓存机制实现伪流传输")

	// 加载配置
	fmt.Printf("📋 加载配置文件: %s\n", configPath)
	cfg, err := config.LoadConfigFromFile(configPath)
	if err != nil {
		log.Fatalf("❌ 配置加载失败: %v", err)
	}

	// 显示配置信息（掩码敏感信息）
	fmt.Printf("📋 企业微信配置: Token=%s, AESKey=%s, BotID=%s\n",
		maskSecret(cfg.WeWork.Token), maskSecret(cfg.WeWork.AESKey), maskSecret(cfg.WeWork.BotID))
	fmt.Printf("🤖 LLM配置: 默认=%s, 提供商数=%d\n",
		cfg.LLM.Default, len(cfg.LLM.Providers))
	fmt.Printf("🔧 MCP服务器: 配置数=%d\n", len(cfg.MCP.Servers))

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
		cfg.WeWork.Token,
		cfg.WeWork.AESKey,
		cfg.WeWork.BotID,
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
	fmt.Printf("\n🌐 企业微信机器人服务启动在: http://localhost:%s\n", cfg.Server.Port)
	fmt.Printf("📡 Webhook地址: http://localhost:%s/b0dy/webhook\n", cfg.Server.Port)
	fmt.Printf("❤️  健康检查: http://localhost:%s/b0dy/health\n", cfg.Server.Port)

	fmt.Println("\n📖 配置说明:")
	fmt.Println("1. 确保已在企业微信后台配置Webhook URL")
	fmt.Println("2. 设置正确的Token和AESKey")
	fmt.Println("3. 确保服务器可被企业微信访问")
	fmt.Println("4. 请配置有效的LLM API密钥（如需要）")

	fmt.Printf("\n🔧 当前配置:\n")
	fmt.Printf("   Token: %s\n", maskSecret(cfg.WeWork.Token))
	fmt.Printf("   AESKey: %s\n", maskSecret(cfg.WeWork.AESKey))
	fmt.Printf("   BotID: %s\n", maskSecret(cfg.WeWork.BotID))
	fmt.Printf("   默认LLM: %s\n", cfg.LLM.Default)

	fmt.Println("\n🎯 核心特性:")
	fmt.Println("✅ 严格按照Python官方示例实现流式机制")
	fmt.Println("✅ TaskCache任务缓存管理（模拟LLMDemo）")
	fmt.Println("✅ 完全复用qwen-http的SessionMCP逻辑")
	fmt.Println("✅ 消息加密解密和签名验证")
	fmt.Println("✅ 企业微信伪流传输（finish=false触发轮询）")
	fmt.Println("✅ 自动去重和错误处理")
	fmt.Println("✅ 实时AI工具调用")

	fmt.Println("\n🚀 服务已启动，等待企业微信消息...")

	// 启动服务器
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("❌ 服务启动失败: %v", err)
	}
}

// maskSecret 掩码敏感信息
func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}
