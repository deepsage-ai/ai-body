# AI-Body 千问 HTTP API

基于千问版本的流式HTTP API，完整复用SessionMCP连接管理和工具调用逻辑。

## 项目特点

### 🎯 设计原则
- **简约而不简单**：单文件实现，复用完整MCP逻辑
- **完全兼容**：100%复用千问版本的SessionMCPManager
- **真实流式**：基于SSE的实时流式传输
- **独立部署**：独立目录结构，无依赖冲突

### 🏗️ 技术架构
```
HTTP请求 → Gin路由 → Agent智能体 → SessionMCPManager → 外部MCP服务器
    ↓
SSE流式响应 ← 事件处理 ← RunStream() ← MCP工具调用
```

## 快速开始

### 1. 安装依赖
```bash
cd examples/streaming-mcp-chat-qwen-http
go get github.com/gin-gonic/gin
go get github.com/Ingenimax/agent-sdk-go/pkg/agent
go mod tidy
```

### 2. 启动服务
```bash
go run main.go
```

启动后显示：
```
🚀 初始化AI助手（基于千问版本）...
使用千问模型: qwen-max (支持工具调用)
连接到: https://dashscope.aliyuncs.com/compatible-mode/v1
=== MCP按需连接配置 ===
配置会话级MCP管理器: http://sn.7soft.cn/sse
✅ 会话级MCP管理器配置完成（连接复用+去重）
正在测试连接和工具发现...
发现 6 个MCP工具:
  [1] currentTime: 获取当前时间
  [2] getBrandList: 获取品牌列表
  ...
✅ AI助手初始化完成

🌐 HTTP API 服务启动在: http://localhost:8080
📡 聊天端点: POST http://localhost:8080/chat
🛠️  工具查看: GET http://localhost:8080/tools
❤️  健康检查: GET http://localhost:8080/health

基于千问版本，完整复用SessionMCPManager和流式处理逻辑
```

## API接口

### 1. 流式聊天 `POST /chat`

**请求格式：**
```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "获取当前时间"}' \
  --no-buffer
```

**响应格式（SSE）：**
```
data: {"type":"content","content":"当前时间是"}
data: {"type":"content","content":"2024-09-16 15:30:25"}
data: {"type":"content","content":"（北京时间）"}
data: {"type":"done","events":15}
```

### 2. 工具查看 `GET /tools`

**请求：**
```bash
curl -X GET http://localhost:8080/tools
```

**响应：**
```json
{
  "tools": [
    {"name": "currentTime", "description": "获取当前时间"},
    {"name": "getBrandList", "description": "获取品牌列表"},
    {"name": "getProductList", "description": "获取产品列表"}
  ],
  "count": 6
}
```

### 3. 健康检查 `GET /health`

**请求：**
```bash
curl -X GET http://localhost:8080/health
```

**响应：**
```json
{
  "status": "healthy",
  "service": "AI-Body 千问 HTTP API", 
  "mcp_status": "connected",
  "features": ["streaming", "mcp_tools", "session_management"]
}
```

## 核心技术

### SessionMCPManager 连接管理
```go
// 完全复用千问版本的连接管理逻辑
type SessionMCPManager struct {
    baseURL       string
    connection    interfaces.MCPServer
    lastActivity  time.Time
    sessionActive bool
    mutex         sync.RWMutex
}
```

**核心特性：**
- **2分钟连接复用**：活跃期内复用同一连接
- **健康检查**：3秒超时验证连接可用性
- **自动重建**：失效时自动创建新连接
- **Schema转换**：确保LLM正确理解工具参数

### 流式传输处理
```go
// 完全复用千问版本的流式事件处理
eventChan, err := agentInstance.RunStream(ctx, req.Message)
for event := range eventChan {
    if event.Content != "" {
        sseEvent := SSEEvent{Type: "content", Content: event.Content}
        c.SSEvent("", string(data))
        c.Writer.Flush()
    }
}
```

### 千问API优化
- **短期记忆**：`memory.WithMaxSize(3)` 避免工具消息格式冲突
- **无工具缓存**：每次调用返回实时结果
- **格式兼容**：专门适配DashScope API严格要求

## 测试示例

### 基础对话测试
```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "你好，请介绍一下你的功能"}' \
  --no-buffer
```

### MCP工具调用测试
```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "帮我查询当前时间和日期"}' \
  --no-buffer
```

### 复杂任务测试
```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "请获取品牌列表，并告诉我有多少个品牌"}' \
  --no-buffer
```

## 部署配置

### 环境变量（可选）
```bash
# 端口配置（默认8080）
export PORT=8080

# 千问API配置（代码中已硬编码）
export QWEN_API_KEY="sk-0d8bebab081044f682fbeb6c147d8f2c"
export QWEN_MODEL="qwen-max"
export QWEN_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1"

# MCP服务器配置（代码中已硬编码）
export MCP_SERVER_URL="http://sn.7soft.cn/sse"
```

### Docker部署（可选）
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o http-api main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/http-api .
EXPOSE 8080
CMD ["./http-api"]
```

## 错误处理

### 常见错误及解决方案

1. **MCP连接失败**
   ```
   Warning: 测试连接失败: connection refused
   ```
   - 检查MCP服务器 `http://sn.7soft.cn/sse` 是否可访问
   - 网络连接是否正常

2. **千问API错误**
   ```
   400 Bad Request: messages with role "tool" must be...
   ```
   - 已通过 `memory.WithMaxSize(3)` 自动解决
   - 短期记忆避免消息格式冲突

3. **Gin依赖缺失**
   ```
   cannot find package "github.com/gin-gonic/gin"
   ```
   - 运行 `go get github.com/gin-gonic/gin`
   - 运行 `go mod tidy`

## 技术对比

| 特性 | 控制台版本 | HTTP API版本 |
|-----|----------|-------------|
| **用户交互** | 控制台输入 | HTTP POST请求 |
| **响应方式** | 终端输出 | SSE流式传输 |
| **MCP逻辑** | ✅ 完全一致 | ✅ 完全复用 |
| **SessionMCP** | ✅ 原始实现 | ✅ 100%复用 |
| **工具调用** | ✅ 支持 | ✅ 支持 |
| **连接管理** | ✅ 2分钟复用 | ✅ 2分钟复用 |
| **部署方式** | 本地运行 | HTTP服务 |

## 开发说明

### 代码结构
```
streaming-mcp-chat-qwen-http/
├── main.go          # 主程序文件
├── go.mod          # Go模块配置
├── go.sum          # 依赖校验文件
└── README.md       # 项目文档
```

### 关键实现
- **完全复用**：SessionMCPManager代码与千问版本完全一致
- **最小改动**：仅替换交互层，核心逻辑不变
- **真实流式**：基于 `agentInstance.RunStream()` 的真实流式传输
- **简约设计**：单文件实现，无复杂目录结构

这个HTTP API版本展示了如何在保持核心功能完整性的同时，实现简约而优雅的架构设计。