package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"

	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/config"
	"github.com/deepsage-ai/b0dy/examples/agent-wework/internal/session"
)

// CreateMCPServersFromConfig 根据配置创建MCP服务器列表
func CreateMCPServersFromConfig(cfg *config.Config) ([]interfaces.MCPServer, error) {
	var servers []interfaces.MCPServer

	for _, serverConfig := range cfg.MCP.Servers {
		// 检查是否通过环境变量禁用
		if isDisabledByEnv(serverConfig.Name) {
			fmt.Printf("⏭️  跳过MCP服务器: %s (被环境变量禁用)\n", serverConfig.Name)
			continue
		}

		if !serverConfig.Enabled {
			fmt.Printf("⏭️  跳过MCP服务器: %s (配置中禁用)\n", serverConfig.Name)
			continue
		}

		// 处理环境变量
		processServerEnvVars(&serverConfig)

		server, err := createMCPServer(serverConfig)
		if err != nil {
			fmt.Printf("⚠️  警告: 创建MCP服务器 '%s' 失败: %v\n", serverConfig.Name, err)
			continue
		}

		// HTTP类型包装为SessionMCPManager以支持连接复用
		if serverConfig.Type == "http" {
			sessionManager := session.NewSessionMCPManager(serverConfig.BaseURL)

			// 尝试初始连接测试
			testCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, testErr := sessionManager.ListTools(testCtx)
			if testErr != nil {
				// 分析错误类型并提供友好提示
				errMsg := analyzeConnectionError(serverConfig.Name, serverConfig.BaseURL, testErr)
				fmt.Printf("⚠️  警告: MCP服务器 '%s' 连接测试失败\n%s", serverConfig.Name, errMsg)
				fmt.Printf("   ℹ️  该服务器将被跳过，但您仍可以启动服务\n\n")
				continue
			}

			servers = append(servers, sessionManager)
			fmt.Printf("✅ 配置MCP服务器: %s (HTTP/SSE，连接正常)\n", serverConfig.Name)
		} else {
			servers = append(servers, server)
			fmt.Printf("✅ 配置MCP服务器: %s (Stdio)\n", serverConfig.Name)
		}
	}

	// 检查是否有额外的MCP服务器通过环境变量添加
	if extraServer := os.Getenv("MCP_EXTRA_SERVER"); extraServer != "" {
		sessionManager := session.NewSessionMCPManager(extraServer)
		servers = append(servers, sessionManager)
		fmt.Printf("✅ 添加额外MCP服务器: %s (通过环境变量)\n", extraServer)
	}

	// 显示MCP服务器配置汇总
	if len(servers) > 0 {
		fmt.Printf("✅ MCP工具服务配置完成，成功加载 %d 个服务器\n", len(servers))
	}

	return servers, nil
}

// createMCPServer 创建单个MCP服务器
func createMCPServer(config config.MCPServerConfig) (interfaces.MCPServer, error) {
	ctx := context.Background()

	switch config.Type {
	case "http":
		return mcp.NewHTTPServer(ctx, mcp.HTTPServerConfig{
			BaseURL: config.BaseURL,
			Path:    config.Path,
			Token:   config.Token,
		})

	case "stdio":
		// 构建环境变量列表
		var env []string
		for k, v := range config.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

		return mcp.NewStdioServer(ctx, mcp.StdioServerConfig{
			Command: config.Command,
			Args:    config.Args,
			Env:     env,
		})

	default:
		return nil, fmt.Errorf("unsupported MCP server type: %s", config.Type)
	}
}

// processServerEnvVars 处理服务器配置中的环境变量引用
func processServerEnvVars(config *config.MCPServerConfig) {
	// 处理BaseURL中的环境变量
	config.BaseURL = processEnvVar(config.BaseURL)
	config.Token = processEnvVar(config.Token)

	// 处理Env映射中的环境变量
	for k, v := range config.Env {
		config.Env[k] = processEnvVar(v)
	}
}

// processEnvVar 处理环境变量引用 ${VAR_NAME}
func processEnvVar(value string) string {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		envVar := strings.Trim(value, "${}")
		return os.Getenv(envVar)
	}
	return value
}

// analyzeConnectionError 分析连接错误并返回友好的错误信息
func analyzeConnectionError(serverName, url string, err error) string {
	var msg strings.Builder

	msg.WriteString(fmt.Sprintf("   🔗 服务器: %s\n", url))
	msg.WriteString(fmt.Sprintf("   ❌ 错误: %v\n", err))

	// 分析具体错误类型
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "connection refused"):
		msg.WriteString("   💡 可能原因: MCP服务器未启动或端口错误\n")
		msg.WriteString("   💡 解决方案: \n")
		msg.WriteString("      1. 确认MCP服务器已启动\n")
		msg.WriteString("      2. 检查端口是否正确\n")
		msg.WriteString("      3. 检查防火墙设置\n")

	case strings.Contains(errStr, "timeout"):
		msg.WriteString("   💡 可能原因: 网络超时或服务器响应慢\n")
		msg.WriteString("   💡 解决方案: \n")
		msg.WriteString("      1. 检查网络连接\n")
		msg.WriteString("      2. 确认服务器地址可访问\n")
		msg.WriteString("      3. 增加超时时间\n")

	case strings.Contains(errStr, "no such host"):
		msg.WriteString("   💡 可能原因: 域名无法解析\n")
		msg.WriteString("   💡 解决方案: \n")
		msg.WriteString("      1. 检查域名拼写\n")
		msg.WriteString("      2. 确认DNS设置\n")
		msg.WriteString("      3. 尝试使用IP地址\n")

	case strings.Contains(errStr, "404"):
		msg.WriteString("   💡 可能原因: MCP端点路径错误\n")
		msg.WriteString("   💡 解决方案: \n")
		msg.WriteString("      1. 确认MCP服务器的正确路径\n")
		msg.WriteString("      2. 检查是否需要添加路径后缀\n")

	default:
		msg.WriteString("   💡 解决方案: \n")
		msg.WriteString("      1. 检查服务器地址和端口\n")
		msg.WriteString("      2. 确认服务器已启动\n")
		msg.WriteString("      3. 查看服务器日志\n")
	}

	return msg.String()
}

// isDisabledByEnv 检查是否通过环境变量禁用了某个MCP服务器
func isDisabledByEnv(serverName string) bool {
	// 将服务器名称转换为环境变量格式
	// 例如: "7soft-tools" -> "MCP_DISABLE_7SOFT_TOOLS"
	envName := "MCP_DISABLE_" + strings.ToUpper(strings.ReplaceAll(serverName, "-", "_"))
	return os.Getenv(envName) == "true"
}
