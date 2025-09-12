# 简单对话示例

这是一个最基础的 AI-Body 对话示例，展示了如何使用 agent-sdk-go 框架创建一个简单的聊天机器人。

## 运行方式

```bash
cd examples/simple-chat
go run main.go
```

## 功能特点

- 连接到 Ollama 的 qwen3:32b 模型
- 支持多轮对话和上下文记忆
- 完整的多租户上下文设置
- 简单的命令行交互界面

## 配置说明

- **Ollama 服务地址**: `http://10.20.88.156:11434/v1`
- **模型**: qwen3:32b
- **组织ID**: ai-body-demo
- **对话ID**: demo-conversation-001

如需修改配置，请编辑 `main.go` 文件中的相应参数。