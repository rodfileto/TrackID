package assist

import (
	"strings"
	"testing"
)

func TestStageFile(t *testing.T) {
	data := []byte("conteúdo do ofício")
	key, f, err := stageFile(data, "oficio.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, draftObjectPrefix+"/") {
		t.Errorf("key not under draft prefix: %q", key)
	}
	if f.SHA256 == "" || f.Name != "oficio.pdf" || f.ContentType == "" || f.StorageRef != "" {
		t.Errorf("stageFile fields: %+v", f)
	}

	// Deterministic: same bytes, same key and hash.
	key2, f2, _ := stageFile(data, "renamed.pdf")
	if key2 != key || f2.SHA256 != f.SHA256 {
		t.Errorf("stageFile not content-addressed: %q vs %q", key, key2)
	}
}

func TestStageFileEmpty(t *testing.T) {
	if _, _, err := stageFile(nil, "x"); err == nil {
		t.Error("empty file accepted")
	}
}

func TestResultKey(t *testing.T) {
	a := StagedFile{SHA256: "aaa"}
	b := StagedFile{SHA256: "bbb"}
	k1 := ResultKey([]StagedFile{a, b})
	k2 := ResultKey([]StagedFile{b, a})
	if k1 == "" || k1 != k2 {
		t.Errorf("ResultKey not order-independent: %q vs %q", k1, k2)
	}
}
