use serde::{Deserialize, Serialize};

/// One detected face, as returned by the ML sidecar. `embedding` and
/// `is_best_face` are sidecar-internal (used for persistence) and are
/// stripped from client responses to match the Python API shape.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FaceDetection {
    pub frame_number: i32,
    pub timestamp_seconds: f64,
    pub confidence: f64,
    pub quality_score: Option<f64>,
    pub blur_score: Option<f64>,
    pub bbox: Option<Vec<f64>>,
    pub is_embedding: bool,
    #[serde(default, skip_serializing)]
    pub embedding: Option<Vec<f64>>,
    pub estimated_age: Option<i32>,
    pub estimated_gender: Option<String>,
    pub cluster_id: Option<i32>,
    pub face_crop: Option<String>,
    #[serde(default, skip_serializing)]
    pub is_best_face: Option<bool>,
    #[serde(default, skip_serializing)]
    pub crop_storage_key: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Track {
    pub track_id: i32,
    pub best_face: FaceDetection,
    pub all_faces: Vec<FaceDetection>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VideoProcessingResponse {
    pub tracks: Vec<Track>,
    pub track_count: i32,
    pub video_id: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
pub struct VideoJobCreated {
    pub job_id: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct VideoJobStatus {
    pub job_id: String,
    pub status: &'static str,
    pub frame_count: i32,
    pub expected_frames: i32,
    pub percent: f64,
    pub elapsed_seconds: f64,
    pub eta_seconds: Option<f64>,
    pub error: Option<String>,
    pub result: Option<VideoProcessingResponse>,
}

#[derive(Debug, Clone, Serialize)]
pub struct VideoSummary {
    pub video_id: String,
    pub original_filename: Option<String>,
    pub created_at: chrono::DateTime<chrono::Utc>,
    pub track_count: i32,
    pub frame_count: i32,
    pub total_detections: i32,
}

#[derive(Debug, Clone, Serialize)]
pub struct VideoListResponse {
    pub items: Vec<VideoSummary>,
    pub total: i64,
    pub page: i32,
    pub page_size: i32,
}

/// The ML sidecar's job-status payload (`GET /ml/v1/video-jobs/{id}`).
#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
pub struct SidecarJobStatus {
    pub job_id: String,
    pub status: String,
    #[serde(default)]
    pub frame_count: i32,
    #[serde(default)]
    pub expected_frames: i32,
    #[serde(default)]
    pub total_detections: i32,
    #[serde(default)]
    pub percent: f64,
    #[serde(default)]
    pub error: Option<String>,
    #[serde(default)]
    pub result: Option<SidecarResult>,
}

#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
pub struct SidecarResult {
    #[serde(default)]
    pub tracks: Vec<Track>,
    #[serde(default)]
    pub track_count: i32,
    #[serde(default)]
    pub video_id: Option<String>,
}

/// Processing params with defaults applied (mirrors the Python orchestration).
#[derive(Debug, Clone)]
pub struct ResolvedParams {
    pub interval_seconds: f64,
    pub embed_interval_seconds: f64,
    pub min_quality: f64,
    pub min_blur: f64,
    pub full_detection_every_frame: bool,
    pub iou_threshold: f64,
    pub cluster_eps: f64,
    pub cluster_min_samples: i32,
}

/// A single face returned by the sidecar's image `detect` endpoint (the full
/// pipeline: detection + embedding + quality). Distinct from `FaceDetection`,
/// which is the video-processing detection shape with frame/timestamp fields.
#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
pub struct DetectedFace {
    pub bbox: Vec<f64>,
    pub confidence: f64,
    pub embedding: Vec<f64>,
    #[serde(default)]
    pub estimated_age: Option<i32>,
    #[serde(default)]
    pub estimated_gender: Option<String>,
    #[serde(default)]
    pub quality_score: Option<f64>,
    #[serde(default)]
    pub face_crop: Option<String>,
}

/// A candidate match for a face observation (merged over the surveillance
/// and enrollment pools, re-ranked by similarity).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Candidate {
    pub kind: String,
    pub person_id: Option<String>,
    pub identity_id: Option<String>,
    pub similarity: f64,
    pub embedding_id: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct ObservationOutcome {
    pub outcome: &'static str,
    pub person_id: Option<String>,
    pub review_item_id: Option<String>,
    pub candidates: Vec<Candidate>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ObservationsResponse {
    pub observation_id: String,
    pub outcomes: Vec<ObservationOutcome>,
}

#[derive(Debug, Clone, Serialize)]
pub struct IdentityResponse {
    pub id: String,
    pub full_name: String,
    pub document_type: Option<String>,
    pub document_number: Option<String>,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct PersonSummary {
    pub id: String,
    pub name: String,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct PersonDetail {
    pub id: String,
    pub name: String,
    pub created_at: String,
    pub identities: Vec<IdentityResponse>,
}

#[derive(Debug, Clone, Serialize)]
pub struct PersonListResponse {
    pub items: Vec<PersonSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ReviewItem {
    pub id: String,
    pub embedding_id: String,
    pub face_record_id: String,
    pub candidate_kind: String,
    pub candidate_person_id: Option<String>,
    pub candidate_identity_id: Option<String>,
    pub similarity: f64,
    pub top_candidates: Vec<Candidate>,
    pub status: String,
    pub reviewed_by: Option<String>,
    pub reviewed_at: Option<String>,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct ReviewListResponse {
    pub items: Vec<ReviewItem>,
    pub total: i64,
    pub page: i32,
    pub page_size: i32,
}

// ─── Target-centric container structure (tid:target-centric) ───────────────

#[derive(Debug, Clone, Serialize)]
pub struct TargetSystemSummary {
    pub id: String,
    pub name: String,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct TargetSystemDetail {
    pub id: String,
    pub name: String,
    pub description: Option<String>,
    pub created_at: String,
    pub situations: Vec<SituationSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct TargetSystemListResponse {
    pub items: Vec<TargetSystemSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct SituationSummary {
    pub id: String,
    pub name: String,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct SituationDetail {
    pub id: String,
    pub name: String,
    pub description: Option<String>,
    pub created_at: String,
    pub target_system_id: String,
    pub incidents: Vec<IncidentSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct IncidentSummary {
    pub id: String,
    pub description: String,
    pub occurred_at: Option<String>,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct IncidentDetail {
    pub id: String,
    pub description: String,
    pub occurred_at: Option<String>,
    pub created_at: String,
    pub observations: Vec<ObservationSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ObservationSummary {
    pub id: String,
    pub description: String,
    pub created_at: String,
}
