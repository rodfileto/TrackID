package examination

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // evidence frames may be JPEG
	"image/png"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/person"
	"github.com/rodfileto/trackid/storage"
)

// ErrNoImage is PairImage's answer for a feature with no stored image.
var ErrNoImage = errors.New("this face has no image")

// Image is one face of a pair, ready to show.
type Image struct {
	Data        []byte
	ContentType string
}

// PairImage returns the image of featureID, one side of a pair in the analysis as user sees it
// (the same scope as GetWorksheet), so the workspace can show a pair without being told whose face
// it is: a blind examiner's worksheet carries no person, file or trace id. A questioned face is
// cut to its box here, so the rest of the scene stays with the case. Checking that user may open
// the case (distribution.CanOpen) is the caller's job.
func PairImage(ctx context.Context, sqlDB *sql.DB, store *storage.Client, user auth.UserProfile, analysisID int64, featureID string) (Image, error) {
	q := db.New(sqlDB)
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return Image{}, err
	}
	all, _, err := scope(ctx, q, t)
	if err != nil {
		return Image{}, err
	}
	pairs := all
	if step, err := analyses.CanWork(user, t); err == nil && t.Stage != analyses.StageReport {
		decided, err := decisionsByPair(ctx, q, t.ID)
		if err != nil {
			return Image{}, err
		}
		pairs = stepScope(all, step, decided)
	}
	inScope := false
	for _, c := range pairs {
		inScope = inScope || c.pair.FeatureA == featureID || c.pair.FeatureB == featureID
	}
	if !inScope {
		return Image{}, ErrNotInScope
	}

	described, err := cases.DescribeFeatures(ctx, q, []string{featureID})
	if err != nil {
		return Image{}, err
	}
	l := described[featureID]
	switch {
	case l.IdentityFileID != 0:
		f, err := person.DownloadIdentityFile(ctx, sqlDB, store, l.PersonID, l.IdentityFileID)
		if err != nil {
			return Image{}, err
		}
		return Image{Data: f.Data, ContentType: f.ContentType}, nil
	case l.ThumbnailFileID != 0:
		f, err := cases.DownloadEvidence(ctx, sqlDB, store, l.CaseID, l.ThumbnailFileID)
		if err != nil {
			return Image{}, err
		}
		if l.ThumbnailBox == nil {
			return Image{Data: f.Data, ContentType: f.ContentType}, nil
		}
		b := l.ThumbnailBox
		crop, err := cropPNG(f.Data, image.Rect(int(b.X1), int(b.Y1), int(b.X2), int(b.Y2)))
		if err != nil {
			return Image{}, err
		}
		if crop == nil {
			return Image{}, ErrNoImage
		}
		return Image{Data: crop, ContentType: "image/png"}, nil
	}
	return Image{}, ErrNoImage
}

// cropPNG decodes data (PNG or JPEG), cuts it to box (in image pixel coordinates relative to its
// top-left corner) and re-encodes the crop as PNG; nil, nil when box doesn't overlap the image.
// The same as fingerprint's, which is unexported there.
func cropPNG(data []byte, box image.Rectangle) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode evidence image: %w", err)
	}
	box = box.Add(img.Bounds().Min).Intersect(img.Bounds())
	if box.Empty() {
		return nil, nil
	}
	crop := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	draw.Draw(crop, crop.Bounds(), img, box.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, crop); err != nil {
		return nil, fmt.Errorf("encode crop: %w", err)
	}
	return buf.Bytes(), nil
}
