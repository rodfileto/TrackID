// Package bulk loads a large identity register into TrackID and deduplicates it: what
// cmd/bulk-enroll (load and vectorize) and cmd/bulk-match (match, calibrate, apply) run. See
// docs/large-gallery-plan.md.
//
// The work is split in two because the halves are bound by different things. Enrolling is bound
// by reading and decoding images and runs once per photo. Matching is bound by GPU compute and by
// examiners' review, and reruns at every batch and every threshold version. Between them sit the
// vector shards (shards.go) and the score store (bulk_match_scores).
package bulk

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rodfileto/trackid/identity"
)

// ManifestFile is one file of a manifest enrollment. Exactly one of Path and URI is set:
//   - Path is a local file, uploaded into core's bucket as identity.UploadFile does;
//   - URI is an object kept where it is, stored as the file's reference and read through
//     storage.Client.Download ("file:///...", "s3://<bucket>/<key>", or "<bucket>/<key>" in
//     core's own bucket).
type ManifestFile struct {
	FileType    string `json:"fileType"`
	Sequence    int16  `json:"sequence,omitempty"`
	Path        string `json:"path,omitempty"`
	URI         string `json:"uri,omitempty"`
	SourcePath  string `json:"sourcePath,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
}

// ManifestEnrollment is one line of a manifest: identity.Enrollment in JSON, with files named by
// path or URI. A deployment's importer writes these; core never sees the source system's format.
type ManifestEnrollment struct {
	PersonID       string          `json:"personId"`
	PersonMeta     json.RawMessage `json:"personMeta,omitempty"`
	DocumentType   string          `json:"documentType"`
	DocumentNumber string          `json:"documentNumber"`
	FiscalNumber   string          `json:"fiscalNumber,omitempty"`
	RegisterNumber string          `json:"registerNumber"`
	Name           string          `json:"name"`
	Parent1Name    string          `json:"parent1Name,omitempty"`
	Parent1Gender  string          `json:"parent1Gender,omitempty"`
	Parent2Name    string          `json:"parent2Name,omitempty"`
	Parent2Gender  string          `json:"parent2Gender,omitempty"`
	BirthDate      string          `json:"birthDate,omitempty"`
	RegisterMeta   json.RawMessage `json:"registerMeta,omitempty"`
	Files          []ManifestFile  `json:"files"`
}

// Validate checks what Ingest requires and that every file names exactly one source.
func (m ManifestEnrollment) Validate() error {
	if m.PersonID == "" || m.DocumentType == "" || m.DocumentNumber == "" || m.RegisterNumber == "" {
		return fmt.Errorf("personId, documentType, documentNumber and registerNumber are required")
	}
	for i, f := range m.Files {
		if f.FileType == "" {
			return fmt.Errorf("file %d: fileType is required", i)
		}
		if (f.Path == "") == (f.URI == "") {
			return fmt.Errorf("file %d: exactly one of path and uri is required", i)
		}
		if f.Path != "" && !filepath.IsAbs(f.Path) {
			return fmt.Errorf("file %d: path %q is not absolute", i, f.Path)
		}
	}
	return nil
}

// Enrollment is m as identity.Ingest takes it, with files given their storage references by
// ref: the URI as it is, or what uploading Path returned. A missing parent gender is "U"
// (unknown), which identity_registers requires.
func (m ManifestEnrollment) Enrollment(refs []string) identity.Enrollment {
	gender := func(g string) string {
		if g == "" {
			return "U"
		}
		return g
	}
	e := identity.Enrollment{
		PersonID:       m.PersonID,
		PersonMeta:     m.PersonMeta,
		DocumentType:   m.DocumentType,
		DocumentNumber: m.DocumentNumber,
		FiscalNumber:   m.FiscalNumber,
		RegisterNumber: m.RegisterNumber,
		Name:           m.Name,
		Parent1Name:    m.Parent1Name,
		Parent1Gender:  gender(m.Parent1Gender),
		Parent2Name:    m.Parent2Name,
		Parent2Gender:  gender(m.Parent2Gender),
		BirthDate:      m.BirthDate,
		RegisterMeta:   m.RegisterMeta,
	}
	for i, f := range m.Files {
		source := f.SourcePath
		if source == "" {
			source = f.Path
			if source == "" {
				source = f.URI
			}
		}
		e.Files = append(e.Files, identity.FileInput{
			FileType:    f.FileType,
			Sequence:    f.Sequence,
			SourcePath:  source,
			StorageRef:  refs[i],
			ContentType: f.ContentType,
			SizeBytes:   f.SizeBytes,
		})
	}
	return e
}

// ManifestLine is one decoded line, or the error that line gave, numbered from 1.
type ManifestLine struct {
	Number     int
	Enrollment ManifestEnrollment
	Err        error
}

// ReadManifest decodes JSON Lines from r, sending every line from startLine on (1-based; blank
// lines skipped) to out, and closes out. A line that does not decode or validate is sent with its
// error, so the caller can count it and go on.
func ReadManifest(r io.Reader, startLine int, out chan<- ManifestLine) error {
	defer close(out)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	n := 0
	for scanner.Scan() {
		n++
		if n < startLine {
			continue
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		line := ManifestLine{Number: n}
		if err := json.Unmarshal([]byte(text), &line.Enrollment); err != nil {
			line.Err = err
		} else {
			line.Err = line.Enrollment.Validate()
		}
		out <- line
	}
	return scanner.Err()
}

// readLocal reads a manifest's local file.
func readLocal(path string) ([]byte, error) {
	return os.ReadFile(path)
}
