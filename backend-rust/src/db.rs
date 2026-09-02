use std::cmp::Ordering;
use std::collections::HashMap;

use sqlx::postgres::PgPool;
use sqlx::Row;
use uuid::Uuid;

use crate::storage::S3Storage;
use crate::types::{
    Candidate, FaceDetection, ResolvedParams, ReviewItem, Track, VideoProcessingResponse,
    VideoSummary,
};

fn to_vector(e: &Option<Vec<f64>>) -> Option<pgvector::Vector> {
    e.as_ref()
        .map(|v| pgvector::Vector::from(v.iter().map(|x| *x as f32).collect::<Vec<f32>>()))
}

pub async fn persist_video(
    pool: &PgPool,
    video_id: Uuid,
    job_id: &str,
    original_filename: Option<&str>,
    content_type: Option<&str>,
    video_storage_key: &str,
    params: &ResolvedParams,
    expected_frames: i32,
    frame_count: i32,
    total_detections: i32,
    tracks: &[Track],
) -> Result<(), sqlx::Error> {
    let mut tx = pool.begin().await?;

    sqlx::query(
        "INSERT INTO videos (
            id, job_id, original_filename, content_type, video_storage_key,
            interval_seconds, embed_interval_seconds, min_quality, min_blur,
            full_detection_every_frame, iou_threshold, cluster_eps, cluster_min_samples,
            expected_frames, frame_count, total_detections, track_count
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
        )",
    )
    .bind(video_id)
    .bind(job_id)
    .bind(original_filename)
    .bind(content_type)
    .bind(video_storage_key)
    .bind(params.interval_seconds)
    .bind(params.embed_interval_seconds)
    .bind(params.min_quality)
    .bind(params.min_blur)
    .bind(params.full_detection_every_frame)
    .bind(params.iou_threshold)
    .bind(params.cluster_eps)
    .bind(params.cluster_min_samples)
    .bind(expected_frames)
    .bind(frame_count)
    .bind(total_detections)
    .bind(tracks.len() as i32)
    .execute(&mut *tx)
    .await?;

    // Two-phase insert to satisfy the person_tracks <-> face_detections
    // circular FK: insert both sides with best_face_detection_id NULL, then
    // UPDATE it once the FaceDetection ids are known (mirrors the Python
    // video_persistence_service).
    let mut best_face_ids: Vec<(Uuid, Uuid)> = Vec::new();

    for track in tracks {
        let track_id = Uuid::new_v4();
        let rep = to_vector(&track.best_face.embedding);

        sqlx::query(
            "INSERT INTO person_tracks (id, video_id, track_local_id, face_count, representative_embedding)
             VALUES ($1, $2, $3, $4, $5)",
        )
        .bind(track_id)
        .bind(video_id)
        .bind(track.track_id)
        .bind(track.all_faces.len() as i32)
        .bind(rep)
        .execute(&mut *tx)
        .await?;

        for det in &track.all_faces {
            let det_id = Uuid::new_v4();
            let emb = to_vector(&det.embedding);

            sqlx::query(
                "INSERT INTO face_detections (
                    id, video_id, person_track_id, frame_number, timestamp_seconds,
                    confidence, quality_score, blur_score, bbox, is_embedding,
                    embedding, estimated_age, estimated_gender, crop_storage_key
                ) VALUES (
                    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
                )",
            )
            .bind(det_id)
            .bind(video_id)
            .bind(track_id)
            .bind(det.frame_number)
            .bind(det.timestamp_seconds)
            .bind(det.confidence)
            .bind(det.quality_score)
            .bind(det.blur_score)
            .bind(det.bbox.clone())
            .bind(det.is_embedding)
            .bind(emb)
            .bind(det.estimated_age)
            .bind(det.estimated_gender.clone())
            .bind(det.crop_storage_key.clone())
            .execute(&mut *tx)
            .await?;

            if det.is_best_face == Some(true) {
                best_face_ids.push((track_id, det_id));
            }
        }
    }

    for (track_id, det_id) in best_face_ids {
        sqlx::query("UPDATE person_tracks SET best_face_detection_id = $1 WHERE id = $2")
            .bind(det_id)
            .bind(track_id)
            .execute(&mut *tx)
            .await?;
    }

    tx.commit().await?;
    Ok(())
}

