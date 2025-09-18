package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"

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
			servers = append(servers, sessionManager)
			fmt.Printf("✅ 配置MCP服务器: %s (HTTP/SSE，支持连接复用)\n", serverConfig.Name)
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

// isDisabledByEnv 检查是否通过环境变量禁用了某个MCP服务器
func isDisabledByEnv(serverName string) bool {
	// 将服务器名称转换为环境变量格式
	// 例如: "7soft-tools" -> "MCP_DISABLE_7SOFT_TOOLS"
	envName := "MCP_DISABLE_" + strings.ToUpper(strings.ReplaceAll(serverName, "-", "_"))
	return os.Getenv(envName) == "true"
}
