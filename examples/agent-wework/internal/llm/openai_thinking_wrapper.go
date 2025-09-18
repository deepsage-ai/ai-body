package llm

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// OpenAIThinkingWrapper 包装OpenAI兼容的LLM客户端以启用reasoning mode
type OpenAIThinkingWrapper struct {
	wrapped interfaces.LLM
}

// NewOpenAIThinkingWrapper 创建一个启用reasoning mode的OpenAI包装器
func NewOpenAIThinkingWrapper(wrapped interfaces.LLM) *OpenAIThinkingWrapper {
	return &OpenAIThinkingWrapper{
		wrapped: wrapped,
	}
}

// Generate implements interfaces.LLM.Generate
func (w *OpenAIThinkingWrapper) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// 添加comprehensive reasoning选项
	options = append(options, openai.WithReasoning("comprehensive"))
	return w.wrapped.Generate(ctx, prompt, options...)
}

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (w *OpenAIThinkingWrapper) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// 添加comprehensive reasoning选项，让模型展示详细推理过程
	options = append(options, openai.WithReasoning("comprehensive"))
	return w.wrapped.(interfaces.StreamingLLM).GenerateStream(ctx, prompt, options...)
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (w *OpenAIThinkingWrapper) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// 添加comprehensive reasoning选项
	options = append(options, openai.WithReasoning("comprehensive"))
	return w.wrapped.GenerateWithTools(ctx, prompt, tools, options...)
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (w *OpenAIThinkingWrapper) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// 添加comprehensive reasoning选项，让模型展示详细推理过程
	options = append(options, openai.WithReasoning("comprehensive"))
	return w.wrapped.(interfaces.StreamingLLM).GenerateWithToolsStream(ctx, prompt, tools, options...)
}

// Name implements interfaces.LLM.Name
func (w *OpenAIThinkingWrapper) Name() string {
	return w.wrapped.Name()
}

// SupportsStreaming implements interfaces.StreamingLLM.SupportsStreaming
func (w *OpenAIThinkingWrapper) SupportsStreaming() bool {
	if streaming, ok := w.wrapped.(interfaces.StreamingLLM); ok {
		return streaming.SupportsStreaming()
	}
	return false
}

// SupportsToolUse implements interfaces.ToolAwareLLM.SupportsToolUse
func (w *OpenAIThinkingWrapper) SupportsToolUse() bool {
	// 检查是否实现了GenerateWithTools方法
	_, hasMethod := w.wrapped.(interface {
		GenerateWithTools(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (string, error)
	})
	return hasMethod
}
