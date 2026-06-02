package config

import (
	"encoding/json"
	"strings"
)

var BuiltinNeed2ResponseModels = []string{
	"mimo-v2.5-pro",
	"mimo-v2-pro",
	"mimo-v2.5",
	"mimo-v2-omni",
	"mimo-v2-flash",
}

func IsBuiltinNeed2ResponseModel(model string) bool {
	modelName := strings.TrimSpace(model)
	if modelName == "" {
		return false
	}

	for _, builtinModel := range BuiltinNeed2ResponseModels {
		if modelName == builtinModel {
			return true
		}
	}
	return false
}

func ShouldSendChatAsResponses(compatibleResponse bool, providerSupportsResponses bool, model string) bool {
	if compatibleResponse || !providerSupportsResponses {
		return !IsBuiltinNeed2ResponseModel(model)
	}
	return true
}

func BuildNeed2ResponseModelSet(models []string) map[string]struct{} {
	modelSet := make(map[string]struct{}, len(models)+len(BuiltinNeed2ResponseModels))
	for _, model := range BuiltinNeed2ResponseModels {
		modelSet[model] = struct{}{}
	}

	for _, model := range models {
		modelName := strings.TrimSpace(model)
		if modelName == "" {
			continue
		}
		modelSet[modelName] = struct{}{}
	}

	return modelSet
}

func ParseNeed2ResponseModels(data string) []string {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return []string{}
	}

	if strings.HasPrefix(trimmed, "[") {
		var modelList []string
		if err := json.Unmarshal([]byte(trimmed), &modelList); err == nil {
			return modelList
		}
	}

	normalized := strings.ReplaceAll(trimmed, "\r\n", "\n")
	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == '\n' || r == ','
	})

	return parts
}
