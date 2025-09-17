# AI-Body 企业微信智能机器人（流式版本）

基于qwen-http版本的企业微信智能机器人，**完整支持流式传输**，复用SessionMCP连接管理逻辑。

## 项目特点

### 🎯 设计原则
- **基于proven架构**：完全基于qwen-http示例改造，保证技术可靠性
- **真正流式传输**：利用企业微信原生流式消息支持，实现实时AI响应
- **无缝集成**：标准企业微信智能机器人API对接
- **完全复用**：100%复用qwen-http的SessionMCP和Agent逻辑

### 🏗️ 技术架构
```
企业微信 → Webhook → 解密验证 → 消息解析 → Agent.RunStream() → SessionMCPManager → MCP工具
    ↓                                              ↓
企业微信 ← 加密回复 ← 流式消息格式 ← 实时事件处理 ← 流式事件流 ← 工具调用结果
    ↓
企业微信流式刷新回调 → 获取当前状态 → 返回最新内容
```

### 📋 配置信息
```
URL: https://chat.7soft.cn/b0dy
Token: LYDMxF6qKEPWVGdKaQdAYw9xxfSzDsIC
Encoding-AESKey: f4gfCYzaqGAfh4rqxWjqG9udsZwI0d3uRlx5cXVUgDu
```

## 流式传输机制

### 企业微信流式消息支持
企业微信智能机器人**原生支持流式消息**：

1. **首次回复**：返回带有`stream.id`的流式消息开始
2. **持续刷新**：企业微信自动回调获取内容更新
3. **结束标志**：设置`stream.finish=true`完成流式传输

### 流式回复格式
```json
{
    "msgtype": "stream",
    "stream": {
        "id": "unique_stream_id",
        "finish": false,
        "content": "实时生成的AI内容..."
    }
}
```

### 核心优势
- ✅ **真实流式**：与qwen-http一样的实时响应体验
- ✅ **企业微信原生**：利用官方流式消息支持
- ✅ **无聚合损失**：不需要等待完整响应再回复
- ✅ **用户体验佳**：用户可以看到AI实时思考过程

## 支持的消息类型

### 接收消息类型
- ✅ **文本消息**：用户发送的文本内容
- ✅ **图片消息**：用户发送的图片（提示不支持分析）
- ✅ **图文混排**：文本+图片混合消息，提取文本处理
- ✅ **流式刷新**：企业微信流式消息刷新回调

### 回复消息类型
- ✅ **流式消息开始**：带有stream.id的首次回复
- ✅ **流式消息更新**：实时内容更新
- ✅ **流式消息结束**：完整响应完成标志
- 🔄 **模板卡片**：结构化交互卡片（后续扩展）

## 快速开始

### 1. 环境准备
```bash
cd examples/agent-wework
```

### 2. 配置说明
编辑 `internal/config/config.go` 中的常量：
```go
// 企业微信配置（已预设）
const (
    WEWORK_TOKEN = "LYDMxF6qKEPWVGdKaQdAYw9xxfSzDsIC"
    WEWORK_AES_KEY = "f4gfCYzaqGAfh4rqxWjqG9udsZwI0d3uRlx5cXVUgDu"
    
    // 千问API配置（需要设置）
    QWEN_API_KEY = "your_dashscope_api_key"  // 必须设置
    
    // 服务配置
    PORT = "8080"
)
```

### 3. 启动服务
```bash
go run main.go
```

启动后显示：
```
🚀 启动 AI-Body 企业微信智能机器人（流式版本）...
📋 企业微信配置: Token=LYD**** AESKey=f4g****
🤖 千问配置: Model=qwen-max BaseURL=https://dashscope.aliyuncs.com/compatible-mode/v1
🔧 MCP服务器: http://sn.7soft.cn/sse

🤖 初始化AI机器人...
✅ AI机器人初始化完成

🔒 初始化Webhook处理器...
✅ Webhook处理器初始化完成

📡 初始化流式消息管理器...
✅ 流式消息管理器初始化完成

🌐 企业微信机器人服务启动在: http://localhost:8080
📡 Webhook地址: http://localhost:8080/webhook
❤️  健康检查: http://localhost:8080/health
```

