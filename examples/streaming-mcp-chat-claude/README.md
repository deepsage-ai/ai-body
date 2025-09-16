# AI-Body Claude 智能流式对话 (MCP增强版)

基于 agent-sdk-go 的 Claude 版本智能对话系统，集成了 MCP (Model Context Protocol) 支持和会话级连接管理。

## 特性

- ✅ **Claude 集成**: 支持最新的 Claude Sonnet 4 模型
- ✅ **原生思维链**: Claude 3.7+ 支持思维过程展示  
- ✅ **MCP 工具调用**: 智能工具选择和调用
- ✅ **流式传输**: 实时响应显示
- ✅ **会话级连接管理**: 高效的 MCP 连接复用
- ✅ **健康检查**: 自动检测和重建失效连接

## 环境配置

### 必需环境变量

```bash
# Anthropic API 密钥 (必需)
export ANTHROPIC_API_KEY="your_anthropic_api_key_here"
```

### 可选环境变量

```bash
# Claude 模型选择 (可选，默认使用 Claude Sonnet 4)
export CLAUDE_MODEL="claude-sonnet-4-20250514"
```

### 支持的 Claude 模型

| 模型常量 | 模型名称 | 特性 |
|---------|---------|------|
| `anthropic.ClaudeSonnet4` | `claude-sonnet-4-20250514` | 最新模型，思维链支持 |
| `anthropic.Claude37Sonnet` | `claude-3-7-sonnet-20250219` | 思维链支持 |
| `anthropic.Claude35Sonnet` | `claude-3-5-sonnet-latest` | 平衡性能 |
| `anthropic.Claude35Haiku` | `claude-3-5-haiku-latest` | 快速响应 |
| `anthropic.Claude3Opus` | `claude-3-opus-latest` | 最高质量 |

## 快速开始

### 1. 设置环境变量

```bash
# 设置 Anthropic API 密钥
export ANTHROPIC_API_KEY="your_api_key"

# (可选) 选择特定模型
export CLAUDE_MODEL="claude-sonnet-4-20250514"
```

### 2. 运行程序

```bash
cd examples/streaming-mcp-chat-claude
go run main.go
```

### 3. 交互命令

- **普通对话**: 直接输入问题
- **查看工具**: 输入 `tools` 查看可用的 MCP 工具
- **退出程序**: 输入 `exit` 或 `quit`

## MCP 集成

程序会自动连接到配置的 MCP 服务器 (`http://sn.7soft.cn/sse`)，发现并集成以下工具：

- `currentTime` - 获取当前时间 (支持多时区)
- `getBrandList` - 获取品牌列表
- `getBrandTemplate` - 获取品牌模板
- `getNickname` - 获取用户昵称
- `getSnDetail` - 获取序列号详情
- `updateBrandTemplate` - 更新品牌模板

## 连接管理特性

### 会话级连接复用
- 2分钟内的多次工具调用复用同一连接
- 自动健康检查，失效时重建连接
- 使用时验证，确保连接可用性

### 无缓存设计
- 时间相关工具每次返回最新结果
- 状态查询工具保证数据实时性
- 避免副作用工具的重复执行问题

## 示例对话

```
你: 查询上海时间
AI: 当前中国标准时间（CST）为：2025年09月15日 16:07:35

你: 获取品牌列表 租户ID 123
AI: [调用getBrandList工具获取租户123的品牌信息...]

你: tools
AI: [显示所有可用的MCP工具及其参数说明]
```

## 技术架构

### 核心组件
- **Claude 客户端**: 基于 agent-sdk-go 的 anthropic 包
- **SessionMCPManager**: 会话级连接管理器
- **流式处理**: 支持实时响应显示
- **工具集成**: 动态发现和调用 MCP 工具

### 连接生命周期
1. **创建**: 首次工具调用时建立连接
2. **复用**: 2分钟内复用现有连接
3. **检查**: 每次使用前验证连接健康状态
4. **重建**: 检测到失效时自动重建
5. **清理**: 超时后自动清理资源

## 故障排除

### 常见问题

**Q: API 密钥错误**
```
错误: 请设置 ANTHROPIC_API_KEY 环境变量
```
A: 确保正确设置了 `ANTHROPIC_API_KEY` 环境变量

**Q: 连接失败**
```
Failed to list MCP tools: connection closed
```
A: 检查 MCP 服务器状态，程序会自动重试连接

**Q: 工具调用失败**
```
不支持的时区: Asia/Shanghai
```
A: 检查工具参数是否符合枚举值要求，如时区应使用 `CST` 而非 `Asia/Shanghai`

### 调试模式

程序输出详细的状态信息：
- `[SessionMCP] 创建新连接` - 新建连接
- `[SessionMCP] 复用现有连接` - 连接复用
- `[SessionMCP] 连接失效，重建连接` - 自动重建
- `[SessionMCP] 调用工具: xxx` - 工具调用

## 与 Ollama 版本的区别

| 特性 | Claude 版本 | Ollama 版本 |
|------|------------|------------|
| 模型 | Claude Sonnet 4 | qwen3:32b |
| 思维链 | ✅ 原生支持 | ❌ 不支持 |
| 中文理解 | ✅ 优秀 | ✅ 良好 |
| 推理能力 | ✅ 更强 | ✅ 良好 |
| 时区识别 | ✅ 智能识别 CST | ❌ 容易混淆 |
| API 成本 | 💰 付费 | 🆓 免费 |
| 本地部署 | ❌ 云服务 | ✅ 支持 |

## 许可证

本项目遵循原 AI-Body 项目的许可证。