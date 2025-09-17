# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

AI-Body 是一个基于 Go 语言和 agent-sdk-go 框架的智能对话系统。项目已有完整的示例实现，展示了 MCP (Model Context Protocol) 客户端集成和流式对话的核心技术。

**当前状态**: 项目拥有三个完整可运行的示例实现，实现了会话级连接管理等关键技术突破。

## 实现架构

项目包含三个完整的示例实现：

### 1. streaming-mcp-chat (Ollama版本)
- **LLM支持**: Ollama qwen3:32b 模型
- **连接方式**: OpenAI兼容接口
- **特性**: 本地部署，免费使用

### 2. streaming-mcp-chat-claude (Claude版本)  
- **LLM支持**: Claude Sonnet 4 (最新模型)
- **连接方式**: Anthropic API
- **特性**: 原生思维链支持，更强推理能力

### 3. streaming-mcp-chat-qwen (千问版本)
- **LLM支持**: qwen-max (千问最强模型)
- **连接方式**: DashScope兼容接口
- **特性**: 云端部署，开箱即用，中文优化

### 4. streaming-mcp-chat-qwen-http (千问HTTP API版本)
- **LLM支持**: qwen-max (千问最强模型)
- **部署方式**: HTTP API服务
- **特性**: SSE流式传输，完全复用千问版本MCP逻辑

### 5. streaming-mcp-chat-qwen-wework (企业微信智能机器人版本)
- **LLM支持**: qwen-max (千问最强模型)  
- **接入方式**: 企业微信Webhook回调
- **特性**: 消息加密解密，去重机制，完全复用千问版本MCP逻辑

## 项目结构说明

### 统一项目结构
- **go.mod文件**: 位于项目根目录，统一管理所有版本的依赖
- **配置方式**: demo版本直接使用常量配置，避免环境变量复杂性
- **代码复用**: 各版本完全复用核心SessionMCP管理和智能体逻辑

## 各版本技术特点

### simple-chat 技术细节
- **最简代码示例**: 展示 agent-sdk-go 基本使用
- **多租户支持**: 完整的OrgID和ConversationID设置
- **记忆管理**: ConversationBuffer实现多轮对话
- **命令行交互**: bufio.Scanner实现用户输入

### streaming-chat 技术细节  
- **流式事件处理**: RunStream API返回事件channel
- **事件类型**: ContentDelta、ContentComplete、Error等
- **实时显示**: 逐字符输出，无缓冲
- **错误恢复**: 流式错误优雅处理

### MCP版本通用技术
- **SessionMCPManager**: 2分钟连接复用，3秒健康检查
- **工具Schema转换**: jsonschema.Schema转map格式
- **无缓存设计**: 每次工具调用返回实时结果
- **连接生命周期**: 自动检测失效并重建

### HTTP API版本技术
- **Gin框架**: 高性能分布式HTTP服务  
- **SSE实现**: `c.SSEvent` + `c.Writer.Flush()`
- **统一结构**: 完全复用千问SessionMCP代码
- **单文件架构**: main.go实现所有功能

### 企业微信版本技术突破
1. **消息加解密**: 基于Python逻辑的自定义实现
2. **伪流式传输**: finish=false触发企业微信轮询
3. **TaskCacheManager**: 模拟Python LLMDemo任务缓存
4. **StreamBuffer**: 累积模式的线程安全缓冲区  
5. **ConversationAgentManager**: 会话级Agent实例管理
6. **Final Call过滤**: 解决agent-sdk-go双重调用问题

## 开发需求记录

### 企业微信智能机器人版本需求
**要求严格性**: 必须基于准确的官方文档进行开发，不允许推测或假设API格式
**参考文档**: 
- 接收消息文档: https://developer.work.weixin.qq.com/document/path/100719
- 回复消息文档: https://developer.work.weixin.qq.com/document/path/101031
- Python官方示例: `/home/kang/Downloads/aibot_demo_python/`
**开发历程**: 
1. 发现Content-Type需要为`text/plain`而非`application/xml`
2. 加密参数顺序为(msg, nonce, timestamp)，与Go SDK不同
3. 企业微信的stream消息机制：finish=false时会持续发送刷新请求
4. 实现累积模式而非消费模式，每次返回完整内容
5. 解决agent-sdk-go的final call导致的重复内容问题

### 核心技术组件

- **SessionMCPManager**: 会话级MCP连接管理器（关键技术突破）
- **流式传输**: 基于 agent-sdk-go 的实时响应
- **工具集成**: 动态发现和调用外部MCP工具
- **连接健康检查**: 自动检测和重建失效连接

## 运行命令

### Ollama版本 (免费本地部署)
```bash
# 进入Ollama版本目录
cd examples/streaming-mcp-chat

# 直接运行 (无需额外配置)
go run main.go
```

### Claude版本 (云端API)
```bash
# 设置必需的API密钥
export ANTHROPIC_API_KEY="your_anthropic_api_key"

# 可选：选择特定模型
export CLAUDE_MODEL="claude-sonnet-4-20250514"

# 进入Claude版本目录
cd examples/streaming-mcp-chat-claude

# 运行应用
go run main.go
```

### 交互命令
```bash
# 普通对话：直接输入问题
你: 查询当前时间

# 查看工具：查看可用的MCP工具
你: tools

# 退出程序
你: exit
```

## 环境变量配置

### Claude版本必需配置
```bash
# Anthropic API 密钥 (必需)
export ANTHROPIC_API_KEY="your_anthropic_api_key"

# Claude 模型选择 (可选，默认使用Claude Sonnet 4)
export CLAUDE_MODEL="claude-sonnet-4-20250514"
```

