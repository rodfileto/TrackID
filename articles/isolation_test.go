package articles

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNothingForensicReadsArticles keeps analysis out of forensic output
// (docs/person-article-plan.md §7): no code in the reports, the matcher, the clusters, the
// examination or the person views may name the article tables or import this package. An
// article is the analysts' assessment; a laudo or a cluster must never rest on it.
func TestNothingForensicReadsArticles(t *testing.T) {
	forbidden := regexp.MustCompile(`\barticle_(revisions|revision_bodies|mentions)\b|\barticles\b\s*\(|FROM\s+articles\b|JOIN\s+articles\b|trackid/articles"`)
	for _, dir := range []string{"reports", "biometricmatch", "cluster", "examination", "analyses", "person", "facecompare", "fingerprint"} {
		files, err := filepath.Glob(filepath.Join("..", dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if loc := forbidden.FindIndex(src); loc != nil {
				t.Errorf("%s reads articles: %q", file, src[loc[0]:loc[1]])
			}
		}
	}

	// The queries those packages run are in db/queries/*.sql: none of the article queries may be
	// called from them either.
	files, err := filepath.Glob(filepath.Join("..", "db", "queries", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no query files: %v", err)
	}
	var queries []byte
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		queries = append(append(queries, b...), '\n')
	}
	articleQueries := regexp.MustCompile(`(?m)^-- name: (\w+) .*\n(?:--.*\n)*[^-].*(?:\n[^\n].*)*`)
	var names []string
	for _, m := range articleQueries.FindAllStringSubmatch(string(queries), -1) {
		if regexp.MustCompile(`\barticle`).MatchString(m[0]) {
			names = append(names, m[1])
		}
	}
	if len(names) == 0 {
		t.Fatal("found no article queries in db/queries")
	}
	call := regexp.MustCompile(`\.(` + strings.Join(names, "|") + `)\(`)
	for _, dir := range []string{"reports", "biometricmatch", "cluster", "examination", "analyses", "person"} {
		files, _ := filepath.Glob(filepath.Join("..", dir, "*.go"))
		for _, file := range files {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if loc := call.FindIndex(src); loc != nil {
				t.Errorf("%s calls an article query: %q", file, src[loc[0]:loc[1]])
			}
		}
	}
}
