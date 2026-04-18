package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type PayloadFingerprint struct {
	KeyPaths []string               `json:"key_paths"`
	Shapes   map[string]string      `json:"shapes"`
	Summary  map[string]interface{} `json:"summary"`
}

func BuildFingerprint(payload string) (PayloadFingerprint, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		return PayloadFingerprint{}, err
	}
	shapes := map[string]string{}
	keys := []string{}
	walkValue("$", v, &keys, shapes)
	sort.Strings(keys)
	summary := map[string]interface{}{
		"key_count": len(keys),
		"keys":      keys,
	}
	return PayloadFingerprint{KeyPaths: keys, Shapes: shapes, Summary: summary}, nil
}

func walkValue(path string, v interface{}, keys *[]string, shapes map[string]string) {
	switch t := v.(type) {
	case map[string]interface{}:
		shapes[path] = "object"
		for k, child := range t {
			childPath := path + "." + sanitizeKey(k)
			*keys = append(*keys, childPath)
			walkValue(childPath, child, keys, shapes)
		}
	case []interface{}:
		shapes[path] = "array"
		for i, child := range t {
			idx := fmt.Sprintf("%s[%d]", path, i)
			walkValue(idx, child, keys, shapes)
		}
	case string:
		shapes[path] = "string"
	case float64:
		shapes[path] = "number"
	case bool:
		shapes[path] = "bool"
	case nil:
		shapes[path] = "null"
	default:
		shapes[path] = "unknown"
	}
}

func sanitizeKey(k string) string {
	if k == "" {
		return "_"
	}
	repl := strings.NewReplacer(" ", "_", "-", "_", "/", "_", ".", "_")
	return repl.Replace(k)
}

func parseJSONStringArray(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	return nil
}

func parseJSONStringMap(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	return nil
}

func mapHasPath(set map[string]struct{}, path string) bool {
	_, ok := set[path]
	return ok
}

func asStringMap(headers map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range headers {
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}

func shapeMatch(expected, got string) bool {
	if expected == "" {
		return true
	}
	if expected == got {
		return true
	}
	// loose numeric compatibility
	if expected == "int" && got == "number" {
		return true
	}
	if expected == "float" && got == "number" {
		return true
	}
	return false
}

func toFloat64(v interface{}, fallback float64) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err == nil {
			return f
		}
	}
	return fallback
}
