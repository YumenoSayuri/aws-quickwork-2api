package proxy

import (
	"strings"
)

// schema annotation keys that only cost bytes; the model does not need them
var leanDropKeys = map[string]bool{
	"description":           true,
	"$schema":               true,
	"$id":                   true,
	"title":                 true,
	"examples":              true,
	"example":               true,
	"default":               true,
	"additionalProperties":  true,
	"deprecated":            true,
	"readOnly":              true,
	"writeOnly":             true,
	"$comment":              true,
	"definitions":           true,
	"$defs":                 true,
	"contentMediaType":      true,
	"contentEncoding":       true,
	"patternProperties":     true,
	"propertyNames":         true,
	"unevaluatedProperties": true,
}

// leanToolDefinitions strips schema verbosity while keeping every fact the
// model needs to emit a valid call: tool name, parameter names, types,
// required lists and enum values.
func leanToolDefinitions(defs []toolDefinition) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		item := map[string]any{"name": d.Name}
		if d.Parameters != nil {
			item["parameters"] = leanSchema(d.Parameters, 0)
		}
		out = append(out, item)
	}
	return out
}

func leanSchema(node any, depth int) any {
	if depth > 12 {
		return nil
	}
	switch v := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			if leanDropKeys[k] {
				continue
			}
			if k == "description" {
				continue
			}
			out[k] = leanSchema(val, depth+1)
		}
		// Collapse the long tail of anyOf/oneOf branches that only differ by
		// description: keep the first branch when all are object types.
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, leanSchema(item, depth+1))
		}
		return out
	case string:
		return strings.TrimSpace(v)
	default:
		return v
	}
}
