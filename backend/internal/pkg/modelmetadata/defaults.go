package modelmetadata

import (
	_ "embed"
	"encoding/json"
)

//go:embed model_prices_and_context_window.json
var defaultModelMetadataJSON []byte

type LiteLLMRawEntry struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
	MaxInputTokens              *int     `json:"max_input_tokens"`
	MaxOutputTokens             *int     `json:"max_output_tokens"`
	MaxTokens                   *int     `json:"max_tokens"`
}

type LiteLLMModelMetadata struct {
	InputCostPerToken           float64
	OutputCostPerToken          float64
	CacheReadInputTokenCost     float64
	CacheCreationInputTokenCost float64
	MaxInputTokens              int
	MaxOutputTokens             int
	MaxTokens                   int
}

func GetDefaultModelMetadata(modelName string) *LiteLLMModelMetadata {
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(defaultModelMetadataJSON, &rawData); err != nil {
		return nil
	}
	entry, ok := rawData[modelName]
	if !ok {
		return nil
	}
	var raw LiteLLMRawEntry
	if err := json.Unmarshal(entry, &raw); err != nil {
		return nil
	}
	meta := &LiteLLMModelMetadata{}
	if raw.InputCostPerToken != nil {
		meta.InputCostPerToken = *raw.InputCostPerToken
	}
	if raw.OutputCostPerToken != nil {
		meta.OutputCostPerToken = *raw.OutputCostPerToken
	}
	if raw.CacheReadInputTokenCost != nil {
		meta.CacheReadInputTokenCost = *raw.CacheReadInputTokenCost
	}
	if raw.CacheCreationInputTokenCost != nil {
		meta.CacheCreationInputTokenCost = *raw.CacheCreationInputTokenCost
	}
	if raw.MaxInputTokens != nil {
		meta.MaxInputTokens = *raw.MaxInputTokens
	}
	if raw.MaxOutputTokens != nil {
		meta.MaxOutputTokens = *raw.MaxOutputTokens
	}
	if raw.MaxTokens != nil {
		meta.MaxTokens = *raw.MaxTokens
	}
	if meta.InputCostPerToken == 0 && meta.OutputCostPerToken == 0 && meta.CacheReadInputTokenCost == 0 && meta.CacheCreationInputTokenCost == 0 && meta.MaxInputTokens == 0 && meta.MaxOutputTokens == 0 && meta.MaxTokens == 0 {
		return nil
	}
	return meta
}
