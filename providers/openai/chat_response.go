package openai

import "encoding/json"

// chatCompletionResponse 仅用于 OpenAI 适配器的非流式 Chat 请求。
// 不修改共享响应类型，避免影响嵌入该类型的其他 provider。
type chatCompletionResponse struct {
	OpenAIProviderChatResponse
	unwrappedBody json.RawMessage
}

func (r *chatCompletionResponse) UnmarshalJSON(body []byte) error {
	r.unwrappedBody = nil
	if err := json.Unmarshal(body, &r.OpenAIProviderChatResponse); err != nil {
		return err
	}

	var envelope struct {
		Choices json.RawMessage `json:"choices"`
		Error   json.RawMessage `json:"error"`
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	// 标准响应优先，包括显式 choices:null / error:null。
	// 不符合包装格式的附加字段继续按原有逻辑忽略。
	if err := json.Unmarshal(body, &envelope); err != nil ||
		envelope.Choices != nil || envelope.Error != nil || !envelope.Success {
		return nil
	}
	var shape struct {
		Object  string            `json:"object"`
		Choices []json.RawMessage `json:"choices"`
	}
	if err := json.Unmarshal(envelope.Data, &shape); err != nil ||
		shape.Object != "chat.completion" || len(shape.Choices) == 0 {
		return nil
	}

	// 只解包一层，并保留内层原始字节供响应透传使用。
	var response OpenAIProviderChatResponse
	if err := json.Unmarshal(envelope.Data, &response); err != nil {
		return err
	}
	r.OpenAIProviderChatResponse = response
	r.unwrappedBody = envelope.Data
	return nil
}