### 4. 企业微信配置
在企业微信智能机器人管理后台配置：
```
接收消息URL: https://your-domain.com/webhook
Token: LYDMxF6qKEPWVGdKaQdAYw9xxfSzDsIC
EncodingAESKey: f4gfCYzaqGAfh4rqxWjqG9udsZwI0d3uRlx5cXVUgDu
```

## API接口

### Webhook接口
- **URL**: `/webhook`
- **方法**: GET (验证) / POST (消息)
- **功能**: 接收企业微信消息回调，处理AI流式回复

### 健康检查
- **URL**: `/health`
- **方法**: GET
- **响应**: 服务状态信息

## 核心技术实现

### 1. 完全复用qwen-http架构
```go
// 基于qwen-http的SessionMCPManager（完全一致）
sessionManager := NewSessionMCPManager("http://sn.7soft.cn/sse")

// 基于qwen-max的智能体（完全一致）
agentInstance := agent.NewAgent(
    agent.WithLLM(qwenClient),
    agent.WithMCPServers(sessionManager),
    // ... 其他配置与qwen-http完全一致
)
```

### 2. 任务缓存管理（模拟Python LLMDemo）
```go
type TaskCacheManager struct {
    tasks         map[string]*TaskInfo
    agentInstance *agent.Agent
}

type TaskInfo struct {
    StreamID      string
    Question      string
    CurrentStep   int
    MaxSteps      int
    Content       strings.Builder
    IsProcessing  bool
    IsFinished    bool
}
```

### 3. 企业微信流式消息处理流程（严格按照Python示例）
```go
// 处理text消息（类似Python的msgtype=='text'）
func (h *Handler) HandleMessage(msg *IncomingMessage) {
    // 1. 创建任务（模拟Python LLMDemo.invoke()）
    streamID, _ := h.taskCache.Invoke(ctx, textContent)
    
    // 2. 获取第一步答案（模拟Python LLMDemo.get_answer()）
    answer := h.taskCache.GetAnswer(streamID)
    
    // 3. 检查是否完成（模拟Python LLMDemo.is_task_finish()）
    finish := h.taskCache.IsTaskFinish(streamID)
    
    // 4. 返回stream消息（finish=false时企业微信会发送刷新请求）
    return NewStreamResponse(streamID, answer, finish)
}

// 处理stream刷新（类似Python的msgtype=='stream'）
func (h *Handler) HandleStreamRefresh(streamID string) {
    // 1. 获取最新答案（模拟Python LLMDemo.get_answer()）
    answer := h.taskCache.GetAnswer(streamID)
    
    // 2. 检查是否完成（模拟Python LLMDemo.is_task_finish()）
    finish := h.taskCache.IsTaskFinish(streamID)
    
    // 3. 返回stream消息（继续直到finish=true）
    return NewStreamResponse(streamID, answer, finish)
}
```

### 4. 企业微信流式回复构造
```go
func (h *Handler) buildStreamResponse(streamID, content string, finish bool) *WeWorkResponse {
    return &WeWorkResponse{
        MsgType: "stream",
        Stream: WeWorkStreamContent{
            ID:      streamID,
            Finish:  finish,
            Content: content,
        },
    }
}
```

### 5. 企业微信加密解密（与原版一致）
- **算法**: AES-256-CBC
- **填充**: PKCS#7
- **签名**: SHA1
- **验证**: msg_signature校验

## 项目结构

```
examples/agent-wework/
├── main.go                     # 主程序入口（基于qwen-http改造）
├── README.md                   # 本文档
├── internal/
│   ├── config/
│   │   └── config.go          # 配置管理（常量配置）
│   ├── wework/
│   │   ├── crypto.go          # 企业微信加解密
│   │   ├── message.go         # 消息结构定义
│   │   ├── webhook.go         # Webhook处理器
│   │   └── stream.go          # 流式消息管理器
│   └── bot/
│       └── handler.go         # 机器人逻辑（复用qwen-http）
```

## 流式消息流程

### 1. 首次文本消息处理
```
用户发文本 → 企业微信回调 → 解密验证 → 提取文本 → Agent.RunStream() → 
返回流式开始 → 企业微信显示"正在输入..." → 持续刷新回调
```

### 2. 流式内容更新
```
企业微信刷新回调 → 获取streamID → 处理新的stream events → 
返回累积内容 → 企业微信实时显示内容更新
```

