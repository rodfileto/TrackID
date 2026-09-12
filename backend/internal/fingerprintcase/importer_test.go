package fingerprintcase

import (
	"strings"
	"testing"
)

func TestParseFirstFiveRows(t *testing.T) {
	input := strings.NewReader("02.2005.01.SRMG.00002-001-01-01;FAFICH - UFMG INFORMAÇÃO TÉCNICA 006/05-NID/SR/MG;null\n02.2005.01.SRMG.00003-003-01-01;ESCOLA DE MEDICINA - UFMG LAUDO 085/05 - NID/SR/MG;null\n02.2005.01.SRMG.00003-003-02-01;ESCOLA DE MEDICINA - UFMG LAUDO 085/05 - NID/SR/MG;null\n02.2005.01.SRMG.00003-003-03-01;ESCOLA DE MEDICINA - UFMG LAUDO 085/05 - NID/SR/MG;csuser93;TPvsULF;550131.990331404/2004-75\n02.2005.01.SRMG.00003-002-01-01;ESCOLA DE MEDICINA - UFMG LAUDO 085/05 - NID/SR/MG;null\n")
	records, stats, err := Parse(input, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Read != 5 || len(records) != 5 {
		t.Fatalf("read=%d records=%d, want 5/5", stats.Read, len(records))
	}
	if records[0].Description != "FAFICH - UFMG INFORMAÇÃO TÉCNICA 006/05-NID/SR/MG" {
		t.Fatalf("unexpected description: %q", records[0].Description)
	}
	if records[0].ResponsibleUser != nil || records[0].RelatedReference != nil {
		t.Fatal("null fields should be nil")
	}
	if records[0].ComparisonType != nil || records[0].RelatedReferenceKind != nil {
		t.Fatal("unmatched three-column rows should not have comparison metadata")
	}
	if records[3].RelatedReferenceKind == nil || *records[3].RelatedReferenceKind != "INFOBIO_NIF" {
		t.Fatalf("TP reference kind = %v", records[3].RelatedReferenceKind)
	}
}

func TestParseLatentVsTenPrintFileResolvesToInfoBio(t *testing.T) {
	records, _, err := Parse(strings.NewReader("02.2025.01.SRMG.00209-002-01-01;Solicitação de confronto de impressão latente revelada em material com impressões digitais de dois suspeitos e banco de dados do MBIS/PF. ;iuri.icap;LTvsTPF;550661.034031287/2015-07\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].ComparisonType == nil || *records[0].ComparisonType != "LTvsTPF" {
		t.Fatalf("comparison type = %v, want LTvsTPF", records[0].ComparisonType)
	}
	if records[0].RelatedReferenceKind == nil || *records[0].RelatedReferenceKind != "INFOBIO_NIF" {
		t.Fatalf("LTvsTPF reference kind = %v, want INFOBIO_NIF", records[0].RelatedReferenceKind)
	}
}

func TestParseUnmatchedLTWithComparisonType(t *testing.T) {
	records, _, err := Parse(strings.NewReader("02.2006.01.SRMG.00030-001-01-01;Tentativa de furto;mguser02;LTvsULF\n"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if records[0].ComparisonType == nil || *records[0].ComparisonType != "LTvsULF" {
		t.Fatalf("comparison type = %v", records[0].ComparisonType)
	}
	if records[0].RelatedReference != nil || records[0].RelatedReferenceKind != nil {
		t.Fatal("unmatched LT row should not have a related reference")
	}
}
