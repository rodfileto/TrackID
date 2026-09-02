mod auth;
mod db;
mod error;
mod graph;
mod jobs;
mod ontology;
mod resolve;
mod sidecar;
mod storage;
mod types;

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::{Duration, Instant};

use axum::extract::{Multipart, Path, Query, State};
use axum::http::StatusCode;
use axum::routing::{get, post};
use axum::{Extension, Json, Router};
use base64::Engine as _;
use serde::Deserialize;
use sqlx::postgres::PgPoolOptions;
use uuid::Uuid;

use auth::{auth_middleware, AuthState, AuthUser};
use error::ApiError;
use graph::GraphService;
use jobs::{JobStatus, JobStore};
use resolve::ResolutionConfig;
use sidecar::{SidecarClient, SidecarError};
use storage::S3Storage;
use types::{
    IdentityResponse, IncidentDetail, IncidentSummary, ObservationSummary, ObservationsResponse,
    PersonDetail, PersonListResponse, PersonSummary, ResolvedParams, ReviewListResponse,
    SituationDetail, SituationSummary, TargetSystemDetail, TargetSystemListResponse,
    TargetSystemSummary, Track, VideoJobCreated, VideoJobStatus, VideoListResponse,
    VideoProcessingResponse,
};

#[derive(Clone)]
struct AppState {
    pool: sqlx::PgPool,
    storage: S3Storage,
    sidecar: SidecarClient,
    jobs: Arc<JobStore>,
    graph: GraphService,
    resolution: ResolutionConfig,
    auth: AuthState,
}

