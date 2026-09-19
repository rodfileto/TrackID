package person

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/storage"
	"github.com/sqlc-dev/pqtype"
)

// Identity is one person's full enrollment chain: the person plus every
// identity document, register, and file/feature recorded for them. See
// MODEL.md section 2.1.
type Identity struct {
	PersonID  string          `json:"personId"`
	Meta      json.RawMessage `json:"meta,omitempty"`
	Documents []Document      `json:"documents"`
}

// Document is one identity_document row plus the registers filed under it.
type Document struct {
	DocumentID     int64      `json:"documentId"`
	DocumentNumber string     `json:"documentNumber"`
	DocumentType   string     `json:"documentType"`
	FiscalNumber   string     `json:"fiscalNumber,omitempty"`
	Registers      []Register `json:"registers"`
}

// Register is one enrollment event (identity_register) plus the files it
// produced.
type Register struct {
	RegisterID     int64           `json:"registerId"`
	RegisterNumber string          `json:"registerNumber"`
	Name           string          `json:"name"`
	Parent1Name    string          `json:"parent1Name"`
	Parent1Gender  string          `json:"parent1Gender"`
	Parent2Name    string          `json:"parent2Name"`
	Parent2Gender  string          `json:"parent2Gender"`
	BirthDate      string          `json:"birthDate,omitempty"`
	Meta           json.RawMessage `json:"meta,omitempty"`
	Files          []File          `json:"files"`
}

// File is one identity_file row -- a raw file produced by an enrollment
// event (a photo, a NIST record, ...) -- plus the KNOWN biometricfeature it
// yielded. FeatureID/FeatureType are zero when the file type yields no
// feature (e.g. a pdf).
type File struct {
	IdentityFileID int64  `json:"identityFileId"`
	FileType       string `json:"fileType"`
	Sequence       int16  `json:"sequence"`
	SourcePath     string `json:"sourcePath"`
	StorageRef     string `json:"storageRef"`
	ContentType    string `json:"contentType,omitempty"`
	SizeBytes      int64  `json:"sizeBytes,omitempty"`

	FeatureID   int64  `json:"featureId,omitempty"`
	FeatureType string `json:"featureType,omitempty"`
}

// GetIdentity loads one person's full enrollment chain -- every identity
// document, register, and file/feature recorded for them -- by
// person.person_id. Returns ErrNotFound if no person row matches.
func GetIdentity(ctx context.Context, sqlDB *sql.DB, personID string) (Identity, error) {
	if sqlDB == nil {
		return Identity{}, fmt.Errorf("person: nil db")
	}
	if personID == "" {
		return Identity{}, fmt.Errorf("person: personID is required")
	}

	q := db.New(sqlDB)

	personRow, err := q.GetPersonByPersonID(ctx, personID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Identity{}, ErrNotFound
		}
		return Identity{}, err
	}

	chainRows, err := identityChain(ctx, sqlDB, personRow.ID)
	if err != nil {
		return Identity{}, err
	}

	return buildIdentity(personID, personRow.Meta, chainRows), nil
}

// buildIdentity assembles an Identity from an already-loaded identity chain
// (see identityChain) -- the reusable core both GetIdentity and GetProfile
// build on, so a combined profile fetch doesn't pay for this join twice.
func buildIdentity(personID string, meta pqtype.NullRawMessage, chainRows []db.ListIdentityChainByPersonRow) Identity {
	identity := Identity{
		PersonID: personID,
		Meta:     json.RawMessage(meta.RawMessage),
	}

	var docOrder []int64
	docs := map[int64]*Document{}
	regOrder := map[int64][]int64{} // documentID -> registerIDs, in first-seen order
	regs := map[int64]*Register{}

	for _, r := range chainRows {
		doc, ok := docs[r.DocumentID]
		if !ok {
			doc = &Document{
				DocumentID:     r.DocumentID,
				DocumentNumber: r.DocumentNumber,
				DocumentType:   r.DocumentType,
				FiscalNumber:   r.FiscalNumber.String,
			}
			docs[r.DocumentID] = doc
			docOrder = append(docOrder, r.DocumentID)
		}

		reg, ok := regs[r.RegisterID]
		if !ok {
			reg = &Register{
				RegisterID:     r.RegisterID,
				RegisterNumber: r.RegisterNumber,
				Name:           r.Name,
				Parent1Name:    r.Parent1Name,
				Parent1Gender:  r.Parent1Gender,
				Parent2Name:    r.Parent2Name,
				Parent2Gender:  r.Parent2Gender,
				BirthDate:      r.BirthDate.String,
				Meta:           json.RawMessage(r.RegisterMeta.RawMessage),
			}
			regs[r.RegisterID] = reg
			regOrder[r.DocumentID] = append(regOrder[r.DocumentID], r.RegisterID)
		}

		if !r.IdentityFileID.Valid {
			// A register with no files uploaded yet -- keep it, just with
			// no Files entries.
			continue
		}
		reg.Files = append(reg.Files, File{
			IdentityFileID: r.IdentityFileID.Int64,
			FileType:       r.FileType.String,
			Sequence:       r.Sequence.Int16,
			SourcePath:     r.SourcePath.String,
			StorageRef:     r.StorageRef.String,
			ContentType:    r.ContentType.String,
			SizeBytes:      r.SizeBytes.Int64,
			FeatureID:      r.BiometricfeatureID.Int64,
			FeatureType:    r.FeatureType.String,
		})
	}

	for _, docID := range docOrder {
		doc := docs[docID]
		for _, regID := range regOrder[docID] {
			doc.Registers = append(doc.Registers, *regs[regID])
		}
		identity.Documents = append(identity.Documents, *doc)
	}

	return identity
}

// IdentityFileContent is a downloaded identity_file's bytes and metadata.
type IdentityFileContent struct {
	Data        []byte
	ContentType string
	Filename    string
}

// DownloadIdentityFile fetches one identity_file's content by person_id and
// file id. The file must belong to the given person (scoped through
// identity_register -> identity_document), same scoping cases.DownloadEvidence
// uses for a case's evidence files. Returns ErrNotFound if no such file
// belongs to that person.
func DownloadIdentityFile(ctx context.Context, sqlDB *sql.DB, store *storage.Client, personID string, identityFileID int64) (IdentityFileContent, error) {
	if sqlDB == nil {
		return IdentityFileContent{}, fmt.Errorf("person: nil db")
	}
	if store == nil {
		return IdentityFileContent{}, fmt.Errorf("person: object storage is not configured")
	}

	fileRow, err := db.New(sqlDB).GetIdentityFileForPerson(ctx, db.GetIdentityFileForPersonParams{
		IdentityFileID: identityFileID,
		PersonID:       personID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return IdentityFileContent{}, ErrNotFound
		}
		return IdentityFileContent{}, err
	}

	data, err := store.Download(ctx, fileRow.StorageRef)
	if err != nil {
		return IdentityFileContent{}, fmt.Errorf("person: download identity file %d: %w", identityFileID, err)
	}

	return IdentityFileContent{
		Data:        data,
		ContentType: fileRow.ContentType.String,
		Filename:    path.Base(fileRow.SourcePath),
	}, nil
}
