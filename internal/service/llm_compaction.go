package service

import (
	"encoding/json"
	"sort"
	"strings"
)

type LLMCompactionConfig struct {
	Enabled         bool
	ThresholdBytes  int
	MaxStringBytes  int
	MaxArrayItems   int
	MaxObjectFields int
}

type CompactionResult struct {
	CompactedPayload  string
	WasCompacted      bool
	OriginalBytes     int
	CompactedBytes    int
	DroppedFields     int
	TruncatedStrings  int
	TruncatedArrays   int
	TopLevelKeysCount int
}

type compactionStats struct {
	droppedFields    int
	truncatedStrings int
	truncatedArrays  int
}

func compactPayloadForLLM(payload string, cfg LLMCompactionConfig) CompactionResult {
	originalBytes := len(payload)
	result := CompactionResult{
		CompactedPayload: payload,
		OriginalBytes:    originalBytes,
		CompactedBytes:   originalBytes,
	}
	if !cfg.Enabled || originalBytes < cfg.ThresholdBytes {
		return result
	}

	var root interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return result
	}

	stats := &compactionStats{}
	compacted := compactValue("$", "", root, cfg, stats)
	wrapped := map[string]interface{}{
		"_compaction": map[string]interface{}{
			"original_bytes":     originalBytes,
			"top_level_keys":     topLevelKeys(root),
			"dropped_fields":     stats.droppedFields,
			"truncated_strings":  stats.truncatedStrings,
			"truncated_arrays":   stats.truncatedArrays,
			"strategy":           "generic_structural",
			"compaction_enabled": true,
		},
		"payload": compacted,
	}
	b, err := json.Marshal(wrapped)
	if err != nil {
		return result
	}
	if len(b) >= originalBytes {
		return result
	}
	result.CompactedPayload = string(b)
	result.WasCompacted = true
	result.CompactedBytes = len(b)
	result.DroppedFields = stats.droppedFields
	result.TruncatedStrings = stats.truncatedStrings
	result.TruncatedArrays = stats.truncatedArrays
	result.TopLevelKeysCount = len(topLevelKeys(root))
	return result
}

func compactValue(path, key string, value interface{}, cfg LLMCompactionConfig, stats *compactionStats) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		type field struct {
			key   string
			score int
			value interface{}
		}
		fields := make([]field, 0, len(typed))
		for k, v := range typed {
			fields = append(fields, field{key: k, value: v, score: fieldScore(k, v)})
		}
		sort.SliceStable(fields, func(i, j int) bool {
			if fields[i].score == fields[j].score {
				return fields[i].key < fields[j].key
			}
			return fields[i].score > fields[j].score
		})
		limit := cfg.MaxObjectFields
		if limit <= 0 {
			limit = len(fields)
		}
		out := map[string]interface{}{}
		for i, item := range fields {
			if i >= limit && item.score <= 0 {
				stats.droppedFields++
				continue
			}
			childPath := path + "." + sanitizeKey(item.key)
			out[item.key] = compactValue(childPath, item.key, item.value, cfg, stats)
		}
		return out
	case []interface{}:
		limit := cfg.MaxArrayItems
		if limit <= 0 || limit > len(typed) {
			limit = len(typed)
		}
		out := make([]interface{}, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, compactValue(path, key, typed[i], cfg, stats))
		}
		if len(typed) > limit {
			stats.truncatedArrays++
			out = append(out, map[string]interface{}{
				"_truncated_items": len(typed) - limit,
			})
		}
		return out
	case string:
		return compactString(key, typed, cfg, stats)
	default:
		return value
	}
}

func compactString(key, value string, cfg LLMCompactionConfig, stats *compactionStats) string {
	trimmed := strings.TrimSpace(value)
	limit := cfg.MaxStringBytes
	if limit <= 0 {
		return trimmed
	}
	if isLikelyNoisyURLField(key, trimmed) {
		stats.truncatedStrings++
		return trimmed[:min(limit/2, len(trimmed))]
	}
	if len(trimmed) <= limit {
		return trimmed
	}
	stats.truncatedStrings++
	return trimmed[:limit]
}

func topLevelKeys(root interface{}) []string {
	object, ok := root.(map[string]interface{})
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fieldScore(key string, value interface{}) int {
	normalized := strings.ToLower(strings.TrimSpace(key))
	score := 0
	for _, hint := range []string{"id", "name", "title", "subject", "status", "state", "action", "event", "type", "kind", "reason", "conclusion", "branch", "repository", "workflow", "url", "message", "text", "summary"} {
		if normalized == hint || strings.Contains(normalized, hint) {
			score += 5
		}
	}
	switch typed := value.(type) {
	case string:
		if len(typed) <= 200 {
			score += 3
		}
		if strings.Contains(strings.ToLower(typed), "http") {
			score--
		}
	case bool, float64, int, int64:
		score += 2
	case map[string]interface{}:
		score += 1
	case []interface{}:
		if len(typed) > 6 {
			score--
		}
	}
	if strings.Contains(normalized, "avatar") || strings.Contains(normalized, "gravatar") || strings.Contains(normalized, "blob") || strings.Contains(normalized, "assignee") || strings.Contains(normalized, "subscriber") || strings.Contains(normalized, "subscription") || strings.Contains(normalized, "owner") {
		score -= 3
	}
	return score
}

func isLikelyNoisyURLField(key, value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if strings.Contains(value, "://") {
		return true
	}
	return strings.Contains(normalized, "url")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
