package articles

import (
	"os"
	"regexp"
	"testing"
)

// TestTemplateWording checks both languages define every key the draft generator uses: a
// missing key would print as empty text, which the schema refuses, and the draft would fail.
func TestTemplateWording(t *testing.T) {
	src, err := os.ReadFile("template.go")
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`d\.t\("([^"]+)"\)`).FindAllStringSubmatch(string(src), -1) {
		used[m[1]] = true
	}
	for _, tier := range tierOrder {
		used["tier."+tier] = true
	}
	used["tierShort."+tierConfirmed], used["tierShort."+tierUnreviewed] = true, true
	for lang, text := range templateText {
		for key := range used {
			if text[key] == "" {
				t.Errorf("%s: no wording for %q", lang, key)
			}
		}
		for key := range text {
			if !used[key] {
				t.Errorf("%s: %q is never used", lang, key)
			}
		}
	}
}
