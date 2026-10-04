package video

import "testing"

func TestDefaultSystemMentionsCoreRules(t *testing.T) {
	system := DefaultSystem()
	for _, phrase := range []string{"shot IDs", "locked", "asset URI", "quality checks"} {
		if !contains(system, phrase) {
			t.Fatalf("system prompt missing %q", phrase)
		}
	}
}

func contains(text, phrase string) bool {
	for i := 0; i+len(phrase) <= len(text); i++ {
		if text[i:i+len(phrase)] == phrase {
			return true
		}
	}
	return false
}
