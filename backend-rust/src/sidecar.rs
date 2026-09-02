use reqwest::multipart;

use crate::types::{DetectedFace, ResolvedParams, SidecarJobStatus};

#[derive(Debug)]
pub struct SidecarError {
    pub status: u16,
    pub detail: String,
}

impl std::fmt::Display for SidecarError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "sidecar {}: {}", self.status, self.detail)
    }
}

impl std::error::Error for SidecarError {}

impl From<reqwest::Error> for SidecarError {
    fn from(e: reqwest::Error) -> Self {
        SidecarError {
            status: 502,
            detail: e.to_string(),
        }
    }
}

#[derive(Clone)]
pub struct SidecarClient {
    base_url: String,
    client: reqwest::Client,
}

impl SidecarClient {
    pub fn new(base_url: String) -> Self {
        Self {
            base_url,
            client: reqwest::Client::new(),
        }
    }

    pub async fn create_video_job(
        &self,
        file_path: &std::path::Path,
        file_name: Option<&str>,
        content_type: Option<&str>,
        params: &ResolvedParams,
    ) -> Result<String, SidecarError> {
        let bytes = tokio::fs::read(file_path)
            .await
            .map_err(|e| SidecarError {
                status: 500,
                detail: e.to_string(),
            })?;

        let part = multipart::Part::bytes(bytes)
            .file_name(file_name.unwrap_or("video.mp4").to_string())
            .mime_str(content_type.unwrap_or("video/mp4"))?;
        let form = multipart::Form::new().part("file", part);

        let resp = self
            .client
            .post(format!("{}/ml/v1/video-jobs", self.base_url))
            .query(&[
                ("interval_seconds", params.interval_seconds.to_string()),
                ("embed_interval_seconds", params.embed_interval_seconds.to_string()),
                ("min_quality", params.min_quality.to_string()),
                ("min_blur", params.min_blur.to_string()),
                (
                    "full_detection_every_frame",
                    params.full_detection_every_frame.to_string(),
                ),
                ("iou_threshold", params.iou_threshold.to_string()),
                ("cluster_eps", params.cluster_eps.to_string()),
                ("cluster_min_samples", params.cluster_min_samples.to_string()),
            ])
            .multipart(form)
            .send()
            .await?;

        if !resp.status().is_success() {
            return Err(SidecarError {
                status: resp.status().as_u16(),
                detail: resp.text().await.unwrap_or_default(),
            });
        }

        let body: serde_json::Value = resp.json().await?;
        let job_id = body["job_id"]
            .as_str()
            .ok_or_else(|| SidecarError {
                status: 502,
                detail: "sidecar response missing job_id".to_string(),
            })?;
        Ok(job_id.to_string())
    }

    pub async fn get_video_job(&self, job_id: &str) -> Result<SidecarJobStatus, SidecarError> {
        let resp = self
            .client
            .get(format!("{}/ml/v1/video-jobs/{job_id}", self.base_url))
            .send()
            .await?;

        if !resp.status().is_success() {
            return Err(SidecarError {
                status: resp.status().as_u16(),
                detail: resp.text().await.unwrap_or_default(),
            });
        }

        Ok(resp.json::<SidecarJobStatus>().await?)
    }

    /// Full-pipeline face detection (detection + embedding + quality). Each
    /// face carries a base64 `face_crop` data URI when a crop was extracted.
    pub async fn detect(
        &self,
        image_bytes: Vec<u8>,
        content_type: &str,
    ) -> Result<Vec<DetectedFace>, SidecarError> {
        let part = multipart::Part::bytes(image_bytes)
            .file_name("image.jpg".to_string())
            .mime_str(content_type)?;
        let form = multipart::Form::new().part("file", part);

        let resp = self
            .client
            .post(format!("{}/ml/v1/detect", self.base_url))
            .multipart(form)
            .send()
            .await?;

        if !resp.status().is_success() {
            return Err(SidecarError {
                status: resp.status().as_u16(),
                detail: resp.text().await.unwrap_or_default(),
            });
        }

        let body: serde_json::Value = resp.json().await?;
        let faces: Vec<DetectedFace> = serde_json::from_value(body["faces"].clone())
            .map_err(|e| SidecarError {
                status: 502,
                detail: format!("bad sidecar response: {e}"),
            })?;
        Ok(faces)
    }
}
