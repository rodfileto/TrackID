package assist

import "testing"

func TestMeanConfidence(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"1\t1\t0\t0\t0\t0\t0\t0\t100\t100\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t0\t0\t10\t10\t90\tDelegado\n" +
		"5\t1\t1\t1\t1\t2\t0\t0\t10\t10\t30\t|\n"
	got := meanConfidence([]byte(tsv))
	want := (90.0*8 + 30) / 9
	if got < want-0.01 || got > want+0.01 {
		t.Errorf("meanConfidence = %.2f, want %.2f", got, want)
	}
	if meanConfidence([]byte("level\n")) != 0 {
		t.Error("no words should be 0")
	}
}

func TestTidyOCR(t *testing.T) {
	if got := tidyOCR("\n  a  \n\n\n b\n\n"); got != "a\n\nb" {
		t.Errorf("tidyOCR = %q", got)
	}
}
