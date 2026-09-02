// Phase 0 spike 3: Rust (axum) orchestration -> Python (FastAPI) ML sidecar.
//
// Proves the §6 seam: multipart upload, JSON round-trip, and the async
// job + polling pattern, over HTTP+JSON. No real ML involved — both sides
// are throwaway, and the sidecar returns mock detections.
use axum::{
    extract::{Multipart, Path, Query},
    http::StatusCode,
    response::{IntoResponse, Response},
    routing::{get, post},
    Json, Router,
};
use serde::Deserialize;

const SIDECAR: &str = "http://localhost:8001";

async fn extract_file(mut multipart: Multipart) -> Option<Vec<u8>> {
    while let Ok(Some(field)) = multipart.next_field().await {
        if field.name() == Some("file") {
            return field.bytes().await.ok().map(|b| b.to_vec());
        }
    }
    None
}

fn file_part(bytes: Vec<u8>, name: &'static str) -> reqwest::multipart::Part {
    reqwest::multipart::Part::bytes(bytes).file_name(name)
}

async fn detect_faces(multipart: Multipart) -> Response {
    let Some(bytes) = extract_file(multipart).await else {
        return (StatusCode::BAD_REQUEST, "missing file field").into_response();
    };
    let form = reqwest::multipart::Form::new().part("file", file_part(bytes, "frame.jpg"));
    match reqwest::Client::new()
        .post(format!("{SIDECAR}/ml/v1/detect"))
        .multipart(form)
        .send()
        .await
    {
        Ok(resp) => Json(resp.json::<serde_json::Value>().await.unwrap_or_default()).into_response(),
        Err(e) => (StatusCode::BAD_GATEWAY, e.to_string()).into_response(),
    }
}

#[derive(Deserialize)]
struct VideoJobParams {
    expected_frames: Option<i64>,
}

async fn process_video(Query(params): Query<VideoJobParams>, multipart: Multipart) -> Response {
    let Some(bytes) = extract_file(multipart).await else {
        return (StatusCode::BAD_REQUEST, "missing file field").into_response();
    };
    let ef = params.expected_frames.unwrap_or(10);
    let form = reqwest::multipart::Form::new().part("file", file_part(bytes, "video.mp4"));
    let url = format!("{SIDECAR}/ml/v1/video-jobs?expected_frames={ef}");
    match reqwest::Client::new().post(&url).multipart(form).send().await {
        Ok(resp) => Json(resp.json::<serde_json::Value>().await.unwrap_or_default()).into_response(),
        Err(e) => (StatusCode::BAD_GATEWAY, e.to_string()).into_response(),
    }
}

async fn poll_video_job(Path(job_id): Path<String>) -> Response {
    let url = format!("{SIDECAR}/ml/v1/video-jobs/{job_id}");
    match reqwest::Client::new().get(&url).send().await {
        Ok(resp) => {
            let status = resp.status();
            let body = resp.json::<serde_json::Value>().await.unwrap_or_default();
            (status, Json(body)).into_response()
        }
        Err(e) => (StatusCode::BAD_GATEWAY, e.to_string()).into_response(),
    }
}

#[tokio::main]
async fn main() {
    let app = Router::new()
        .route("/api/v1/detect-faces", post(detect_faces))
        .route("/api/v1/process-video", post(process_video))
        .route("/api/v1/process-video/{job_id}", get(poll_video_job));

    let listener = tokio::net::TcpListener::bind("0.0.0.0:8080").await.unwrap();
    println!("axum orchestration spike on :8080 -> sidecar {SIDECAR}");
    axum::serve(listener, app).await.unwrap();
}