pub async fn list_videos(
    pool: &PgPool,
    page: i32,
    page_size: i32,
) -> Result<(Vec<VideoSummary>, i64), sqlx::Error> {
    let total: i64 = sqlx::query_scalar("SELECT count(*) FROM videos")
        .fetch_one(pool)
        .await?;

    let rows = sqlx::query(
        "SELECT id, original_filename, created_at, track_count, frame_count, total_detections
         FROM videos ORDER BY created_at DESC OFFSET $1 LIMIT $2",
    )
    .bind(((page - 1) * page_size) as i64)
    .bind(page_size as i64)
    .fetch_all(pool)
    .await?;

    let items = rows
        .iter()
        .map(|row| {
            let id: Uuid = row.get("id");
            VideoSummary {
                video_id: id.to_string(),
                original_filename: row.get("original_filename"),
                created_at: row.get("created_at"),
                track_count: row.get("track_count"),
                frame_count: row.get("frame_count"),
                total_detections: row.get("total_detections"),
            }
        })
        .collect();

    Ok((items, total))
}

struct TrackRow {
    id: Uuid,
    track_local_id: i32,
    best_face_detection_id: Option<Uuid>,
}

struct DetRow {
    id: Uuid,
    person_track_id: Option<Uuid>,
    frame_number: i32,
    timestamp_seconds: f64,
    confidence: f64,
    quality_score: Option<f64>,
    blur_score: Option<f64>,
    bbox: Option<Vec<f64>>,
    is_embedding: bool,
    estimated_age: Option<i32>,
    estimated_gender: Option<String>,
    crop_storage_key: Option<String>,
}

pub async fn get_video(
    pool: &PgPool,
    storage: &S3Storage,
    video_id: Uuid,
) -> Result<Option<VideoProcessingResponse>, sqlx::Error> {
    let exists = sqlx::query("SELECT id FROM videos WHERE id = $1")
        .bind(video_id)
        .fetch_optional(pool)
        .await?;
    if exists.is_none() {
        return Ok(None);
    }

    let track_rows = sqlx::query(
        "SELECT id, track_local_id, best_face_detection_id FROM person_tracks
         WHERE video_id = $1 ORDER BY track_local_id",
    )
    .bind(video_id)
    .fetch_all(pool)
    .await?;

    let det_rows = sqlx::query(
        "SELECT id, person_track_id, frame_number, timestamp_seconds, confidence,
                quality_score, blur_score, bbox, is_embedding, estimated_age,
                estimated_gender, crop_storage_key
         FROM face_detections WHERE video_id = $1",
    )
    .bind(video_id)
    .fetch_all(pool)
    .await?;

    let tracks: Vec<TrackRow> = track_rows
        .iter()
        .map(|r| TrackRow {
            id: r.get("id"),
            track_local_id: r.get("track_local_id"),
            best_face_detection_id: r.get("best_face_detection_id"),
        })
        .collect();

    let dets: Vec<DetRow> = det_rows
        .iter()
        .map(|r| DetRow {
            id: r.get("id"),
            person_track_id: r.get("person_track_id"),
            frame_number: r.get("frame_number"),
            timestamp_seconds: r.get("timestamp_seconds"),
            confidence: r.get("confidence"),
            quality_score: r.get("quality_score"),
            blur_score: r.get("blur_score"),
            bbox: r.get("bbox"),
            is_embedding: r.get("is_embedding"),
            estimated_age: r.get("estimated_age"),
            estimated_gender: r.get("estimated_gender"),
            crop_storage_key: r.get("crop_storage_key"),
        })
        .collect();

    let mut out_tracks = Vec::new();
    for tr in &tracks {
        let mut track_dets: Vec<&DetRow> = dets
            .iter()
            .filter(|d| d.person_track_id == Some(tr.id))
            .collect();
        if track_dets.is_empty() {
            continue;
        }
        track_dets.sort_by(|a, b| {
            b.quality_score
                .unwrap_or(f64::NEG_INFINITY)
                .partial_cmp(&a.quality_score.unwrap_or(f64::NEG_INFINITY))
                .unwrap_or(std::cmp::Ordering::Equal)
        });

        let best = track_dets
            .iter()
            .find(|d| Some(d.id) == tr.best_face_detection_id)
            .copied()
            .unwrap_or(track_dets[0]);

        let to_det = async |d: &DetRow| -> Result<FaceDetection, sqlx::Error> {
            let face_crop = match &d.crop_storage_key {
                Some(key) => Some(
                    storage
                        .presign_get(key, std::time::Duration::from_secs(3600))
                        .await
                        .map_err(|e| sqlx::Error::Protocol(format!("presign: {e}")))?,
                ),
                None => None,
            };
            Ok(FaceDetection {
                frame_number: d.frame_number,
                timestamp_seconds: d.timestamp_seconds,
                confidence: d.confidence,
                quality_score: d.quality_score,
                blur_score: d.blur_score,
                bbox: d.bbox.clone(),
                is_embedding: d.is_embedding,
                embedding: None,
                estimated_age: d.estimated_age,
                estimated_gender: d.estimated_gender.clone(),
                cluster_id: Some(tr.track_local_id),
                face_crop,
                is_best_face: None,
                crop_storage_key: d.crop_storage_key.clone(),
            })
        };

        let mut all_faces = Vec::with_capacity(track_dets.len());
        for d in &track_dets {
            all_faces.push(to_det(d).await?);
        }
        let best_face = to_det(best).await?;

        out_tracks.push(Track {
            track_id: tr.track_local_id,
            best_face,
            all_faces,
        });
    }

    Ok(Some(VideoProcessingResponse {
        track_count: out_tracks.len() as i32,
        tracks: out_tracks,
        video_id: Some(video_id.to_string()),
    }))
}

