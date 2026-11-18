package mcp

// tagFilter is a {key: value} Tag filter: every pair must match exactly.
func tagFilter(desc string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": desc}
}

func flag(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }
