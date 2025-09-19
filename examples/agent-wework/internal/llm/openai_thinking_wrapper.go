package llm

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

// OpenAIThinkingWrapper 包装OpenAI兼容的LLM客户端以启用reasoning mode
type OpenAIThinkingWrapper struct {
	wrapped        interfaces.LLM
	reasoningLevel string // "comprehensive" 或 "minimal"
}

// NewOpenAIThinkingWrapper 创建一个启用reasoning mode的OpenAI包装器
func NewOpenAIThinkingWrapper(wrapped interfaces.LLM) *OpenAIThinkingWrapper {
	return &OpenAIThinkingWrapper{
		wrapped:        wrapped,
		reasoningLevel: "minimal", // 默认简洁推理，确保回复精炼
	}
}

// NewOpenAIThinkingWrapperWithLevel 创建指定推理级别的包装器
func NewOpenAIThinkingWrapperWithLevel(wrapped interfaces.LLM, level string) *OpenAIThinkingWrapper {
	return &OpenAIThinkingWrapper{
		wrapped:        wrapped,
		reasoningLevel: level,
	}
}

// Generate implements interfaces.LLM.Generate
func (w *OpenAIThinkingWrapper) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// 使用配置的推理级别
	options = append(options, openai.WithReasoning(w.reasoningLevel))
	return w.wrapped.Generate(ctx, prompt, options...)
}

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (w *OpenAIThinkingWrapper) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// 使用配置的推理级别
	options = append(options, openai.WithReasoning(w.reasoningLevel))
	return w.wrapped.(interfaces.StreamingLLM).GenerateStream(ctx, prompt, options...)
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (w *OpenAIThinkingWrapper) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// 使用配置的推理级别
	options = append(options, openai.WithReasoning(w.reasoningLevel))
	return w.wrapped.GenerateWithTools(ctx, prompt, tools, options...)
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (w *OpenAIThinkingWrapper) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// 使用配置的推理级别
	options = append(options, openai.WithReasoning(w.reasoningLevel))
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