// ─── Face resolution (face-record model) ───────────────────────────────────

pub async fn ensure_face_schema(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query("CREATE EXTENSION IF NOT EXISTS vector")
        .execute(pool)
        .await?;
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS face_embeddings (
            id uuid PRIMARY KEY,
            face_record_id varchar NOT NULL,
            person_id varchar,
            identity_id varchar,
            source varchar NOT NULL,
            embedding vector(512) NOT NULL,
            quality_score double precision,
            detection_confidence double precision,
            source_image_storage_key varchar,
            created_at timestamptz NOT NULL DEFAULT now()
        )",
    )
    .execute(pool)
    .await?;
    sqlx::query(
        "CREATE INDEX IF NOT EXISTS ix_face_embeddings_embedding_hnsw
         ON face_embeddings USING hnsw (embedding vector_cosine_ops)",
    )
    .execute(pool)
    .await?;
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS face_review_items (
            id uuid PRIMARY KEY,
            embedding_id uuid NOT NULL REFERENCES face_embeddings(id) ON DELETE CASCADE,
            face_record_id varchar NOT NULL,
            candidate_kind varchar NOT NULL,
            candidate_person_id varchar,
            candidate_identity_id varchar,
            similarity double precision NOT NULL,
            top_candidates jsonb NOT NULL DEFAULT '[]',
            status varchar NOT NULL DEFAULT 'pending',
            reviewed_by varchar,
            reviewed_at timestamptz,
            created_at timestamptz NOT NULL DEFAULT now()
        )",
    )
    .execute(pool)
    .await?;
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS users (
            id uuid PRIMARY KEY,
            username varchar UNIQUE NOT NULL,
            password_hash varchar NOT NULL,
            created_at timestamptz NOT NULL DEFAULT now()
        )",
    )
    .execute(pool)
    .await?;

    // Video-processing tables (formerly created by the Python alembic
    // migrations). The person_tracks <-> face_detections circular FK is added
    // separately (idempotently) because both tables must exist first.
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS videos (
            id uuid PRIMARY KEY,
            job_id varchar,
            original_filename varchar,
            content_type varchar,
            video_storage_key varchar,
            interval_seconds double precision NOT NULL,
            embed_interval_seconds double precision NOT NULL,
            min_quality double precision NOT NULL,
            min_blur double precision NOT NULL,
            full_detection_every_frame boolean NOT NULL DEFAULT false,
            iou_threshold double precision NOT NULL,
            cluster_eps double precision NOT NULL,
            cluster_min_samples integer NOT NULL,
            expected_frames integer NOT NULL DEFAULT 0,
            frame_count integer NOT NULL DEFAULT 0,
            total_detections integer NOT NULL DEFAULT 0,
            track_count integer NOT NULL DEFAULT 0,
            created_at timestamptz NOT NULL DEFAULT now()
        )",
    )
    .execute(pool)
    .await?;
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS person_tracks (
            id uuid PRIMARY KEY,
            video_id uuid NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
            track_local_id integer NOT NULL,
            best_face_detection_id uuid,
            face_count integer NOT NULL DEFAULT 0,
            representative_embedding vector(512),
            created_at timestamptz NOT NULL DEFAULT now(),
            UNIQUE (video_id, track_local_id)
        )",
    )
    .execute(pool)
    .await?;
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS face_detections (
            id uuid PRIMARY KEY,
            video_id uuid NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
            person_track_id uuid REFERENCES person_tracks(id) ON DELETE CASCADE,
            frame_number integer NOT NULL,
            timestamp_seconds double precision NOT NULL,
            confidence double precision NOT NULL,
            quality_score double precision,
            blur_score double precision,
            bbox double precision[],
            is_embedding boolean NOT NULL,
            embedding vector(512),
            estimated_age integer,
            estimated_gender varchar,
            crop_storage_key varchar,
            created_at timestamptz NOT NULL DEFAULT now(),
            CONSTRAINT ck_face_detection_bbox_len CHECK (bbox IS NULL OR array_length(bbox, 1) = 4)
        )",
    )
    .execute(pool)
    .await?;
    sqlx::query(
        "DO $$
         BEGIN
           IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_person_tracks_best_face_detection_id') THEN
             ALTER TABLE person_tracks ADD CONSTRAINT fk_person_tracks_best_face_detection_id
               FOREIGN KEY (best_face_detection_id) REFERENCES face_detections(id) ON DELETE SET NULL;
           END IF;
         END $$;",
    )
    .execute(pool)
    .await?;
    sqlx::query("CREATE INDEX IF NOT EXISTS ix_videos_job_id ON videos (job_id)")
        .execute(pool)
        .await?;
    sqlx::query("CREATE INDEX IF NOT EXISTS ix_face_detections_video_id ON face_detections (video_id)")
        .execute(pool)
        .await?;
    sqlx::query(
        "CREATE INDEX IF NOT EXISTS ix_face_detections_person_track_id ON face_detections (person_track_id)",
    )
    .execute(pool)
    .await?;
    sqlx::query("CREATE INDEX IF NOT EXISTS ix_person_tracks_video_id ON person_tracks (video_id)")
        .execute(pool)
        .await?;
    sqlx::query(
        "CREATE INDEX IF NOT EXISTS ix_face_detections_embedding_hnsw ON face_detections USING hnsw (embedding vector_cosine_ops)",
    )
    .execute(pool)
    .await?;
    sqlx::query(
        "CREATE INDEX IF NOT EXISTS ix_person_tracks_representative_embedding_hnsw ON person_tracks USING hnsw (representative_embedding vector_cosine_ops)",
    )
    .execute(pool)
    .await?;
    Ok(())
}