fn env(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

fn extension_of(filename: Option<&str>) -> String {
    filename
        .and_then(|f| std::path::Path::new(f).extension())
        .and_then(|e| e.to_str())
        .map(|e| format!(".{e}"))
        .unwrap_or_else(|| ".mp4".to_string())
}

fn data_uri_to_bytes(uri: &str) -> Option<Vec<u8>> {
    let (_, payload) = uri.split_once(',')?;
    base64::engine::general_purpose::STANDARD.decode(payload).ok()
}

#[derive(Deserialize)]
struct ProcessVideoQuery {
    interval_seconds: Option<f64>,
    embed_interval_seconds: Option<f64>,
    min_quality: Option<f64>,
    min_blur: Option<f64>,
    full_detection_every_frame: Option<bool>,
    iou_threshold: Option<f64>,
    cluster_eps: Option<f64>,
    cluster_min_samples: Option<i32>,
}

impl ProcessVideoQuery {
    fn resolved(&self) -> ResolvedParams {
        ResolvedParams {
            interval_seconds: self.interval_seconds.unwrap_or(0.5),
            embed_interval_seconds: self.embed_interval_seconds.unwrap_or(1.0),
            min_quality: self.min_quality.unwrap_or(0.0),
            min_blur: self.min_blur.unwrap_or(50.0),
            full_detection_every_frame: self.full_detection_every_frame.unwrap_or(false),
            iou_threshold: self.iou_threshold.unwrap_or(0.15),
            cluster_eps: self.cluster_eps.unwrap_or(0.45),
            cluster_min_samples: self.cluster_min_samples.unwrap_or(2),
        }
    }
}

#[derive(Deserialize)]
struct ListQuery {
    page: Option<i32>,
    page_size: Option<i32>,
}

async fn process_video(
    State(state): State<AppState>,
    Query(q): Query<ProcessVideoQuery>,
    mut multipart: Multipart,
) -> Result<Json<VideoJobCreated>, ApiError> {
    let mut file_name: Option<String> = None;
    let mut content_type: Option<String> = None;
    let mut data: Option<Vec<u8>> = None;

    while let Some(field) = multipart
        .next_field()
        .await
        .map_err(|e| ApiError::bad_request(e.to_string()))?
    {
        if field.name() == Some("file") {
            file_name = field.file_name().map(String::from);
            content_type = field.content_type().map(String::from);
            data = Some(
                field
                    .bytes()
                    .await
                    .map_err(|e| ApiError::bad_request(e.to_string()))?
                    .to_vec(),
            );
            break;
        }
    }

    let data = data.ok_or_else(|| ApiError::bad_request("missing file field"))?;
    let content_type = content_type.unwrap_or_default();
    if !content_type.starts_with("video/") {
        return Err(ApiError::bad_request("File must be a video"));
    }

    let params = q.resolved();
    let ext = extension_of(file_name.as_deref());
    let temp_path = std::env::temp_dir().join(format!("{}{}", Uuid::new_v4(), ext));
    tokio::fs::write(&temp_path, &data)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    let sidecar_job_id = match state
        .sidecar
        .create_video_job(&temp_path, file_name.as_deref(), Some(&content_type), &params)
        .await
    {
        Ok(id) => id,
        Err(e) => {
            let _ = tokio::fs::remove_file(&temp_path).await;
            let status = if e.status == 400 {
                StatusCode::BAD_REQUEST
            } else {
                StatusCode::BAD_GATEWAY
            };
            return Err(ApiError::new(status, e.detail));
        }
    };

    let job_id = Uuid::new_v4().to_string();
    state.jobs.create(job_id.clone());

    let st = state.clone();
    let jid = job_id.clone();
    tokio::spawn(async move {
        poll_and_persist(st, jid, sidecar_job_id, temp_path, file_name, content_type, params).await;
    });

    Ok(Json(VideoJobCreated { job_id }))
}

async fn poll_and_persist(
    state: AppState,
    job_id: String,
    sidecar_job_id: String,
    temp_path: PathBuf,
    original_filename: Option<String>,
    content_type: String,
    params: ResolvedParams,
) {
    // Poll the ML sidecar until the job completes or fails.
    let status = loop {
        match state.sidecar.get_video_job(&sidecar_job_id).await {
            Ok(st) => {
                state.jobs.update_progress(
                    &job_id,
                    st.frame_count,
                    st.expected_frames,
                    st.total_detections,
                );
                match st.status.as_str() {
                    "failed" => {
                        let msg = st.error.unwrap_or_else(|| "video processing failed".to_string());
                        state.jobs.fail(&job_id, msg);
                        let _ = tokio::fs::remove_file(&temp_path).await;
                        return;
                    }
                    "completed" => break st,
                    _ => tokio::time::sleep(Duration::from_millis(500)).await,
                }
            }
            Err(e) => {
                state.jobs.fail(&job_id, format!("ML sidecar error: {e}"));
                let _ = tokio::fs::remove_file(&temp_path).await;
                return;
            }
        }
    };

    let mut tracks = status
        .result
        .map(|r| r.tracks)
        .unwrap_or_default();

    let video_id = Uuid::new_v4();
    let ext = extension_of(original_filename.as_deref());
    let video_key = format!("videos/{video_id}/original{ext}");

    let persisted = persist_run(
        &state,
        &video_id,
        &job_id,
        original_filename.as_deref(),
        Some(&content_type),
        &video_key,
        &params,
        status.expected_frames,
        status.frame_count,
        status.total_detections,
        &temp_path,
        &mut tracks,
    )
    .await;

    let video_id = match persisted {
        Ok(()) => Some(video_id.to_string()),
        Err(e) => {
            tracing::warn!("failed to persist video results for job {job_id}: {e}");
            None
        }
    };

    state.jobs.complete(&job_id, tracks, video_id);
    let _ = tokio::fs::remove_file(&temp_path).await;
}

#[allow(clippy::too_many_arguments)]
async fn persist_run(
    state: &AppState,
    video_id: &Uuid,
    job_id: &str,
    original_filename: Option<&str>,
    content_type: Option<&str>,
    video_key: &str,
    params: &ResolvedParams,
    expected_frames: i32,
    frame_count: i32,
    total_detections: i32,
    temp_path: &std::path::Path,
    tracks: &mut [Track],
) -> anyhow::Result<()> {
    state
        .storage
        .put_file(video_key, temp_path, content_type.unwrap_or("video/mp4"))
        .await?;

    for track in tracks.iter_mut() {
        for det in track.all_faces.iter_mut() {
            if let Some(data_uri) = &det.face_crop {
                let crop_key = format!("videos/{video_id}/crops/{}.jpg", Uuid::new_v4());
                match data_uri_to_bytes(data_uri) {
                    Some(bytes) => match state.storage.put_bytes(&crop_key, bytes, "image/jpeg").await {
                        Ok(()) => det.crop_storage_key = Some(crop_key),
                        Err(e) => tracing::warn!("crop upload failed (frame {}): {e}", det.frame_number),
                    },
                    None => tracing::warn!("malformed crop data URI at frame {}", det.frame_number),
                }
            }
        }
    }

    db::persist_video(
        &state.pool,
        *video_id,
        job_id,
        original_filename,
        content_type,
        video_key,
        params,
        expected_frames,
        frame_count,
        total_detections,
        tracks,
    )
    .await?;

    Ok(())
}

async fn get_job(
    State(state): State<AppState>,
    Path(job_id): Path<String>,
) -> Result<Json<VideoJobStatus>, ApiError> {
    let snap = state
        .jobs
        .get(&job_id)
        .ok_or_else(|| ApiError::not_found("Job not found"))?;

    let finished = snap.finished_at.unwrap_or_else(Instant::now);
    let elapsed = finished.duration_since(snap.started_at).as_secs_f64();

    let percent = if snap.expected_frames > 0 {
        (snap.frame_count as f64 / snap.expected_frames as f64 * 100.0).min(100.0)
    } else {
        0.0
    };

    let eta = if snap.status == JobStatus::Processing
        && snap.frame_count > 0
        && snap.expected_frames > snap.frame_count
    {
        let rate = snap.frame_count as f64 / elapsed.max(1e-9);
        if rate > 0.0 {
            Some((snap.expected_frames - snap.frame_count) as f64 / rate)
        } else {
            None
        }
    } else {
        None
    };

    let result = snap.tracks.map(|tracks| VideoProcessingResponse {
        track_count: tracks.len() as i32,
        tracks,
        video_id: snap.video_id.clone(),
    });

    Ok(Json(VideoJobStatus {
        job_id: snap.job_id,
        status: snap.status.as_str(),
        frame_count: snap.frame_count,
        expected_frames: snap.expected_frames,
        percent,
        elapsed_seconds: elapsed,
        eta_seconds: eta,
        error: snap.error,
        result,
    }))
}

async fn list_videos(
    State(state): State<AppState>,
    Query(q): Query<ListQuery>,
) -> Result<Json<VideoListResponse>, ApiError> {
    let page = q.page.unwrap_or(1).max(1);
    let page_size = q.page_size.unwrap_or(10).clamp(1, 100);

    let (items, total) = db::list_videos(&state.pool, page, page_size)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(VideoListResponse {
        items,
        total,
        page,
        page_size,
    }))
}

