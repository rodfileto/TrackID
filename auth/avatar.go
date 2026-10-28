package auth

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/jpeg"
	"time"

	"golang.org/x/image/draw"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/imageproc"
)

// AvatarSize is the side, in pixels, of the square picture stored for a user.
const AvatarSize = 256

// MaxAvatarUpload is the largest file SetAvatar accepts.
const MaxAvatarUpload = 10 << 20

// ErrNotAnImage is returned for an upload that is not a JPEG, PNG, BMP or TIFF image.
var ErrNotAnImage = errors.New("the file is not an image this server reads (JPEG, PNG, BMP, TIFF)")

// SetAvatar stores data as the user's picture: oriented as its EXIF says, cropped to the central
// square, resized to AvatarSize and re-encoded as JPEG, so nothing else from the file (metadata,
// location) is kept.
func (r *Repository) SetAvatar(ctx context.Context, id int64, data []byte) (UserProfile, error) {
	original, err := imageproc.Decode(data)
	if err != nil {
		return UserProfile{}, ErrNotAnImage
	}
	side := min(original.Width, original.Height)
	b := original.Image.Bounds()
	square := image.Rect(0, 0, side, side).Add(b.Min).Add(image.Pt((original.Width-side)/2, (original.Height-side)/2))
	out := image.NewRGBA(image.Rect(0, 0, AvatarSize, AvatarSize))
	draw.CatmullRom.Scale(out, out.Bounds(), original.Image, square, draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: 85}); err != nil {
		return UserProfile{}, err
	}
	if _, err := r.queries.SetUserAvatar(ctx, db.SetUserAvatarParams{ID: id, Avatar: buf.Bytes()}); err != nil {
		return UserProfile{}, err
	}
	return r.FindByID(ctx, id)
}

// RemoveAvatar deletes the user's picture.
func (r *Repository) RemoveAvatar(ctx context.Context, id int64) (UserProfile, error) {
	if _, err := r.queries.SetUserAvatar(ctx, db.SetUserAvatarParams{ID: id}); err != nil {
		return UserProfile{}, err
	}
	return r.FindByID(ctx, id)
}

// Avatar returns the user's picture (a JPEG) and when it was set; sql.ErrNoRows when they have
// none.
func (r *Repository) Avatar(ctx context.Context, id int64) ([]byte, time.Time, error) {
	row, err := r.queries.GetUserAvatar(ctx, id)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !row.AvatarUpdatedAt.Valid {
		return nil, time.Time{}, sql.ErrNoRows
	}
	return row.Avatar, row.AvatarUpdatedAt.Time, nil
}