fn to_vector_f32(embedding: &[f64]) -> pgvector::Vector {
    pgvector::Vector::from(embedding.iter().map(|x| *x as f32).collect::<Vec<f32>>())
}

#[allow(clippy::too_many_arguments)]
pub async fn insert_face_embedding(
    pool: &PgPool,
    face_record_id: &str,
    person_id: Option<&str>,
    identity_id: Option<&str>,
    source: &str,
    embedding: &[f64],
    quality: Option<f64>,
    confidence: Option<f64>,
    source_key: Option<&str>,
) -> Result<Uuid, sqlx::Error> {
    let id = Uuid::new_v4();
    sqlx::query(
        "INSERT INTO face_embeddings (
            id, face_record_id, person_id, identity_id, source, embedding,
            quality_score, detection_confidence, source_image_storage_key
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
    )
    .bind(id)
    .bind(face_record_id)
    .bind(person_id)
    .bind(identity_id)
    .bind(source)
    .bind(to_vector_f32(embedding))
    .bind(quality)
    .bind(confidence)
    .bind(source_key)
    .execute(pool)
    .await?;
    Ok(id)
}

pub async fn set_embedding_person_id(
    pool: &PgPool,
    embedding_id: Uuid,
    person_id: &str,
) -> Result<(), sqlx::Error> {
    sqlx::query("UPDATE face_embeddings SET person_id = $1 WHERE id = $2")
        .bind(person_id)
        .bind(embedding_id)
        .execute(pool)
        .await?;
    Ok(())
}

/// ANN search (pgvector HNSW cosine) over both pools, deduped to the single
/// best-matching observation per Person (surveillance) or Identity
/// (enrollment), merged and re-ranked.
pub async fn find_candidates(
    pool: &PgPool,
    embedding: &[f64],
    pool_size: i64,
    top_k: usize,
) -> Result<Vec<Candidate>, sqlx::Error> {
    let emb = to_vector_f32(embedding);
    let mut best: HashMap<(String, String), Candidate> = HashMap::new();

    let surveillance_rows = sqlx::query(
        "SELECT id, person_id, embedding <=> $1 AS distance
         FROM face_embeddings
         WHERE source = 'surveillance' AND person_id IS NOT NULL
         ORDER BY embedding <=> $1 LIMIT $2",
    )
    .bind(&emb)
    .bind(pool_size)
    .fetch_all(pool)
    .await?;

    for row in surveillance_rows {
        let id: Uuid = row.get("id");
        let person_id: Option<String> = row.get("person_id");
        let distance: f64 = row.get("distance");
        if let Some(pid) = person_id {
            let similarity = 1.0 - distance;
            let cand = Candidate {
                kind: "person".to_string(),
                person_id: Some(pid.clone()),
                identity_id: None,
                similarity,
                embedding_id: id.to_string(),
            };
            best.entry(("person".to_string(), pid))
                .and_modify(|e| {
                    if similarity > e.similarity {
                        *e = cand.clone()
                    }
                })
                .or_insert(cand);
        }
    }

    let enrollment_rows = sqlx::query(
        "SELECT id, identity_id, embedding <=> $1 AS distance
         FROM face_embeddings
         WHERE source = 'enrollment' AND identity_id IS NOT NULL
         ORDER BY embedding <=> $1 LIMIT $2",
    )
    .bind(&emb)
    .bind(pool_size)
    .fetch_all(pool)
    .await?;

    for row in enrollment_rows {
        let id: Uuid = row.get("id");
        let identity_id: Option<String> = row.get("identity_id");
        let distance: f64 = row.get("distance");
        if let Some(iid) = identity_id {
            let similarity = 1.0 - distance;
            let cand = Candidate {
                kind: "identity".to_string(),
                person_id: None,
                identity_id: Some(iid.clone()),
                similarity,
                embedding_id: id.to_string(),
            };
            best.entry(("identity".to_string(), iid))
                .and_modify(|e| {
                    if similarity > e.similarity {
                        *e = cand.clone()
                    }
                })
                .or_insert(cand);
        }
    }

    let mut all: Vec<Candidate> = best.into_values().collect();
    all.sort_by(|a, b| b.similarity.partial_cmp(&a.similarity).unwrap_or(Ordering::Equal));
    all.truncate(top_k);
    Ok(all)
}

pub async fn create_review_item(
    pool: &PgPool,
    embedding_id: Uuid,
    face_record_id: &str,
    top: &Candidate,
    candidates: &[Candidate],
) -> Result<Uuid, sqlx::Error> {
    let id = Uuid::new_v4();
    let top_candidates = serde_json::to_value(candidates).unwrap_or_default();
    sqlx::query(
        "INSERT INTO face_review_items (
            id, embedding_id, face_record_id, candidate_kind,
            candidate_person_id, candidate_identity_id, similarity, top_candidates
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
    )
    .bind(id)
    .bind(embedding_id)
    .bind(face_record_id)
    .bind(&top.kind)
    .bind(&top.person_id)
    .bind(&top.identity_id)
    .bind(top.similarity)
    .bind(top_candidates)
    .execute(pool)
    .await?;
    Ok(id)
}

pub struct ReviewItemRow {
    pub embedding_id: Uuid,
    pub face_record_id: String,
    pub similarity: f64,
    pub status: String,
}

pub async fn get_review_item(pool: &PgPool, id: Uuid) -> Result<Option<ReviewItemRow>, sqlx::Error> {
    let row = sqlx::query(
        "SELECT embedding_id, face_record_id, similarity, status
         FROM face_review_items WHERE id = $1",
    )
    .bind(id)
    .fetch_optional(pool)
    .await?;

    let Some(row) = row else { return Ok(None) };

    Ok(Some(ReviewItemRow {
        embedding_id: row.get("embedding_id"),
        face_record_id: row.get("face_record_id"),
        similarity: row.get("similarity"),
        status: row.get("status"),
    }))
}

pub async fn set_review_status(
    pool: &PgPool,
    id: Uuid,
    status: &str,
    reviewed_by: &str,
) -> Result<(), sqlx::Error> {
    sqlx::query(
        "UPDATE face_review_items SET status = $1, reviewed_by = $2, reviewed_at = now()
         WHERE id = $3",
    )
    .bind(status)
    .bind(reviewed_by)
    .bind(id)
    .execute(pool)
    .await?;
    Ok(())
}