async fn get_video(
    State(state): State<AppState>,
    Path(video_id): Path<String>,
) -> Result<Json<VideoProcessingResponse>, ApiError> {
    let id =
        Uuid::parse_str(&video_id).map_err(|_| ApiError::not_found("Video not found"))?;

    match db::get_video(&state.pool, &state.storage, id)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?
    {
        Some(resp) => Ok(Json(resp)),
        None => Err(ApiError::not_found("Video not found")),
    }
}

async fn health() -> Json<serde_json::Value> {
    Json(serde_json::json!({ "status": "ok" }))
}

// ─── Face resolution (face-record model) ───────────────────────────────────

fn ierr(e: impl std::fmt::Display) -> ApiError {
    ApiError::internal(e.to_string())
}

fn sidecar_err(e: SidecarError) -> ApiError {
    let status = if e.status == 400 {
        StatusCode::BAD_REQUEST
    } else {
        StatusCode::BAD_GATEWAY
    };
    ApiError::new(status, e.detail)
}

async fn parse_multipart(
    multipart: &mut Multipart,
) -> Result<(HashMap<String, String>, Option<(Vec<u8>, Option<String>, String)>), ApiError> {
    let mut fields = HashMap::new();
    let mut file = None;
    while let Some(field) = multipart
        .next_field()
        .await
        .map_err(|e| ApiError::bad_request(e.to_string()))?
    {
        let name = field.name().unwrap_or("").to_string();
        if field.file_name().is_some() {
            let content_type = field.content_type().unwrap_or("").to_string();
            let bytes = field
                .bytes()
                .await
                .map_err(|e| ApiError::bad_request(e.to_string()))?
                .to_vec();
            file = Some((bytes, Some(name), content_type));
        } else {
            let text = field
                .text()
                .await
                .map_err(|e| ApiError::bad_request(e.to_string()))?;
            fields.insert(name, text);
        }
    }
    Ok((fields, file))
}

