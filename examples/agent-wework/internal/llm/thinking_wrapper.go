package llm

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ThinkingLLMWrapper 包装LLM客户端以启用thinking mode
type ThinkingLLMWrapper struct {
	wrapped interfaces.LLM
	model   string
}

// NewThinkingLLMWrapper 创建一个启用thinking mode的LLM包装器
func NewThinkingLLMWrapper(wrapped interfaces.LLM, model string) *ThinkingLLMWrapper {
	return &ThinkingLLMWrapper{
		wrapped: wrapped,
		model:   model,
	}
}

// Generate implements interfaces.LLM.Generate
func (w *ThinkingLLMWrapper) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// 暂时不支持非流式thinking，直接调用原方法
	return w.wrapped.Generate(ctx, prompt, options...)
}

// GenerateStream implements interfaces.StreamingLLM.GenerateStream
func (w *ThinkingLLMWrapper) GenerateStream(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// 注意：thinking mode在当前SDK版本中主要通过模型自动启用
	// 模型会自动在回复中包含<think>标签的思考内容
	return w.wrapped.(interfaces.StreamingLLM).GenerateStream(ctx, prompt, options...)
}

// GenerateWithTools implements interfaces.LLM.GenerateWithTools
func (w *ThinkingLLMWrapper) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	// 暂时不支持非流式thinking，直接调用原方法
	return w.wrapped.GenerateWithTools(ctx, prompt, tools, options...)
}

// GenerateWithToolsStream implements interfaces.StreamingLLM.GenerateWithToolsStream
func (w *ThinkingLLMWrapper) GenerateWithToolsStream(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (<-chan interfaces.StreamEvent, error) {
	// 注意：thinking mode在当前SDK版本中主要通过模型自动启用
	// 模型会自动在回复中包含<think>标签的思考内容
	return w.wrapped.(interfaces.StreamingLLM).GenerateWithToolsStream(ctx, prompt, tools, options...)
}

// Name implements interfaces.LLM.Name
func (w *ThinkingLLMWrapper) Name() string {
	return w.wrapped.Name()
}

// SupportsStreaming implements interfaces.StreamingLLM.SupportsStreaming
func (w *ThinkingLLMWrapper) SupportsStreaming() bool {
	if streaming, ok := w.wrapped.(interfaces.StreamingLLM); ok {
		return streaming.SupportsStreaming()
	}
	return false
}

// SupportsToolUse implements interfaces.ToolAwareLLM.SupportsToolUse
func (w *ThinkingLLMWrapper) SupportsToolUse() bool {
	// 检查是否实现了GenerateWithTools方法
	_, hasMethod := w.wrapped.(interface {
		GenerateWithTools(context.Context, string, []interfaces.Tool, ...interfaces.GenerateOption) (string, error)
	})
	return hasMethod
}
