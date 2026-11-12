package assist

import (
	"regexp"
	"slices"
	"testing"

	"github.com/rodfileto/trackid/db/migrations"
)

// TestDefaultOccurrenceTypesAreTheTable keeps the offline copy in step with what the migrations insert: a type
// missing from it is one a model drafting with it can never choose (UNDETERMINED was, until DAMAGE came).
func TestDefaultOccurrenceTypesAreTheTable(t *testing.T) {
	inserted := regexp.MustCompile(`\('([A-Z_]+)', '(?:CRIMINAL|CIVIL)', '[A-Z_]+'\)`)
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var table []string
	for _, entry := range entries {
		data, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range inserted.FindAllStringSubmatch(string(data), -1) {
			table = append(table, m[1])
		}
	}
	got := slices.Clone(DefaultOccurrenceTypes)
	slices.Sort(got)
	slices.Sort(table)
	if !slices.Equal(got, slices.Compact(table)) {
		t.Errorf("DefaultOccurrenceTypes %v\nare not the table's %v", got, table)
	}
}