async fn upload_face_crop(
    state: &AppState,
    folder_id: &str,
    face_crop: &Option<String>,
) -> Option<String> {
    let uri = face_crop.as_ref()?;
    let bytes = data_uri_to_bytes(uri)?;
    let key = format!("faces/{folder_id}/crop.jpg");
    match state.storage.put_bytes(&key, bytes, "image/jpeg").await {
        Ok(()) => Some(key),
        Err(_) => None,
    }
}

async fn ingest_observation(
    State(state): State<AppState>,
    mut multipart: Multipart,
) -> Result<Json<ObservationsResponse>, ApiError> {
    let (fields, file) = parse_multipart(&mut multipart).await?;
    let (bytes, _name, content_type) =
        file.ok_or_else(|| ApiError::bad_request("missing file field"))?;
    if !content_type.starts_with("image/") {
        return Err(ApiError::bad_request("File must be an image"));
    }

    let incident_id = fields.get("incident_id").cloned();
    let target_system_id = fields.get("target_system_id").cloned();
    let description = fields
        .get("description")
        .cloned()
        .unwrap_or_else(|| "Face observation".to_string());

    // An Observation is an atomic raw capture; it may be provisionally
    // assigned to an Incident and/or TargetSystem at ingest time.
    let observation_id = state
        .graph
        .create_observation(&description)
        .await
        .map_err(ierr)?;
    if let Some(iid) = &incident_id {
        state
            .graph
            .link_observation_to_incident(&observation_id, iid)
            .await
            .map_err(ierr)?;
    }
    if let Some(tsid) = &target_system_id {
        state
            .graph
            .link_observation_to_target_system(&observation_id, tsid)
            .await
            .map_err(ierr)?;
    }

    let faces = state.sidecar.detect(bytes, &content_type).await.map_err(sidecar_err)?;

    let mut outcomes = Vec::new();
    for face in &faces {
        let embedding = face.embedding.clone();

        let face_record_id = state
            .graph
            .create_face_record("surveillance")
            .await
            .map_err(ierr)?;
        state
            .graph
            .link_observation_to_face_record(&observation_id, &face_record_id)
            .await
            .map_err(ierr)?;

        let source_key = upload_face_crop(&state, &face_record_id, &face.face_crop).await;
        let embedding_id = db::insert_face_embedding(
            &state.pool,
            &face_record_id,
            None,
            None,
            "surveillance",
            &embedding,
            face.quality_score,
            Some(face.confidence),
            source_key.as_deref(),
        )
        .await
        .map_err(ierr)?;

        let outcome = resolve::resolve_observation(
            &state.graph,
            &state.pool,
            &state.resolution,
            &face_record_id,
            embedding_id,
            &embedding,
        )
        .await
        .map_err(ierr)?;
        outcomes.push(outcome);
    }

    Ok(Json(ObservationsResponse {
        observation_id,
        outcomes,
    }))
}

async fn enroll_identity(
    State(state): State<AppState>,
    mut multipart: Multipart,
) -> Result<Json<IdentityResponse>, ApiError> {
    let (fields, file) = parse_multipart(&mut multipart).await?;
    let full_name = fields
        .get("full_name")
        .cloned()
        .ok_or_else(|| ApiError::bad_request("full_name is required"))?;
    let document_type = fields.get("document_type").cloned();
    let document_number = fields.get("document_number").cloned();

    let (bytes, _name, content_type) =
        file.ok_or_else(|| ApiError::bad_request("photo is required"))?;
    if !content_type.starts_with("image/") {
        return Err(ApiError::bad_request("Photo must be an image"));
    }

    let faces = state.sidecar.detect(bytes, &content_type).await.map_err(sidecar_err)?;
    if faces.is_empty() {
        return Err(ApiError::new(
            StatusCode::UNPROCESSABLE_ENTITY,
            "No face detected in enrollment photo",
        ));
    }
    if faces.len() > 1 {
        return Err(ApiError::new(
            StatusCode::UNPROCESSABLE_ENTITY,
            format!(
                "Enrollment photo must contain exactly one face, found {}",
                faces.len()
            ),
        ));
    }
    let face = &faces[0];
    let embedding = face.embedding.clone();

    let identity_id = state
        .graph
        .create_identity(&full_name, document_type.as_deref(), document_number.as_deref())
        .await
        .map_err(ierr)?;
    let face_record_id = state
        .graph
        .create_face_record("enrollment")
        .await
        .map_err(ierr)?;
    let source_key = upload_face_crop(&state, &identity_id, &face.face_crop).await;
    db::insert_face_embedding(
        &state.pool,
        &face_record_id,
        None,
        Some(&identity_id),
        "enrollment",
        &embedding,
        face.quality_score,
        Some(face.confidence),
        source_key.as_deref(),
    )
    .await
    .map_err(ierr)?;

    let created_at = state
        .graph
        .get_identity(&identity_id)
        .await
        .map_err(ierr)?
        .map(|i| i.created_at)
        .unwrap_or_default();

    Ok(Json(IdentityResponse {
        id: identity_id,
        full_name,
        document_type,
        document_number,
        created_at,
    }))
}