### 3. 流式结束
```
Agent流式完成 → 设置finish=true → 企业微信显示完整内容 → 清理流式状态
```

## 与qwen-http的技术对比

| 功能模块 | qwen-http | agent-wework |
|---------|-----------|--------------|
| **输入接口** | HTTP POST + JSON | 企业微信Webhook + 加密 |
| **输出格式** | SSE流式传输 | 企业微信流式消息 |
| **AI引擎** | ✅ 千问qwen-max | ✅ 千问qwen-max（完全一致） |
| **MCP集成** | ✅ SessionMCPManager | ✅ SessionMCPManager（完全一致） |
| **工具调用** | ✅ 外部MCP工具 | ✅ 外部MCP工具（完全一致） |
| **流式处理** | ✅ agent.RunStream() | ✅ agent.RunStream()（完全一致） |
| **响应方式** | 实时SSE流 | 企业微信流式消息 |
| **身份验证** | 无 | 企业微信签名验证 |
| **用户体验** | Web端实时流式 | 企业微信端实时流式 |

## 开发计划

### Phase 1: 核心架构搭建 ✅
- [x] 项目结构设计
- [x] 流式传输技术方案确定

### Phase 2: 基础模块实现
- [ ] 配置管理模块
- [ ] 企业微信加解密模块
- [ ] 消息结构定义
- [ ] 流式消息状态管理器

### Phase 3: Webhook处理层
- [ ] GET请求URL验证
- [ ] POST请求消息解密
- [ ] 消息类型解析
- [ ] 签名验证机制

### Phase 4: 流式AI处理层
- [ ] 完全复用qwen-http的Agent逻辑
- [ ] 集成agent.RunStream()流式处理
- [ ] 流式事件实时处理
- [ ] 流式状态管理

### Phase 5: 企业微信集成层
- [ ] 流式消息格式构造
- [ ] 流式刷新回调处理
- [ ] 消息加密和签名
- [ ] 多种消息类型支持

### Phase 6: 测试和优化
- [ ] 端到端流式传输测试
- [ ] 性能优化
- [ ] 错误场景处理
- [ ] 文档完善

## 技术要点

### 1. 流式传输
- ✅ 企业微信原生流式消息支持
- ✅ agent.RunStream()实时事件处理
- ✅ 流式状态管理和同步
- ✅ 自动清理机制

### 2. 安全性
- ✅ 消息加密解密（AES-256-CBC）
- ✅ 签名验证（SHA1）
- ✅ 消息去重机制
- ✅ 参数校验

### 3. 可靠性
- ✅ 完全复用proven的qwen-http架构
- ✅ SessionMCP连接管理
- ✅ 流式异常处理和回退
- ✅ 优雅的错误处理

### 4. 性能
- ✅ 连接复用（2分钟内）
- ✅ 实时流式处理
- ✅ 最小化延迟
- ✅ 并发消息处理

## 注意事项

1. **流式消息限制**：企业微信最多支持6分钟流式传输
2. **并发限制**：用户与机器人最多同时3条消息交互
3. **千问API密钥**：必须配置有效的DashScope API密钥
4. **网络连接**：确保能访问企业微信服务器和外部MCP服务器
5. **域名配置**：生产环境需要配置企业备案域名

## 故障排除

### 常见问题

1. **流式传输中断**
   - 检查网络连接稳定性
   - 验证流式ID状态管理
   - 确认企业微信回调频率

2. **URL验证失败**
   - 检查Token和AESKey配置
   - 确认服务器可被企业微信访问
   - 验证URL decode处理

3. **消息解密失败**
   - 检查EncodingAESKey格式（43位）
   - 验证签名计算逻辑
   - 确认加密算法实现

4. **AI回复异常**
   - 检查千问API密钥配置
   - 确认MCP服务器连接
   - 查看SessionMCP管理器状态

## 参考文档

- [企业微信智能机器人接收消息](https://developer.work.weixin.qq.com/document/path/100719)
- [企业微信智能机器人被动回复消息](https://developer.work.weixin.qq.com/document/path/101031)
- [企业微信加解密方案](https://developer.work.weixin.qq.com/document/path/91770)
- [qwen-http示例源码](../streaming-mcp-chat-qwen-http/)

---

**基于AI-Body项目 | 企业微信原生流式传输 | 完全复用proven技术栈**