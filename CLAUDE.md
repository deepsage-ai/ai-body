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

## 开发需求记录

### 企业微信智能机器人版本需求
**要求严格性**: 必须基于准确的官方文档进行开发，不允许推测或假设API格式
**参考文档**: 
- 接收消息文档: https://developer.work.weixin.qq.com/document/path/100719
- 回复消息文档: https://developer.work.weixin.qq.com/document/path/101031
**设计要求**: 
1. 必须先获取准确的企业微信API文档内容
2. 基于真实API格式设计消息处理流程
3. 严格按照官方文档实现消息验证和回复机制
4. 完全复用现有千问版本的SessionMCP管理和智能体逻辑
5. 实现严谨的错误处理和企业级安全要求
6. **配置方式**: 使用常量配置而非环境变量，简化demo部署
**开发原则**: 准确性优于快速实现，必须确保API调用格式的正确性

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
- **本地开发**: 使用Ollama版本，无需API费用
- **生产部署**: 使用Claude版本，获得更强的推理能力
- **功能测试**: 两个版本的MCP集成功能完全一致