### Ollama版本配置
```bash
# 无需特殊环境变量配置
# 默认连接到 http://10.20.88.156:11434/v1
# 使用 qwen3:32b 模型
```

## 关键技术突破

项目在MCP客户端集成方面实现了重要技术突破：

### 1. SessionMCPManager 会话级连接管理
**位置**: `examples/streaming-mcp-chat/main.go` (406-679行)

**核心特性**:
- **连接复用**: 2分钟内的工具调用复用同一连接
- **健康检查**: 3秒超时的连接可用性验证
- **自动重建**: 检测到连接失效时自动重建
- **无缓存设计**: 移除工具调用缓存，确保时间工具返回实时结果

**解决的技术问题**:
- SSE连接超时导致的"connection closed"错误
- Agent框架重复调用的竞态条件问题
- MCP连接的生命周期管理

### 2. 多LLM支持架构
- **Ollama集成**: 通过OpenAI兼容接口连接本地模型
- **Claude集成**: 使用agent-sdk-go原生anthropic包
- **统一接口**: 两个版本共享相同的MCP集成逻辑

### 3. 流式对话实现
- **实时响应**: 基于agent.RunStream()的流式传输
- **思维链展示**: Claude版本支持`<think>`推理过程显示
- **错误处理**: 优雅的流式传输错误回退机制

## 关键文件说明

### 核心实现文件
- **`examples/streaming-mcp-chat/main.go`**: Ollama版本完整实现 (679行)
  - SessionMCPManager 连接管理器实现
  - 流式对话和MCP工具集成
  - 连接健康检查和自动重建逻辑

- **`examples/streaming-mcp-chat-claude/main.go`**: Claude版本实现
  - 基于Anthropic API的Claude集成
  - 支持思维链和更强推理能力
  - 完全兼容SessionMCPManager架构

### 文档文件
- **`examples/streaming-mcp-chat-claude/README.md`**: Claude版本详细使用说明
- **`项目架构设计文档.md`**: 原始架构设计文档
- **`CLAUDE.md`**: 项目记忆文档 (本文件)

## MCP 客户端集成

本项目作为 MCP 客户端，连接外部 MCP 服务器获取工具能力：

### 当前连接的外部服务
- **MCP服务器地址**: `http://sn.7soft.cn/sse`
- **传输方式**: HTTP/SSE
- **工具数量**: 6个 (currentTime、getBrandList等)

### 集成特点
- **动态发现**: 自动获取外部服务器提供的工具
- **Schema转换**: 将外部工具Schema转换为agent-sdk-go兼容格式
- **智能调用**: AI根据用户问题自动选择合适工具
- **参数验证**: 严格按照工具要求的参数格式调用

## 重要注意事项

### 技术限制
1. **模型要求**: Ollama版本需要支持Function Calling的模型(qwen3:32b)
2. **网络依赖**: 需要能访问外部MCP服务器 (`http://sn.7soft.cn/sse`)
3. **API密钥**: Claude版本需要有效的Anthropic API密钥
4. **连接稳定性**: MCP连接可能因网络问题中断，程序具备自动重建能力

### 开发注意点
1. **SessionMCPManager**: 这是核心技术组件，包含复杂的连接管理逻辑
2. **无缓存设计**: 工具调用不进行缓存，确保获取实时数据
3. **错误处理**: 实现了完整的连接失效检测和重建机制
4. **参数格式**: 外部工具有严格的参数要求(如时区使用CST而非Asia/Shanghai)

### 版本选择建议
- **入门学习**: 从 simple-chat 和 streaming-chat 开始
- **本地开发**: 使用Ollama版本，无需API费用
- **生产部署**: 使用Claude或千问版本，获得更强的推理能力
- **API服务**: 使用HTTP API版本，方便集成
- **企业应用**: 使用企业微信版本，实现IM集成

## 最新更新 (2025-09-17)

### 企业微信版本重大更新
1. **会话记忆问题修复**: 
   - 从使用streamID改为使用conversationID作为记忆标识
   - 实现ConversationAgentManager管理会话级Agent实例
   - 每个用户/群组拥有独立的对话历史

2. **日志优化**:
   - 移除所有调试日志输出
   - 仅保留必要的业务日志
   - 提升生产环境运行稳定性

3. **技术架构优化**:
   - 彻底解决agent-sdk-go的final call重复调用问题
   - 实现完整的会话生命周期管理
   - 优化内存使用和性能表现

## 重要指导原则

### 代码开发原则
- **精简直接**: 代码应当简洁明了，避免过度设计
- **注释最少化**: 除非用户明确要求，不要添加任何注释
- **遵循惯例**: 严格遵循现有代码风格和项目约定
- **安全第一**: 不暴露密钥，不记录敏感信息

### 日志处理原则  
- **生产环境**: 只保留必要的业务日志
- **调试信息**: 开发阶段使用，生产环境必须移除
- **格式统一**: 使用项目约定的日志格式

### 任务管理原则
- **使用TodoWrite**: 对于复杂任务必须使用任务管理工具
- **及时更新**: 完成任务立即标记为completed
- **合理拆分**: 大任务拆分为可执行的小任务

### 测试验证原则
- **编译优先**: 每次修改后首先确保编译通过
- **功能验证**: 关键功能修改必须进行实际测试
- **错误处理**: 所有可能的错误都要有处理机制

## important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.