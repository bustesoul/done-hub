package types

// Function tool choices use a nested function.name in Chat and a flat name in
// Responses. Leave strings and other tool types alone, and never mutate input.
func chatToolChoiceToResponses(choice any) any {
	object, ok := choice.(map[string]any)
	if !ok || object["type"] != "function" {
		return choice
	}
	function, ok := object["function"].(map[string]any)
	if !ok {
		return choice
	}
	name, ok := function["name"].(string)
	if !ok || name == "" {
		return choice
	}
	converted := make(map[string]any, len(object))
	for key, value := range object {
		if key != "function" {
			converted[key] = value
		}
	}
	converted["name"] = name
	return converted
}

func responsesToolChoiceToChat(choice any) any {
	object, ok := choice.(map[string]any)
	if !ok || object["type"] != "function" {
		return choice
	}
	name, ok := object["name"].(string)
	if !ok || name == "" {
		return choice
	}
	converted := make(map[string]any, len(object))
	for key, value := range object {
		if key != "name" {
			converted[key] = value
		}
	}
	converted["function"] = map[string]any{"name": name}
	return converted
}
