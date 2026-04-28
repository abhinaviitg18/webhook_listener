package store

import (
	"strings"
	"testing"
)

func TestBuildEventSelectQueryCanExcludeTagsWithoutDroppingProcessedText(t *testing.T) {
	query := buildEventSelectQuery(`WHERE account_id=? AND id=?`, true, true, false)
	if !strings.Contains(query, `COALESCE(processed_text,'')`) {
		t.Fatalf("expected processed_text selection in query: %s", query)
	}
	if strings.Contains(query, `COALESCE(tags_json,'')`) {
		t.Fatalf("did not expect tags_json selection in query: %s", query)
	}
}

func TestEventSelectVariantsIncludeProcessedTextWithoutTags(t *testing.T) {
	found := false
	for _, variant := range eventSelectVariants() {
		if variant.includeProcessed && !variant.includeTags {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected a fallback variant that keeps processed_text when tags_json is unavailable")
	}
}
