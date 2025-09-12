# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

AI-Body 是一个基于 Go 语言的企业微信智能机器人项目，采用 agent-sdk-go 框架作为核心智能体引擎。项目目前处于架构设计和配置阶段，主要实现代码尚未开发。

**重要提醒**: 当前 main.go 仅包含优雅关闭的占位符代码，核心功能尚未实现。

## 核心架构

项目采用三层架构设计：

1. **企业微信回调层**: HTTP 接口接收企业微信消息推送
2. **Agent-SDK-GO 核心层**: 智能体引擎，集成 LLM 适配器和 MCP 客户端
3. **外部服务层**: 多 LLM 提供商支持 + MCP 服务器工具集成

### 关键设计特点

- **三步式工作流**: 决策 → 工具调用(可选) → 响应生成
- **MCP 客户端**: 通过 MCP 协议集成外部工具和资源
- **多 LLM 统一接口**: 支持 Qwen、DeepSeek、Ollama
- **配置驱动**: 所有组件通过 YAML 配置文件管理

## 开发命令

### 基础命令
```bash
# 初始化依赖 (需要首先实现 go.mod 中的依赖)
go mod tidy

# 运行应用 (当前仅为占位符)
go run main.go

# 构建应用
go build -o ai-body main.go

# 运行测试 (测试文件尚未创建)
go test ./...
```

### 配置管理
```bash
# 验证配置文件格式
# config/app.yaml - 主应用配置
# config/llm.yaml - LLM 提供商配置  
# config/mcp.json - MCP 服务器配置
# config/workflow.yaml - 工作流配置
```

## 环境变量配置

关键环境变量 (参考 config/app.yaml 和 config/llm.yaml):

```bash
# 企业微信配置
export WECOM_CORP_ID="your_corp_id"
export WECOM_AGENT_ID="your_agent_id"
export WECOM_BOT_TOKEN="your_bot_token"
export WECOM_BOT_ENCODING_AES_KEY="your_aes_key"

# LLM API 密钥
export QWEN_API_KEY="your_qwen_key"
export DEEPSEEK_API_KEY="your_deepseek_key"
```

## 实现优先级

基于架构文档，建议的开发顺序：

1. **配置管理模块** (`config/`)
   - 实现配置文件加载和验证
   - 环境变量注入机制

2. **企业微信集成** (`wecom/`)  
   - HTTP 服务器和回调处理
   - XML 消息解析和签名验证

3. **LLM 适配器** (`llm/`)
   - 统一 LLM 接口和工厂模式
   - Qwen/DeepSeek/Ollama 适配器实现

4. **MCP 客户端** (`mcp/`)
   - MCP 协议实现和工具注册
   - 外部服务器连接管理

5. **简单工作流引擎** (`workflow/`)
   - 三步执行流程
   - 智能决策机制

6. **Agent-SDK-GO 集成**
   - 适配器实现和会话管理

## 关键文件说明

- `项目架构设计文档.md`: 完整的中文架构设计文档，包含所有技术细节
- `config/*.yaml`: 配置文件模板，使用环境变量占位符
- `main.go`: 当前仅为占位符，需要实现完整的应用启动流程

## 企业微信集成要点

- 回调 URL: `/wecom/callback` (GET 用于验证，POST 接收消息)
- 消息格式: XML (需要解析企业微信标准格式)  
- 安全验证: 支持签名验证和 AES 消息加解密
- 会话管理: 基于企业微信用户 ID 的会话隔离

## MCP 协议集成

项目作为 MCP 客户端连接外部 MCP 服务器:
- 支持 stdio 和 SSE 传输方式
- 自动发现外部服务器的工具、资源、提示词
- 运行时动态注册和调用外部能力
- 遵循 MCP 2024-11-05 协议规范

## 注意事项

1. **当前状态**: 项目处于设计阶段，核心代码尚未实现
2. **依赖管理**: go.mod 文件存在但 go.sum 为空，需要添加实际依赖
3. **测试框架**: 尚未建立测试架构和测试文件
4. **文档语言**: 架构文档为中文，代码注释建议使用中文
5. **配置安全**: 敏感信息通过环境变量管理，避免硬编码