async fn get_identity(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<IdentityResponse>, ApiError> {
    match state.graph.get_identity(&id).await.map_err(ierr)? {
        Some(i) => Ok(Json(IdentityResponse {
            id: i.id,
            full_name: i.full_name,
            document_type: i.document_type,
            document_number: i.document_number,
            created_at: i.created_at,
        })),
        None => Err(ApiError::not_found("Identity not found")),
    }
}

#[derive(Deserialize)]
struct ReviewQuery {
    status: Option<String>,
    page: Option<i32>,
    page_size: Option<i32>,
}

async fn list_review_queue(
    State(state): State<AppState>,
    Query(q): Query<ReviewQuery>,
) -> Result<Json<ReviewListResponse>, ApiError> {
    let page = q.page.unwrap_or(1).max(1);
    let page_size = q.page_size.unwrap_or(20).clamp(1, 100);
    let (items, total) = db::list_review_items(&state.pool, q.status.as_deref(), page, page_size)
        .await
        .map_err(ierr)?;
    Ok(Json(ReviewListResponse {
        items,
        total,
        page,
        page_size,
    }))
}

#[derive(Deserialize)]
struct ConfirmPersonBody {
    person_id: String,
    reviewed_by: String,
}

#[derive(Deserialize)]
struct ConfirmNewBody {
    name: String,
    reviewed_by: String,
}

#[derive(Deserialize)]
struct ConfirmIdentityBody {
    identity_id: String,
    reviewed_by: String,
}

#[derive(Deserialize)]
struct RejectBody {
    reviewed_by: String,
    #[serde(default)]
    notes: Option<String>,
}

async fn confirm_review(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(body): Json<ConfirmPersonBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let item_id = Uuid::parse_str(&id).map_err(|_| ApiError::not_found("Review item not found"))?;
    let done = resolve::confirm_review_as_person(&state.graph, &state.pool, item_id, &body.person_id, &body.reviewed_by)
        .await
        .map_err(ierr)?;
    if done.is_none() {
        return Err(ApiError::not_found("Review item not found or already reviewed"));
    }
    Ok(Json(serde_json::json!({ "ok": true })))
}

async fn confirm_review_new(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(body): Json<ConfirmNewBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let item_id = Uuid::parse_str(&id).map_err(|_| ApiError::not_found("Review item not found"))?;
    let done = resolve::confirm_review_as_new(&state.graph, &state.pool, item_id, &body.name, &body.reviewed_by)
        .await
        .map_err(ierr)?;
    if done.is_none() {
        return Err(ApiError::not_found("Review item not found or already reviewed"));
    }
    Ok(Json(serde_json::json!({ "ok": true })))
}

async fn confirm_review_identity(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(body): Json<ConfirmIdentityBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let item_id = Uuid::parse_str(&id).map_err(|_| ApiError::not_found("Review item not found"))?;
    let done = resolve::confirm_review_as_identity(&state.graph, &state.pool, item_id, &body.identity_id, &body.reviewed_by)
        .await
        .map_err(ierr)?;
    if done.is_none() {
        return Err(ApiError::not_found("Review item not found or already reviewed"));
    }
    Ok(Json(serde_json::json!({ "ok": true })))
}

async fn reject_review(
    State(state): State<AppState>,
    Path(id): Path<String>,
    Json(body): Json<RejectBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let item_id = Uuid::parse_str(&id).map_err(|_| ApiError::not_found("Review item not found"))?;
    let done = resolve::reject_review(&state.graph, &state.pool, item_id, &body.reviewed_by, body.notes.as_deref())
        .await
        .map_err(ierr)?;
    if done.is_none() {
        return Err(ApiError::not_found("Review item not found or already reviewed"));
    }
    Ok(Json(serde_json::json!({ "ok": true })))
}

async fn list_persons(
    State(state): State<AppState>,
) -> Result<Json<PersonListResponse>, ApiError> {
    let persons = state.graph.list_persons().await.map_err(ierr)?;
    let items = persons
        .into_iter()
        .map(|p| PersonSummary {
            id: p.id,
            name: p.name,
            created_at: p.created_at,
        })
        .collect();
    Ok(Json(PersonListResponse { items }))
}

async fn get_person(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<PersonDetail>, ApiError> {
    let Some(p) = state.graph.get_person(&id).await.map_err(ierr)? else {
        return Err(ApiError::not_found("Person not found"));
    };
    let identities = state
        .graph
        .list_identities_for_person(&id)
        .await
        .map_err(ierr)?
        .into_iter()
        .map(|i| IdentityResponse {
            id: i.id,
            full_name: i.full_name,
            document_type: i.document_type,
            document_number: i.document_number,
            created_at: i.created_at,
        })
        .collect();
    Ok(Json(PersonDetail {
        id: p.id,
        name: p.name,
        created_at: p.created_at,
        identities,
    }))
}

// ─── Target-centric container structure (tid:target-centric) ───────────────

#[derive(Deserialize)]
struct CreateTargetSystemBody {
    name: String,
    #[serde(default)]
    description: Option<String>,
}

#[derive(Deserialize)]
struct CreateSituationBody {
    name: String,
    #[serde(default)]
    description: Option<String>,
}

#[derive(Deserialize)]
struct CreateIncidentBody {
    description: String,
    #[serde(default)]
    occurred_at: Option<String>,
}

async fn create_target_system(
    State(state): State<AppState>,
    Json(body): Json<CreateTargetSystemBody>,
) -> Result<Json<TargetSystemSummary>, ApiError> {
    let id = state
        .graph
        .create_target_system(&body.name, body.description.as_deref())
        .await
        .map_err(ierr)?;
    let node = state.graph.get_target_system(&id).await.map_err(ierr)?.unwrap();
    Ok(Json(TargetSystemSummary {
        id: node.id,
        name: node.name,
        created_at: node.created_at,
    }))
}

async fn list_target_systems(
    State(state): State<AppState>,
) -> Result<Json<TargetSystemListResponse>, ApiError> {
    let items = state
        .graph
        .list_target_systems()
        .await
        .map_err(ierr)?
        .into_iter()
        .map(|t| TargetSystemSummary {
            id: t.id,
            name: t.name,
            created_at: t.created_at,
        })
        .collect();
    Ok(Json(TargetSystemListResponse { items }))
}

async fn get_target_system(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<TargetSystemDetail>, ApiError> {
    let Some(t) = state.graph.get_target_system(&id).await.map_err(ierr)? else {
        return Err(ApiError::not_found("Target system not found"));
    };
    let situations = state
        .graph
        .list_situations(&id)
        .await
        .map_err(ierr)?
        .into_iter()
        .map(|s| SituationSummary {
            id: s.id,
            name: s.name,
            created_at: s.created_at,
        })
        .collect();
    Ok(Json(TargetSystemDetail {
        id: t.id,
        name: t.name,
        description: t.description,
        created_at: t.created_at,
        situations,
    }))
}

async fn create_situation(
    State(state): State<AppState>,
    Path(target_system_id): Path<String>,
    Json(body): Json<CreateSituationBody>,
) -> Result<Json<SituationSummary>, ApiError> {
    let Some(id) = state
        .graph
        .create_situation(&target_system_id, &body.name, body.description.as_deref())
        .await
        .map_err(ierr)?
    else {
        return Err(ApiError::not_found("Target system not found"));
    };
    let node = state.graph.get_situation(&id).await.map_err(ierr)?.unwrap();
    Ok(Json(SituationSummary {
        id: node.id,
        name: node.name,
        created_at: node.created_at,
    }))
}

async fn get_situation(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<SituationDetail>, ApiError> {
    let Some(s) = state.graph.get_situation(&id).await.map_err(ierr)? else {
        return Err(ApiError::not_found("Situation not found"));
    };
    let incidents = state
        .graph
        .list_incidents(&id)
        .await
        .map_err(ierr)?
        .into_iter()
        .map(|i| IncidentSummary {
            id: i.id,
            description: i.description,
            occurred_at: i.occurred_at,
            created_at: i.created_at,
        })
        .collect();
    Ok(Json(SituationDetail {
        id: s.id,
        name: s.name,
        description: s.description,
        created_at: s.created_at,
        target_system_id: s.target_system_id,
        incidents,
    }))
}

async fn create_incident(
    State(state): State<AppState>,
    Path(situation_id): Path<String>,
    Json(body): Json<CreateIncidentBody>,
) -> Result<Json<IncidentSummary>, ApiError> {
    let Some(id) = state
        .graph
        .create_incident(&situation_id, &body.description, body.occurred_at.as_deref())
        .await
        .map_err(ierr)?
    else {
        return Err(ApiError::not_found("Situation not found"));
    };
    let node = state.graph.get_incident(&id).await.map_err(ierr)?.unwrap();
    Ok(Json(IncidentSummary {
        id: node.id,
        description: node.description,
        occurred_at: node.occurred_at,
        created_at: node.created_at,
    }))
}

async fn get_incident(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<IncidentDetail>, ApiError> {
    let Some(i) = state.graph.get_incident(&id).await.map_err(ierr)? else {
        return Err(ApiError::not_found("Incident not found"));
    };
    let observations = state
        .graph
        .list_observations(&id)
        .await
        .map_err(ierr)?
        .into_iter()
        .map(|o| ObservationSummary {
            id: o.id,
            description: o.description,
            created_at: o.created_at,
        })
        .collect();
    Ok(Json(IncidentDetail {
        id: i.id,
        description: i.description,
        occurred_at: i.occurred_at,
        created_at: i.created_at,
        observations,
    }))
}

// ─── Auth ───────────────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct RegisterBody {
    username: String,
    password: String,
}

#[derive(Deserialize)]
struct LoginBody {
    username: String,
    password: String,
}

async fn register(
    State(state): State<AppState>,
    Json(body): Json<RegisterBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let username = body.username.trim();
    if username.is_empty() {
        return Err(ApiError::bad_request("username is required"));
    }
    if body.password.len() < 8 {
        return Err(ApiError::bad_request("password must be at least 8 characters"));
    }

    let hash = auth::hash_password(&body.password).map_err(ierr)?;
    let id = Uuid::new_v4();
    match db::create_user(&state.pool, id, username, &hash).await {
        Ok(()) => Ok(Json(serde_json::json!({ "id": id.to_string(), "username": username }))),
        Err(e) if db::is_unique_violation(&e) => {
            Err(ApiError::new(StatusCode::CONFLICT, "username already exists"))
        }
        Err(e) => Err(ierr(e)),
    }
}

async fn login(
    State(state): State<AppState>,
    Json(body): Json<LoginBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let user = db::get_user_by_username(&state.pool, body.username.trim())
        .await
        .map_err(ierr)?
        .ok_or_else(|| ApiError::unauthorized("invalid credentials"))?;

    if !auth::verify_password(&body.password, &user.password_hash) {
        return Err(ApiError::unauthorized("invalid credentials"));
    }

    let token = auth::create_token(&state.auth, &user.id, &user.username).map_err(ierr)?;
    Ok(Json(serde_json::json!({ "token": token })))
}

async fn auth_me(Extension(user): Extension<AuthUser>) -> Json<serde_json::Value> {
    Json(serde_json::json!({ "user_id": user.user_id, "username": user.username }))
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info,trackid_backend=debug".into()),
        )
        .init();

    let database_url = env("DATABASE_URL", "postgres://postgres:password@localhost:5432/trackid");
    let ml_sidecar_url = env("ML_SIDECAR_URL", "http://localhost:8001");
    let memgraph_uri = env("MEMGRAPH_URI", "bolt://localhost:7687");
    let s3_endpoint = env("S3_ENDPOINT_URL", "http://localhost:9010");
    let s3_public = env("S3_PUBLIC_URL", &s3_endpoint);
    let s3_ak = env("S3_ACCESS_KEY_ID", "minioadmin");
    let s3_sk = env("S3_SECRET_ACCESS_KEY", "minioadmin");
    let s3_bucket = env("S3_BUCKET_NAME", "trackid-media");
    let s3_region = env("S3_REGION_NAME", "us-east-1");
    let bind = env("BIND", "0.0.0.0:8080");

    let pool = PgPoolOptions::new()
        .max_connections(10)
        .connect(&database_url)
        .await?;
    db::ensure_face_schema(&pool).await?;

    let storage = S3Storage::new(
        &s3_bucket,
        &s3_endpoint,
        &s3_public,
        &s3_ak,
        &s3_sk,
        &s3_region,
    )?;
    let sidecar = SidecarClient::new(ml_sidecar_url);
    let jobs = Arc::new(JobStore::new());
    let graph = GraphService::new(&memgraph_uri)?;

    let ontology_dir = env("ONTOLOGY_DIR", "../backend/app/ontology/vendor");
    match ontology::load_taxonomy(std::path::Path::new(&ontology_dir)) {
        Ok(taxonomy) => {
            let (valid, violations) = ontology::check_conformance(&taxonomy);
            tracing::info!(
                "ontology: {} classes, {} relations, conformant={valid} ({} violations)",
                taxonomy.nodes.len(),
                taxonomy.relations.len(),
                violations.len()
            );
            for v in &violations {
                tracing::warn!("ontology violation: {v}");
            }
            match ontology::seed_taxonomy(&graph, &taxonomy).await {
                Ok((c, r, sc, d, rg)) => tracing::info!(
                    "taxonomy seeded: {c} classes, {r} relations, {sc} subclass, {d} domain, {rg} range edges"
                ),
                Err(e) => tracing::warn!("taxonomy seed failed: {e}"),
            }
        }
        Err(e) => tracing::warn!("ontology import failed: {e}"),
    }

    let resolution = ResolutionConfig {
        tau_high: env("RESOLUTION_TAU_HIGH", "0.75").parse().unwrap_or(0.75),
        tau_low: env("RESOLUTION_TAU_LOW", "0.50").parse().unwrap_or(0.50),
        pool_size: env("RESOLUTION_CANDIDATE_POOL_SIZE", "200")
            .parse()
            .unwrap_or(200),
        top_k: env("RESOLUTION_TOP_K", "5").parse().unwrap_or(5),
    };

    let auth = AuthState::new(
        env("JWT_SECRET", "dev-secret-change-me"),
        env("JWT_TOKEN_HOURS", "24").parse().unwrap_or(24),
    );

    let state = AppState {
        pool,
        storage,
        sidecar,
        jobs,
        graph,
        resolution,
        auth,
    };

    let auth_mw = axum::middleware::from_fn_with_state(state.auth.clone(), auth_middleware);

    let app = Router::new()
        .route("/health", get(health))
        .route("/api/v1/auth/register", post(register))
        .route("/api/v1/auth/login", post(login))
        .merge(
            Router::new()
                .route("/api/v1/auth/me", get(auth_me))
                .route("/api/v1/process-video", post(process_video))
                .route("/api/v1/process-video/{job_id}", get(get_job))
                .route("/api/v1/videos", get(list_videos))
                .route("/api/v1/videos/{video_id}", get(get_video))
                .route("/api/v1/observations", post(ingest_observation))
                .route("/api/v1/identities", post(enroll_identity))
                .route("/api/v1/identities/{identity_id}", get(get_identity))
                .route("/api/v1/review-queue", get(list_review_queue))
                .route("/api/v1/review-queue/{item_id}/confirm", post(confirm_review))
                .route("/api/v1/review-queue/{item_id}/confirm-new", post(confirm_review_new))
                .route(
                    "/api/v1/review-queue/{item_id}/confirm-identity",
                    post(confirm_review_identity),
                )
                .route("/api/v1/review-queue/{item_id}/reject", post(reject_review))
                .route("/api/v1/persons", get(list_persons))
                .route("/api/v1/persons/{person_id}", get(get_person))
                .route(
                    "/api/v1/target-systems",
                    get(list_target_systems).post(create_target_system),
                )
                .route("/api/v1/target-systems/{id}", get(get_target_system))
                .route(
                    "/api/v1/target-systems/{id}/situations",
                    post(create_situation),
                )
                .route("/api/v1/situations/{id}", get(get_situation))
                .route("/api/v1/situations/{id}/incidents", post(create_incident))
                .route("/api/v1/incidents/{id}", get(get_incident))
                .route_layer(auth_mw),
        )
        .with_state(state);

    let listener = tokio::net::TcpListener::bind(&bind).await?;
    tracing::info!("trackid backend (Rust) listening on {bind}");
    axum::serve(listener, app).await?;
    Ok(())
}