pub async fn list_review_items(
    pool: &PgPool,
    status: Option<&str>,
    page: i32,
    page_size: i32,
) -> Result<(Vec<ReviewItem>, i64), sqlx::Error> {
    let total: i64 = match status {
        Some(s) => {
            sqlx::query_scalar("SELECT count(*) FROM face_review_items WHERE status = $1")
                .bind(s)
                .fetch_one(pool)
                .await?
        }
        None => {
            sqlx::query_scalar("SELECT count(*) FROM face_review_items")
                .fetch_one(pool)
                .await?
        }
    };

    let rows = match status {
        Some(s) => sqlx::query(
            "SELECT id, embedding_id, face_record_id, candidate_kind, candidate_person_id,
                    candidate_identity_id, similarity, top_candidates, status,
                    reviewed_by, reviewed_at, created_at
             FROM face_review_items WHERE status = $1
             ORDER BY created_at DESC OFFSET $2 LIMIT $3",
        )
        .bind(s)
        .bind(((page - 1) * page_size) as i64)
        .bind(page_size as i64)
        .fetch_all(pool)
        .await?,
        None => sqlx::query(
            "SELECT id, embedding_id, face_record_id, candidate_kind, candidate_person_id,
                    candidate_identity_id, similarity, top_candidates, status,
                    reviewed_by, reviewed_at, created_at
             FROM face_review_items
             ORDER BY created_at DESC OFFSET $1 LIMIT $2",
        )
        .bind(((page - 1) * page_size) as i64)
        .bind(page_size as i64)
        .fetch_all(pool)
        .await?,
    };

    let items = rows
        .iter()
        .map(|r| {
            let top_candidates: Vec<Candidate> =
                serde_json::from_value(r.get("top_candidates")).unwrap_or_default();
            let reviewed_at: Option<chrono::DateTime<chrono::Utc>> = r.get("reviewed_at");
            let created_at: chrono::DateTime<chrono::Utc> = r.get("created_at");
            ReviewItem {
                id: r.get::<Uuid, _>("id").to_string(),
                embedding_id: r.get::<Uuid, _>("embedding_id").to_string(),
                face_record_id: r.get("face_record_id"),
                candidate_kind: r.get("candidate_kind"),
                candidate_person_id: r.get("candidate_person_id"),
                candidate_identity_id: r.get("candidate_identity_id"),
                similarity: r.get("similarity"),
                top_candidates,
                status: r.get("status"),
                reviewed_by: r.get("reviewed_by"),
                reviewed_at: reviewed_at.map(|d| d.to_rfc3339()),
                created_at: created_at.to_rfc3339(),
            }
        })
        .collect();

    Ok((items, total))
}

pub async fn enrollment_face_record_ids(
    pool: &PgPool,
    identity_id: &str,
) -> Result<Vec<String>, sqlx::Error> {
    let rows = sqlx::query("SELECT face_record_id FROM face_embeddings WHERE identity_id = $1")
        .bind(identity_id)
        .fetch_all(pool)
        .await?;
    Ok(rows
        .iter()
        .map(|r| r.get::<String, _>("face_record_id"))
        .collect())
}

// ─── Auth (users) ──────────────────────────────────────────────────────────

pub struct User {
    pub id: String,
    pub username: String,
    pub password_hash: String,
}

pub async fn create_user(
    pool: &PgPool,
    id: Uuid,
    username: &str,
    password_hash: &str,
) -> Result<(), sqlx::Error> {
    sqlx::query("INSERT INTO users (id, username, password_hash) VALUES ($1, $2, $3)")
        .bind(id)
        .bind(username)
        .bind(password_hash)
        .execute(pool)
        .await?;
    Ok(())
}

pub async fn get_user_by_username(
    pool: &PgPool,
    username: &str,
) -> Result<Option<User>, sqlx::Error> {
    let row = sqlx::query("SELECT id, username, password_hash FROM users WHERE username = $1")
        .bind(username)
        .fetch_optional(pool)
        .await?;

    Ok(row.map(|r| User {
        id: r.get::<Uuid, _>("id").to_string(),
        username: r.get("username"),
        password_hash: r.get("password_hash"),
    }))
}

pub fn is_unique_violation(e: &sqlx::Error) -> bool {
    matches!(e, sqlx::Error::Database(db) if db.code().as_deref() == Some("23505"))
}
