package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/embedding"
)

// WithdrawRegister takes one register's KNOWN features out of matching, keeping the identity (020_feature_withdrawal):
// for an enrolled photo found later not to be the person it was taken for. Their embeddings are deleted, so face search
// and matching no longer find them, and nothing embeds them again until RestoreRegister. The AUTOMATIC clusters drop
// the links only a SYSTEM decision made to them on the next face sync, which this asks for; links people confirmed
// stay until people undo them. by names who withdrew it (a username, or a system); reason says why. It returns how
// many features it withdrew: none when the register's were withdrawn already.
func WithdrawRegister(ctx context.Context, sqlDB *sql.DB, registerID int64, by, reason string, now time.Time, queue embedding.Enqueuer) (int, error) {
	if sqlDB == nil {
		return 0, errors.New("identity: nil db")
	}
	if strings.TrimSpace(by) == "" || strings.TrimSpace(reason) == "" {
		return 0, errors.New("identity: withdrawing needs who and why")
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	ids, err := q.WithdrawRegisterFeatures(ctx, db.WithdrawRegisterFeaturesParams{
		WithdrawnAt: sql.NullTime{Time: now, Valid: true}, WithdrawnBy: sql.NullString{String: by, Valid: true},
		WithdrawnReason: sql.NullString{String: reason, Valid: true}, RegisterID: registerID,
	})
	if err != nil {
		return 0, fmt.Errorf("identity: withdraw register %d: %w", registerID, err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if _, err := q.DeleteFeatureEmbeddings(ctx, ids); err != nil {
		return 0, fmt.Errorf("identity: withdraw register %d: embeddings: %w", registerID, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	embedding.EnqueueSyncFace(queue)
	return len(ids), nil
}

// RestoreRegister puts a withdrawn register's features back into matching and asks for their embeddings again. It
// returns how many it restored: none when none was withdrawn.
func RestoreRegister(ctx context.Context, sqlDB *sql.DB, registerID int64, queue embedding.Enqueuer) (int, error) {
	if sqlDB == nil {
		return 0, errors.New("identity: nil db")
	}
	ids, err := db.New(sqlDB).RestoreRegisterFeatures(ctx, registerID)
	if err != nil {
		return 0, fmt.Errorf("identity: restore register %d: %w", registerID, err)
	}
	if queue != nil {
		for _, id := range ids {
			task, err := embedding.NewComputeIdentityFeatureTask(id)
			if err == nil {
				_, err = queue.Enqueue(task)
			}
			if err != nil {
				log.Printf("identity: restore register %d: enqueue embedding of feature %d: %v", registerID, id, err)
			}
		}
	}
	return len(ids), nil
}
